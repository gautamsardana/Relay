package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
)

// ErrRateLimited marks a scoring failure caused by the model provider's rate or
// quota limits. The executor fails the step immediately rather than retrying
// inline (which would just hit the same wall) — the worker's schedule is the retry.
var ErrRateLimited = errors.New("scoring rate-limited by the model provider")

// recencyWindow defines how the posting date maps to a 0..1 recency score:
// posted now → 1.0, posted >= 30 days ago → 0.0, linear in between.
const recencyWindow = 30 * 24 * time.Hour

// maxRankedResults caps how many jobs a single run surfaces. Only these are
// recorded as "seen", so any new matches beyond the cap resurface in later runs
// (best-first pagination) rather than being silently dropped.
const maxRankedResults = 25

// Scoring runs in batches only to keep each LLM request under the provider's
// per-request token limit. scoreBudget caps how many jobs (most recent) we score
// per run; anything beyond it resurfaces next run. We deliberately do NOT throttle
// or retry between batches: if the provider rate-limits us, the run fails fast
// (ErrRateLimited) and the worker's next scheduled run retries once quota resets.
// Blocking on backoff here only made a step outlive its deadline and collide with
// the reconciler.
const (
	scoreBudget = 40
	batchSize   = 20
)

// scoreThreshold drops weak matches: we don't surface anything below this blended score.
const scoreThreshold = 0.5

// fitFloor is a hard minimum on the LLM fit score, applied independently of
// recency. Without it, a brand-new but poor-fit job (high recency, low fit)
// could clear scoreThreshold purely on freshness. Only enforced when scoring
// actually ran (RecencyWeight < 100).
const fitFloor = 0.4

// scoringDescriptionChars caps the description text sent to the LLM. We spend it
// on the requirements section (see scoringExcerpt), not the opening boilerplate.
const scoringDescriptionChars = 400

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

	// Per-job fit + reason. Skip the LLM entirely when recency is weighted 100%.
	scored := exec.RecencyWeight < 100
	results := map[string]scoreResult{}
	if scored {
		var err error
		results, err = sj.scoreFitBatched(ctx, buildProfile(exec), jobs)
		if err != nil {
			// Fail loudly. Returning nothing here would look identical to
			// "no jobs matched", which is the opposite of what happened.
			return nil, fmt.Errorf("score_jobs: %w", err)
		}
	}

	type rankedJob struct {
		job     models.Job
		fit     float64
		recency float64
		final   float64
		reason  string
	}
	ranked := make([]rankedJob, 0, len(jobs))
	for _, job := range jobs {
		res := results[job.JobID]
		r := recencyScore(job.PostedAt, now)
		ranked = append(ranked, rankedJob{job: job, fit: res.fit, recency: r, final: w*r + (1-w)*res.fit, reason: res.reason})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].final > ranked[j].final })

	// Drop weak matches entirely (better to show a few great ones than pad). The
	// fit floor stops a fresh-but-poor-fit job from clearing the blended threshold
	// on recency alone.
	var dropThreshold, dropFit, dropLevel, dropUnscored int
	kept := make([]rankedJob, 0, len(ranked))
	for _, rj := range ranked {
		res, wasScored := results[rj.job.JobID]
		// A job the scorer never returned has no fit signal; drop it rather than
		// surfacing an unjudged match, but count it separately so a systematic
		// scoring gap is visible in the logs instead of looking like "no matches".
		if scored && !wasScored {
			dropUnscored++
			continue
		}
		if rj.final < scoreThreshold {
			dropThreshold++
			continue
		}
		if scored && rj.fit < fitFloor {
			dropFit++
			continue
		}
		// Level backstop: drop roles the scorer itself read as too senior, even
		// when they score well on skills. This is what the regex can't catch.
		if scored && !passesLevelBackstop(res, exec.Level) {
			dropLevel++
			slog.Info("score_jobs: dropped over-senior job",
				"title", rj.job.Title, "target_level", exec.Level,
				"job_seniority", res.seniority, "min_years", res.minYears)
			continue
		}
		kept = append(kept, rj)
	}
	slog.Info("score_jobs: results",
		"candidates", len(ranked), "kept", len(kept),
		"dropped_unscored", dropUnscored, "dropped_low_score", dropThreshold,
		"dropped_low_fit", dropFit, "dropped_over_level", dropLevel)
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
		m["reason"] = rj.reason
		out[i] = m
	}
	return map[string]any{"ranked_jobs": out}, nil
}

// buildProfile renders the candidate's structured profile: the keywords, level,
// and years the user reviewed at creation time. This replaces dumping raw résumé
// text into the prompt — résumé text is long, noisy, and (for many PDFs) badly
// extracted, whereas the profile is compact, user-corrected, and exactly the
// signal we want matched.
func buildProfile(exec ExecutionContext) string {
	var b strings.Builder
	if exec.Category != "" {
		b.WriteString("Field: " + exec.Category + "\n")
	}
	if len(exec.Keywords) > 0 {
		b.WriteString("Skills and roles to match on: " + strings.Join(exec.Keywords, ", ") + "\n")
	}
	if exec.YearsExperience > 0 {
		b.WriteString(fmt.Sprintf("Years of professional experience: %d\n", exec.YearsExperience))
	}
	if exec.Level != "" && exec.Level != LevelAny {
		b.WriteString("Target seniority: " + exec.Level + "\n")
	}
	if strings.TrimSpace(exec.LocationPref) != "" {
		b.WriteString("Preferred location: " + exec.LocationPref + "\n")
	}
	if strings.TrimSpace(exec.Instructions) != "" {
		b.WriteString("Additional notes: " + exec.Instructions + "\n")
	}
	return strings.TrimSpace(b.String())
}

// passesLevelBackstop rejects jobs the LLM itself reports as above the worker's
// target level. The deterministic filter in job_search runs first and is the
// cheap gate (explicit senior/staff titles, hard "N+ years" numbers); this is the
// safety net for what a regex fundamentally can't read — "seasoned", "you'll
// mentor the team", seniority implied by scope. Costs no extra LLM call: these
// two fields ride along in the JSON the scorer already returns.
func passesLevelBackstop(res scoreResult, level string) bool {
	targetRank, ok := LevelRank(level)
	if !ok {
		return true // "any" or unset: no level enforcement
	}
	if r, known := LevelRank(res.seniority); known && r > targetRank {
		return false
	}
	if ceiling, has := LevelYearsCeiling(level); has && res.minYears > ceiling {
		return false
	}
	return true
}

func truncateChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// requirementMarkers locate the part of a posting that actually carries matching
// signal. The opening of a job description is almost always company boilerplate
// ("ABOUT ACME: Since 2018 we've been transforming…"), so sending the first N
// characters spends tokens on marketing copy and hides the requirements — which
// is what we need to judge both fit and seniority.
var requirementMarkers = regexp.MustCompile(`(?i)(what we'?re looking for|what you'?ll bring|what you bring|requirements|qualifications|about you|who you are|your experience|you have|skills? (and|&) experience|years of experience)`)

// scoringExcerpt returns the most informative n characters of a description:
// the requirements section when we can find it, otherwise the opening.
func scoringExcerpt(desc string, n int) string {
	if loc := requirementMarkers.FindStringIndex(desc); loc != nil {
		return truncateChars(desc[loc[0]:], n)
	}
	return truncateChars(desc, n)
}

// isRateLimited reports whether a provider error is a rate or quota limit.
// Matched by string because this package depends only on the narrow Completer
// interface, not on any provider SDK.
func isRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "429") ||
		strings.Contains(s, "rate limit") ||
		strings.Contains(s, "rate_limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "quota") ||
		strings.Contains(s, "tokens per day") ||
		strings.Contains(s, "tpd")
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

type scoreResult struct {
	fit       float64
	reason    string
	seniority string // level the LLM read off the posting (may be "")
	minYears  int    // years of experience the posting requires (0 = unstated)
}

// scoreFitBatched scores up to scoreBudget jobs (most recent) in batches sized to
// fit the provider's per-request limit. It fails fast: the first batch error
// aborts the whole scoring pass and returns — no partial "success" that the fit
// floor would silently turn into zero results, and no inline retry. A rate-limit
// error is wrapped as ErrRateLimited so the executor knows to fail cleanly and
// let the schedule retry.
func (sj *ScoreJobs) scoreFitBatched(ctx context.Context, profile string, jobs []models.Job) (map[string]scoreResult, error) {
	jobs = mostRecent(jobs, scoreBudget)
	fit := make(map[string]scoreResult, len(jobs))

	for i := 0; i < len(jobs); i += batchSize {
		end := min(i+batchSize, len(jobs))
		scores, err := sj.scoreFit(ctx, profile, jobs[i:end])
		if err != nil {
			if isRateLimited(err) {
				return nil, fmt.Errorf("%w: %v", ErrRateLimited, err)
			}
			return nil, fmt.Errorf("scoring batch at %d failed: %w", i, err)
		}
		for k, v := range scores {
			fit[k] = v
		}
	}

	slog.Info("score_jobs: scoring complete", "jobs", len(jobs), "scored", len(fit))
	return fit, nil
}

// scoreFit asks the LLM to score each job against the candidate's structured
// profile, and to report the seniority and required years it reads off the
// posting so code can enforce the level gate deterministically.
func (sj *ScoreJobs) scoreFit(ctx context.Context, profile string, jobs []models.Job) (map[string]scoreResult, error) {
	// Jobs are referenced by a short 1-based index, not their real job_id. ATS
	// ids are long UUIDs and models frequently mangle or abbreviate them when
	// echoing; a mismatched key means the job silently scores 0 and gets dropped
	// by the fit floor. An integer index is unambiguous and cheaper in tokens.
	type brief struct {
		Ref         int    `json:"ref"`
		Title       string `json:"title"`
		Company     string `json:"company"`
		Location    string `json:"location"`
		Description string `json:"description"`
	}
	briefs := make([]brief, len(jobs))
	for i, j := range jobs {
		briefs[i] = brief{
			Ref:         i + 1,
			Title:       j.Title,
			Company:     j.Company,
			Location:    j.Location,
			Description: scoringExcerpt(j.Description, scoringDescriptionChars),
		}
	}
	jobsJSON, _ := json.Marshal(briefs)

	if strings.TrimSpace(profile) == "" {
		profile = "(not specified)"
	}

	system := `You are a strict job-matching assistant. For each job, read its title and description and return four things.

1. "score" (0-100): how well the job matches the candidate's profile — their listed skills/roles, field, and seniority.
   - Reserve 70+ for genuinely strong matches in the right role, level, and domain.
   - Penalize wrong role, wrong domain, or a location conflicting with the stated preference.
   - Judge against the profile's skills and roles, not just the job title.
2. "seniority": the level THIS JOB requires, judged from the whole posting (scope, expectations, phrases like "seasoned", "you'll mentor the team", "own the roadmap"), EXACTLY one of: intern, junior, mid, senior, staff_plus. Report what the job actually demands, even when the title doesn't say so.
3. "min_years": minimum years of experience the posting requires, as an integer. Use 0 if it is not stated.
4. "reason": one short phrase (max 12 words) on why it fits the candidate.

Be accurate on "seniority" and "min_years" — they are used to filter, so do not soften them to be helpful.
Return one object per job, echoing its "ref" number exactly as given.
Respond ONLY with a JSON array of objects {"ref": number, "score": number, "seniority": string, "min_years": number, "reason": string}. No markdown.`
	user := fmt.Sprintf("Candidate profile:\n%s\n\nJobs to score:\n%s\n\nReturn the JSON array now.", profile, jobsJSON)

	raw, err := sj.llm.Complete(ctx, system, user)
	if err != nil {
		return nil, err
	}

	var scores []struct {
		Ref       int     `json:"ref"`
		Score     float64 `json:"score"`
		Seniority string  `json:"seniority"`
		MinYears  int     `json:"min_years"`
		Reason    string  `json:"reason"`
	}
	if err := json.Unmarshal([]byte(stripCodeFences(raw)), &scores); err != nil {
		return nil, fmt.Errorf("parse scores: %w, raw: %s", err, truncateChars(raw, 400))
	}

	// Map refs back to real job ids, ignoring any the model invented.
	out := make(map[string]scoreResult, len(scores))
	for _, s := range scores {
		if s.Ref < 1 || s.Ref > len(jobs) {
			slog.Warn("score_jobs: model returned an out-of-range ref", "ref", s.Ref, "batch_size", len(jobs))
			continue
		}
		out[jobs[s.Ref-1].JobID] = scoreResult{
			fit:       clamp01(s.Score / 100.0),
			reason:    s.Reason,
			seniority: strings.ToLower(strings.TrimSpace(s.Seniority)),
			minYears:  s.MinYears,
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable scores parsed from response: %s", truncateChars(raw, 400))
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
