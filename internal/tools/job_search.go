package tools

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/gautamsardana/relay/internal/models"
	"github.com/gautamsardana/relay/internal/store"
	"github.com/gautamsardana/relay/internal/tools/ats"
)

// fetchConcurrency caps how many company boards we hit at once.
const fetchConcurrency = 10

type JobSearch struct {
	store    *store.Store
	adapters map[string]ats.Adapter
}

func NewJobSearch(s *store.Store) *JobSearch {
	return &JobSearch{
		store:    s,
		adapters: ats.NewAdapters(),
	}
}

func (j *JobSearch) Name() string { return "job_search" }

func (j *JobSearch) Description() string {
	return `Searches curated company job boards (Greenhouse, Lever, Ashby) for open roles and
returns only NEW matches for this worker — jobs already shown in past runs are filtered out automatically.
Input: {"role_keywords": ["backend", "golang", "infrastructure"]}
  - role_keywords: list of terms matched against job titles (case-insensitive). Omit to return all open roles.
Output: {"jobs": [{"company": "Stripe", "title": "Backend Engineer", "url": "...", "location": "Remote", "posted_at": "2026-06-10T...", "company_id": "stripe", "job_id": "123", "ats": "greenhouse"}]}
Use this as the first step of a job hunt, then pass its output to score_jobs to rank the results.`
}

func (j *JobSearch) Execute(ctx context.Context, input map[string]any, exec ExecutionContext) (map[string]any, error) {
	keywords := parseKeywords(input["role_keywords"])

	companies, err := j.store.ListActiveCompanies(ctx)
	if err != nil {
		return nil, fmt.Errorf("job_search: list companies: %w", err)
	}

	allJobs := j.fetchAll(ctx, companies)
	slog.Info("job_search: fetched jobs", "companies", len(companies), "jobs", len(allJobs))

	matched := filterByKeywords(allJobs, keywords)

	seen, err := j.store.ListSeenJobKeys(ctx, exec.WorkerID)
	if err != nil {
		return nil, fmt.Errorf("job_search: list seen jobs: %w", err)
	}
	newJobs := dropSeen(matched, seen)
	slog.Info("job_search: filtered", "matched", len(matched), "new", len(newJobs))

	return map[string]any{"jobs": jobsToMaps(newJobs)}, nil
}

// fetchAll fans out across company boards concurrently (bounded). A failed board
// is logged and skipped, never fatal to the run.
func (j *JobSearch) fetchAll(ctx context.Context, companies []models.Company) []models.Job {
	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []models.Job

	for _, c := range companies {
		adapter, ok := j.adapters[c.ATS]
		if !ok {
			slog.Warn("job_search: no adapter for ats", "ats", c.ATS, "company", c.Name)
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(c models.Company, adapter ats.Adapter) {
			defer wg.Done()
			defer func() { <-sem }()

			jobs, err := adapter.Fetch(ctx, c.Slug, c.Name)
			if err != nil {
				slog.Warn("job_search: board fetch failed", "company", c.Name, "ats", c.ATS, "error", err)
				return
			}
			mu.Lock()
			all = append(all, jobs...)
			mu.Unlock()
		}(c, adapter)
	}

	wg.Wait()
	return all
}

// filterByKeywords keeps jobs where any keyword appears as a whole word in the
// title OR description. Whole-word matching avoids substring false positives
// (e.g. "go" matching "logo"), and searching the description (not just the
// title) is what stops us dropping relevant roles whose titles are generic
// (e.g. "Backend Engineer" whose description says "Go"). This is a recall net;
// precision is the scorer's job.
func filterByKeywords(jobs []models.Job, keywords []string) []models.Job {
	matchers := compileKeywordMatchers(keywords)
	if len(matchers) == 0 {
		return jobs
	}

	out := make([]models.Job, 0, len(jobs))
	for _, job := range jobs {
		haystack := job.Title + " " + job.Description
		for _, re := range matchers {
			if re.MatchString(haystack) {
				out = append(out, job)
				break
			}
		}
	}
	return out
}

func compileKeywordMatchers(keywords []string) []*regexp.Regexp {
	matchers := make([]*regexp.Regexp, 0, len(keywords))
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		// (?i) case-insensitive, \b word boundaries for whole-word matching.
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
		if err != nil {
			continue
		}
		matchers = append(matchers, re)
	}
	return matchers
}

func dropSeen(jobs []models.Job, seen store.SeenJobSet) []models.Job {
	out := make([]models.Job, 0, len(jobs))
	for _, job := range jobs {
		if seen.Has(job.CompanyID, job.JobID) {
			continue
		}
		out = append(out, job)
	}
	return out
}

// parseKeywords accepts the LLM-supplied role_keywords as a JSON array, a Go
// []string, or a comma-separated string.
func parseKeywords(v any) []string {
	switch val := v.(type) {
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		return val
	case string:
		var out []string
		for _, s := range strings.Split(val, ",") {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}
