package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
)

// recencyWindow defines how the posting date maps to a 0..1 recency score:
// posted now → 1.0, posted >= 30 days ago → 0.0, linear in between.
const recencyWindow = 30 * 24 * time.Hour

// maxRankedResults caps how many jobs a single run surfaces. Only these are
// recorded as "seen", so any new matches beyond the cap resurface in later runs
// (best-first pagination) rather than being silently dropped.
const maxRankedResults = 25

// Scoring is split into throttled batches so we can evaluate a large slice of
// the candidate set per run (not just the freshest few) without tripping the
// provider's per-minute token limit (Groq free tier ≈ 12k TPM). Runs are
// background jobs, so a couple of minutes is fine. scoreBudget caps total jobs
// scored per run; batchSize per LLM call; batchDelay throttles between calls.
const (
	scoreBudget = 100
	batchSize   = 20
	batchDelay  = 35 * time.Second
)

// scoreThreshold drops weak matches: we don't surface anything below this blended score.
const scoreThreshold = 0.5

// scoringDescriptionChars / scoringResumeChars cap text sent to the LLM. The
// opening of a description and résumé carry most of the matching signal, so
// trimming them keeps the request small without hurting ranking much.
const (
	scoringDescriptionChars = 400
	scoringResumeChars      = 3000
)

// Completer is the slice of the agent we need: a single raw LLM completion.
// Declared here (not imported from agent) to avoid an import cycle, since the
// agent package imports tools. *agent.AgentManager satisfies this structurally.
type Completer interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type ScoreJobs struct {
	store *store.Store
	llm   Completer
}

func NewScoreJobs(s *store.Store, llm Completer) *ScoreJobs {
	return &ScoreJobs{store: s, llm: llm}
}

func (sj *ScoreJobs) Name() string { return "score_jobs" }

func (sj *ScoreJobs) Description() string {
	return `Ranks jobs (from job_search) by how well they fit the candidate's resume, blended with
how recently each was posted. The recency-vs-fit balance is set per worker. Also records the
ranked jobs as "seen" so they aren't shown again in future runs.
Input: {"jobs": [ ...the output array from job_search... ]}
Output: {"ranked_jobs": [{...job fields..., "score": 0-1, "fit_score": 0-1, "recency_score": 0-1}]} (up to 25 best)
Use this as the final step of a job hunt, after job_search.`
}

func (sj *ScoreJobs) Execute(ctx context.Context, input map[string]any, exec ExecutionContext) (map[string]any, error) {
	jobs := parseJobs(input["jobs"])
	if len(jobs) == 0 {
		return map[string]any{"ranked_jobs": []any{}}, nil
	}

	now := time.Now()
	w := float64(exec.RecencyWeight) / 100.0

	// fit is 0..1 per job_id. Skip the LLM entirely when recency is weighted 100%.
	fit := map[string]float64{}
	if exec.RecencyWeight < 100 {
		fit = sj.scoreFitBatched(ctx, buildIntent(exec), exec.ResumeText, jobs)
	}

	type rankedJob struct {
		job     models.Job
		fit     float64
		recency float64
		final   float64
	}
	ranked := make([]rankedJob, 0, len(jobs))
	for _, job := range jobs {
		f := fit[job.JobID]
		r := recencyScore(job.PostedAt, now)
		ranked = append(ranked, rankedJob{job: job, fit: f, recency: r, final: w*r + (1-w)*f})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].final > ranked[j].final })

	// Drop weak matches entirely (better to show a few great ones than pad).
	kept := make([]rankedJob, 0, len(ranked))
	for _, rj := range ranked {
		if rj.final >= scoreThreshold {
			kept = append(kept, rj)
		}
	}
	ranked = kept

	// Keep only the best N. The rest stay unseen and come back next run.
	if len(ranked) > maxRankedResults {
		ranked = ranked[:maxRankedResults]
	}

	// Record the surfaced jobs as seen (idempotent), in ranked order.
	surfaced := make([]models.Job, len(ranked))
	for i, rj := range ranked {
		surfaced[i] = rj.job
	}
	if err := sj.store.RecordSeenJobs(ctx, exec.WorkerID, surfaced); err != nil {
		return nil, fmt.Errorf("score_jobs: record seen: %w", err)
	}

	out := make([]map[string]any, len(ranked))
	for i, rj := range ranked {
		m := jobToMap(rj.job)
		delete(m, "description") // not needed in the stored/displayed result
		m["fit_score"] = rj.fit
		m["recency_score"] = rj.recency
		m["score"] = rj.final
		out[i] = m
	}
	return map[string]any{"ranked_jobs": out}, nil
}

// buildIntent describes what the user is looking for, from the worker's
// structured config, for the scorer to weigh alongside the résumé.
func buildIntent(exec ExecutionContext) string {
	var b strings.Builder
	if exec.Category != "" {
		b.WriteString("Category: " + exec.Category + ". ")
	}
	if len(exec.Keywords) > 0 {
		b.WriteString("Must relate to: " + strings.Join(exec.Keywords, ", ") + ". ")
	}
	if strings.TrimSpace(exec.Instructions) != "" {
		b.WriteString("Notes: " + exec.Instructions)
	}
	return strings.TrimSpace(b.String())
}

func truncateChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// mostRecent returns the n most-recently-posted jobs.
func mostRecent(jobs []models.Job, n int) []models.Job {
	if len(jobs) <= n {
		return jobs
	}
	sorted := append([]models.Job(nil), jobs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PostedAt.After(sorted[j].PostedAt) })
	return sorted[:n]
}

// scoreFitBatched scores up to scoreBudget jobs (most recent) across several LLM
// calls, throttled to respect the per-minute token limit. Returns job_id → fit
// (0..1). On a batch error (e.g. rate limit), it keeps what it has so the run
// still completes; unscored jobs simply rank on recency this run.
func (sj *ScoreJobs) scoreFitBatched(ctx context.Context, intent, resume string, jobs []models.Job) map[string]float64 {
	jobs = mostRecent(jobs, scoreBudget)
	fit := make(map[string]float64, len(jobs))
	for i := 0; i < len(jobs); i += batchSize {
		end := i + batchSize
		if end > len(jobs) {
			end = len(jobs)
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				return fit
			case <-time.After(batchDelay):
			}
		}
		scores, err := sj.scoreFit(ctx, intent, resume, jobs[i:end])
		if err != nil {
			slog.Warn("score_jobs: batch scoring failed", "error", err, "batch_start", i)
			break
		}
		for k, v := range scores {
			fit[k] = v
		}
	}
	return fit
}

// scoreFit asks the LLM for a 0..100 fit score per job and returns job_id → 0..1.
// It scores each job against both the user's search intent (instructions) and
// their résumé.
func (sj *ScoreJobs) scoreFit(ctx context.Context, instructions, resume string, jobs []models.Job) (map[string]float64, error) {
	type brief struct {
		JobID       string `json:"job_id"`
		Title       string `json:"title"`
		Company     string `json:"company"`
		Location    string `json:"location"`
		Description string `json:"description"`
	}
	briefs := make([]brief, len(jobs))
	for i, j := range jobs {
		briefs[i] = brief{
			JobID:       j.JobID,
			Title:       j.Title,
			Company:     j.Company,
			Location:    j.Location,
			Description: truncateChars(j.Description, scoringDescriptionChars),
		}
	}
	jobsJSON, _ := json.Marshal(briefs)

	if strings.TrimSpace(instructions) == "" {
		instructions = "(not specified)"
	}
	if strings.TrimSpace(resume) == "" {
		resume = "(not provided)"
	} else {
		resume = truncateChars(resume, scoringResumeChars)
	}

	system := `You are a strict job-matching assistant. For each job, use its title and description to score how well it matches BOTH what the user is looking for AND their résumé, from 0 to 100.
Be strict and deterministic:
- Infer the candidate's seniority/years of experience from the résumé. HEAVILY penalize roles that require clearly more seniority than the candidate has (e.g. Staff, Principal, Lead, Director, or "8+ years" roles for a mid-level candidate) even if the skills match — score those under 30.
- Penalize wrong role, wrong domain, or a location that conflicts with the user's stated preference.
- Reserve 70+ for genuinely strong matches in the right role, level, and domain.
Respond ONLY with a JSON array of objects {"job_id": string, "score": number}. No prose, no markdown.`
	user := fmt.Sprintf("What the user is looking for:\n%s\n\nCandidate résumé:\n%s\n\nJobs to score:\n%s\n\nReturn the JSON array now.", instructions, resume, jobsJSON)

	raw, err := sj.llm.Complete(ctx, system, user)
	if err != nil {
		return nil, err
	}

	var scores []struct {
		JobID string  `json:"job_id"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(stripCodeFences(raw)), &scores); err != nil {
		return nil, fmt.Errorf("parse scores: %w, raw: %s", err, raw)
	}

	out := make(map[string]float64, len(scores))
	for _, s := range scores {
		out[s.JobID] = clamp01(s.Score / 100.0)
	}
	return out, nil
}

func recencyScore(postedAt, now time.Time) float64 {
	if postedAt.IsZero() {
		return 0
	}
	age := now.Sub(postedAt)
	if age < 0 {
		age = 0
	}
	return clamp01(1 - age.Hours()/recencyWindow.Hours())
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func parseJobs(v any) []models.Job {
	switch arr := v.(type) {
	case []any:
		jobs := make([]models.Job, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				jobs = append(jobs, jobFromMap(m))
			}
		}
		return jobs
	case []map[string]any:
		jobs := make([]models.Job, 0, len(arr))
		for _, m := range arr {
			jobs = append(jobs, jobFromMap(m))
		}
		return jobs
	default:
		return nil
	}
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
