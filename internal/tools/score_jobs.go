package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
		scored, err := sj.scoreFit(ctx, exec.ResumeText, jobs)
		if err != nil {
			return nil, fmt.Errorf("score_jobs: fit scoring: %w", err)
		}
		fit = scored
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
		m["fit_score"] = rj.fit
		m["recency_score"] = rj.recency
		m["score"] = rj.final
		out[i] = m
	}
	return map[string]any{"ranked_jobs": out}, nil
}

// scoreFit asks the LLM for a 0..100 fit score per job and returns job_id → 0..1.
func (sj *ScoreJobs) scoreFit(ctx context.Context, resume string, jobs []models.Job) (map[string]float64, error) {
	type brief struct {
		JobID    string `json:"job_id"`
		Title    string `json:"title"`
		Company  string `json:"company"`
		Location string `json:"location"`
	}
	briefs := make([]brief, len(jobs))
	for i, j := range jobs {
		briefs[i] = brief{JobID: j.JobID, Title: j.Title, Company: j.Company, Location: j.Location}
	}
	jobsJSON, _ := json.Marshal(briefs)

	system := `You are a job-matching assistant. Score how well each job fits the candidate's resume on a scale of 0 to 100 (100 = perfect fit). Respond ONLY with a JSON array of objects {"job_id": string, "score": number}. No prose, no markdown.`
	user := fmt.Sprintf("Candidate resume:\n%s\n\nJobs to score:\n%s\n\nReturn the JSON array now.", resume, jobsJSON)

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
