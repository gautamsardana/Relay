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
	return `Searches curated company job boards (Greenhouse, Lever, Ashby) and returns only NEW
matches for this worker (jobs shown in past runs are filtered out). It filters by the worker's
configured category (against ATS department metadata) and keywords, and takes no input.
Output: {"jobs": [{"company": "Stripe", "title": "Backend Engineer", "url": "...", "location": "Remote", "department": "Engineering", "posted_at": "...", "company_id": "stripe", "job_id": "123", "ats": "greenhouse"}]}
Use this as the first step of a job hunt, then pass its output to score_jobs.`
}

func (j *JobSearch) Execute(ctx context.Context, input map[string]any, exec ExecutionContext) (map[string]any, error) {
	companies, err := j.store.ListActiveCompanies(ctx)
	if err != nil {
		return nil, fmt.Errorf("job_search: list companies: %w", err)
	}

	allJobs := j.fetchAll(ctx, companies)
	slog.Info("job_search: fetched jobs", "companies", len(companies), "jobs", len(allJobs))

	matched := filterJobs(allJobs, exec.Category, exec.Keywords, exec.LocationPref, exec.Level)

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

// filterJobs keeps jobs matching the worker's category (against ATS department/
// team metadata) AND its keywords (whole-word over title+description). Each
// filter is applied only when set. Whole-word matching avoids substring false
// positives (e.g. "go" matching "logo"); searching the description (not just the
// title) is what stops us dropping relevant roles whose titles are generic
// (e.g. "Backend Engineer" whose description says "Go"). This is a recall net;
// precision is the scorer's job.
// Level (experience) and location are hard filters: a role above the seeker's
// level or outside their location preference is removed here, before scoring, so
// recency can never float it back up. See levels.go for the rules.
func filterJobs(jobs []models.Job, category string, keywords []string, locationPref, level string) []models.Job {
	kwMatchers := compileKeywordMatchers(keywords)
	out := make([]models.Job, 0, len(jobs))
	for _, job := range jobs {
		if !passesLevel(job.Title, job.Description, level) {
			continue
		}
		if !matchesLocation(job.Location, locationPref) {
			continue
		}
		if !matchesCategory(job, category) {
			continue
		}
		if len(kwMatchers) > 0 && !matchesAny(kwMatchers, job.Title+" "+job.Description) {
			continue
		}
		out = append(out, job)
	}
	return out
}

func matchesAny(matchers []*regexp.Regexp, haystack string) bool {
	for _, re := range matchers {
		if re.MatchString(haystack) {
			return true
		}
	}
	return false
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
