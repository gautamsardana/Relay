`# Structured Job Matching + Worker Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace fragile free-text/LLM-keyword job matching with structured, reliable matching (user-chosen category mapped to ATS department metadata + explicit keywords), make the job-hunt run deterministic, switch résumé input from pasted text to PDF upload (parsed server-side) with AI pre-fill of category/keywords, and add pause/resume + delete for workers.

**Architecture:** A worker now carries a `category` (mapped to a curated department-synonym set) and optional `keywords`. `job_search` filters live ATS results by category (against the structured `department`/`team` fields the ATS returns, which we currently discard) and keywords (whole-word over title+description), reading these from the worker config via `ExecutionContext` rather than from LLM-generated input. The job-hunt run becomes a deterministic two-step pipeline (`job_search → score_jobs`), removing the LLM planner from the path entirely (which is the root cause of the wrong-tool / resolver-crash bugs). The LLM is kept for semantic scoring and for a one-time résumé classification: at create time the user uploads a PDF résumé, the server extracts its text and asks the LLM to suggest a category + keywords to pre-fill the form. The extracted text is still stored in `resume_text` (column unchanged); the textarea is gone. Worker lifecycle adds pause/resume and soft delete (archive + hide from the list), all through a single status endpoint — no rows are ever destroyed.

**Tech Stack:** Go, sqlc, PostgreSQL, RabbitMQ, `github.com/ledongthuc/pdf` (PDF text extraction), vanilla-JS frontend (ES modules).

---

## Why this fixes the reported issues

- **Inconsistent plans / `web_search → document_read` / resolver crash (`results[0]` not found):** root cause is the LLM freely assembling tool plans. Part D makes job-hunt runs a fixed `job_search → score_jobs` pipeline, so the LLM never plans tools and the crash path disappears.
- **Bad results (golang search → social-market jobs, best 48):** root cause is title-substring keyword matching dropping good roles and admitting junk. Parts B+C filter on structured ATS department metadata + whole-word keywords over title+description, and scoring weighs the explicit category/keywords.
- **No way to stop/delete a worker:** Part A.

## File map

- `internal/migrations/02_create_workers.sql` — add `category`, `keywords` columns
- `internal/store/queries/workers.sql` — category/keywords in CRUD; `ListWorkersByUser` hides archived
- `internal/models/worker.go` — `Category string`, `Keywords []string`
- `internal/models/job.go` — `Department`, `Team`
- `internal/store/adapter.go` — worker category/keywords conversion (+ csv helpers)
- `internal/tools/categories.go` (new) — category constants + department synonym map + `matchesCategory`
- `internal/tools/job_search.go` — structured filter reading from `ExecutionContext`
- `internal/tools/score_jobs.go` — intent string from category/keywords/instructions
- `internal/tools/registry.go` — `ExecutionContext` gains `Category`, `Keywords`
- `internal/tools/ats/{greenhouse,lever,ashby}.go` — capture department/team
- `internal/tools/jobs.go` — department/team in wire shape
- `internal/executor/worker.go` — populate new `ExecutionContext` fields
- `internal/planner/run.go` — deterministic pipeline (no `GeneratePlan`)
- `internal/planner/worker.go`, `internal/api/handler.go`, `internal/api/server.go` — category/keywords + status (pause/resume/archive)
- `internal/resume/parse.go` (new) — PDF text extraction (`ledongthuc/pdf`)
- `internal/planner/resume.go` (new) — `ParseResume` + LLM category/keyword suggestion
- `internal/tools/categories.go` — also exposes `Categories()` for the suggestion prompt
- `web/js/api.js`, `web/js/views/workerNew.js`, `web/js/views/worker.js`, `web/styles.css` — form fields, PDF upload + pre-fill, lifecycle buttons

---

## Part A — Worker structured fields + lifecycle

### Task A1: Migration (worker columns)

**Files:**
- Modify: `internal/migrations/02_create_workers.sql`

- [ ] **Step 1: Add columns to workers.** In `02_create_workers.sql`, after the `recency_weight` line add:

```sql
    category     TEXT NOT NULL DEFAULT '',
    keywords     TEXT NOT NULL DEFAULT '',
```

(No FK cascade is needed: "delete" is a soft delete that sets `status = 'archived'`, so worker rows and their run history are never removed.)

- [ ] **Step 2: Re-apply schema to the dev DB** (drops/recreates; this is pre-prod). Verify the columns exist:

Run: `psql "$DATABASE_URL" -c "\d workers" | grep -E "category|keywords"`
Expected: both columns listed.

- [ ] **Step 3: Commit**

```bash
git add internal/migrations/02_create_workers.sql
git commit -m "feat(db): worker category/keywords columns"
```

### Task A2: Worker model + queries + sqlc

**Files:**
- Modify: `internal/models/worker.go`
- Modify: `internal/store/queries/workers.sql`

- [ ] **Step 1: Add fields to the model.** In `internal/models/worker.go`, in the `Worker` struct after `Instructions`:

```go
	Category        string
	Keywords        []string
```

- [ ] **Step 2: Update queries.** In `internal/store/queries/workers.sql`, add `category, keywords` to every column list and to `CreateWorker` ($10, $11). The `CreateWorker` insert becomes:

```sql
-- name: CreateWorker :one
INSERT INTO workers (worker_id, user_id, name, instructions, interval_seconds, status, resume_text, recency_weight, next_run_at, category, keywords)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING worker_id, user_id, name, instructions, interval_seconds, status, resume_text, recency_weight, next_run_at, created_at, updated_at, category, keywords;
```

Add the same `category, keywords` to the SELECT lists in `GetWorkerByID`, `ListWorkersByUser`, `ListDueWorkers`. Also change `ListWorkersByUser` to hide archived workers:

```sql
-- name: ListWorkersByUser :many
SELECT worker_id, user_id, name, instructions, interval_seconds, status, resume_text, recency_weight, next_run_at, created_at, updated_at, category, keywords
FROM workers
WHERE user_id = $1 AND status != 'archived'
ORDER BY created_at DESC;
```

(No delete query — "delete" archives via the existing `UpdateWorkerStatus`. `ListDueWorkers` already filters `status = 'active'`, so paused and archived workers never run.)

- [ ] **Step 3: Regenerate sqlc.**

Run: `sqlc generate`
Expected: no error; `internal/store/sqlc/workers.sql.go` now has `Category` and `Keywords` (both `string`).

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(store): worker category/keywords queries; hide archived"`

### Task A3: Adapter (csv <-> []string)

**Files:**
- Modify: `internal/store/adapter.go`
- Test: `internal/store/adapter_test.go` (new)

- [ ] **Step 1: Write the failing test** for the csv helpers. Create `internal/store/adapter_test.go`:

```go
package store

import (
	"reflect"
	"testing"
)

func TestCSVRoundTrip(t *testing.T) {
	cases := [][]string{
		{},
		{"golang"},
		{"golang", "kubernetes"},
	}
	for _, c := range cases {
		got := splitCSV(joinCSV(c))
		if len(c) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c) {
			t.Fatalf("round trip: in=%v out=%v", c, got)
		}
	}
	if got := splitCSV("a, b ,,c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("splitCSV trim/empty handling: %v", got)
	}
}
```

- [ ] **Step 2: Run it, expect FAIL** (`splitCSV`/`joinCSV` undefined).

Run: `go test ./internal/store/ -run TestCSVRoundTrip`

- [ ] **Step 3: Implement.** In `internal/store/adapter.go` add helpers and wire into the worker converters:

```go
func joinCSV(items []string) string {
	return strings.Join(items, ",")
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
```

Add `"strings"` to the import block. In `toModelWorker`, add to the returned struct: `Category: sw.Category,` and `Keywords: splitCSV(sw.Keywords),`. In `fromModelWorkerCreate`, add: `Category: mw.Category,` and `Keywords: joinCSV(mw.Keywords),`.

- [ ] **Step 4: Run tests + build.**

Run: `go test ./internal/store/ -run TestCSVRoundTrip && go build ./...`
Expected: PASS, build OK.

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(store): worker csv adapter"`

### Task A4: Planner methods

**Files:**
- Modify: `internal/planner/worker.go`

- [ ] **Step 1: Extend `CreateWorker`.** Change the signature and body to accept `category` and `keywords`, validating the category against the known set (defined in Task C1, `tools.IsValidCategory`):

```go
func (p *Planner) CreateWorker(ctx context.Context, userID, name, instructions string, intervalSeconds int, resumeText string, recencyWeight int, category string, keywords []string) (models.Worker, error) {
	if intervalSeconds < minIntervalSeconds {
		return models.Worker{}, fmt.Errorf("interval must be at least %d seconds (1 hour), got %d", minIntervalSeconds, intervalSeconds)
	}
	if recencyWeight < 0 || recencyWeight > 100 {
		return models.Worker{}, fmt.Errorf("recency_weight must be between 0 and 100, got %d", recencyWeight)
	}
	if category != "" && !tools.IsValidCategory(category) {
		return models.Worker{}, fmt.Errorf("unknown category: %q", category)
	}

	id, _ := uuid.NewV7()
	nextRunAt := time.Now().Add(time.Duration(intervalSeconds) * time.Second)

	worker := &models.Worker{
		WorkerID:        id.String(),
		UserID:          userID,
		Name:            name,
		Instructions:    instructions,
		IntervalSeconds: intervalSeconds,
		Status:          models.WorkerStatusActive,
		ResumeText:      resumeText,
		RecencyWeight:   recencyWeight,
		Category:        category,
		Keywords:        keywords,
		NextRunAt:       &nextRunAt,
	}
	return p.store.CreateWorker(ctx, worker)
}
```

Add the `tools` import: `"github.com/gautamsardana/relay/internal/tools"`.

- [ ] **Step 2: Add a status passthrough** in `internal/planner/worker.go` (used for pause, resume, and archive/delete). If an `UpdateWorkerStatus` passthrough already exists, reuse it and skip this:

```go
func (p *Planner) SetWorkerStatus(ctx context.Context, workerID string, status models.WorkerStatus) error {
	return p.store.UpdateWorkerStatus(ctx, workerID, status)
}
```

(No `DeleteWorker` method: "delete" is `SetWorkerStatus(..., archived)`, and `ListWorkersByUser` hides archived workers.)

- [ ] **Step 3: Build.** `go build ./...` Expected: fails only in `api/handler.go` (CreateWorker arity) — fixed in Task A5. Confirm the planner package compiles in isolation: `go build ./internal/planner/`.

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(planner): worker category/keywords + status"`

### Task A5: API handlers + routes

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Extend `CreateWorker` handler.** In `internal/api/handler.go`, update the request struct and call:

```go
	var req struct {
		UserID        string   `json:"user_id"`
		Name          string   `json:"name"`
		Instructions  string   `json:"instructions"`
		IntervalHours int      `json:"interval_hours"`
		ResumeText    string   `json:"resume_text"`
		RecencyWeight *int     `json:"recency_weight"`
		Category      string   `json:"category"`
		Keywords      []string `json:"keywords"`
	}
```

After decoding and the `recencyWeight` defaulting:

```go
	worker, err := s.planner.CreateWorker(r.Context(), req.UserID, req.Name, req.Instructions, req.IntervalHours*3600, req.ResumeText, recencyWeight, req.Category, req.Keywords)
```

- [ ] **Step 2: Add the status handler** in `internal/api/handler.go` (pause/resume/archive all go through this; "delete" sends `status: "archived"`):

```go
func (s *Server) SetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	workerID := chi.URLParam(r, "id")
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	status := models.WorkerStatus(req.Status)
	if status != models.WorkerStatusActive && status != models.WorkerStatusPaused && status != models.WorkerStatusArchived {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := s.planner.SetWorkerStatus(r.Context(), workerID, status); err != nil {
		slog.Error("api/SetWorkerStatus", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add `"github.com/gautamsardana/relay/internal/models"` to the handler imports.

- [ ] **Step 3: Route.** In `internal/api/server.go`, after `r.Get("/workers/{id}", s.GetWorker)`:

```go
	r.Patch("/workers/{id}/status", s.SetWorkerStatus)
```

- [ ] **Step 4: Build + vet.** `go build ./... && go vet ./...` Expected: OK.

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(api): worker create category/keywords + status route"`

---

## Part B — Capture ATS department/team

### Task B1: Job model fields

**Files:**
- Modify: `internal/models/job.go`

- [ ] **Step 1:** Add to the `Job` struct after `Location`:

```go
	Department  string
	Team        string
```

- [ ] **Step 2: Build.** `go build ./...` Expected: OK.

### Task B2: Adapters populate department/team

**Files:**
- Modify: `internal/tools/ats/greenhouse.go`, `lever.go`, `ashby.go`
- Test: `internal/tools/ats/adapters_dept_test.go` (new)

Field sources (verified against live APIs):
- Greenhouse: `departments[]` array of `{name}` → use first name.
- Lever: `categories.department`, `categories.team`.
- Ashby: top-level `department`, `team`.

- [ ] **Step 1: Greenhouse.** Add a `Departments` field to the per-job anonymous struct:

```go
			Departments []struct {
				Name string `json:"name"`
			} `json:"departments"`
```

Then extract it inline in the loop, before constructing the `models.Job`:

```go
		dept := ""
		if len(j.Departments) > 0 {
			dept = j.Departments[0].Name
		}
```

and set `Department: dept,` on the Job. (Greenhouse has no team concept; leave `Team` empty.)

- [ ] **Step 2: Lever.** Extend `Categories` to include `Department string` and `Team string`; set `Department: p.Categories.Department, Team: p.Categories.Team` on the Job.

- [ ] **Step 3: Ashby.** Add `Department string` and `Team string` to the per-job struct; set `Department: j.Department, Team: j.Team` on the Job.

- [ ] **Step 4: Write a parse test** at `internal/tools/ats/adapters_dept_test.go` that feeds each adapter a small mock server with a representative payload and asserts `Department`/`Team` populate. Use `httptest.NewServer`, point the adapter's `baseURL` at it (construct the struct directly, e.g. `&greenhouseAdapter{client: srv.Client(), baseURL: srv.URL}`). Example for Greenhouse:

```go
func TestGreenhouseDepartment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jobs":[{"id":1,"title":"Backend Engineer","absolute_url":"u","first_published":"2026-06-01T00:00:00Z","departments":[{"name":"Engineering"}],"location":{"name":"Remote"}}]}`))
	}))
	defer srv.Close()
	a := &greenhouseAdapter{client: srv.Client(), baseURL: srv.URL}
	jobs, err := a.Fetch(context.Background(), "x", "X")
	if err != nil || len(jobs) != 1 || jobs[0].Department != "Engineering" {
		t.Fatalf("got %+v err=%v", jobs, err)
	}
}
```

(Write the Lever and Ashby equivalents with their payload shapes.)

- [ ] **Step 5: Run tests.** `go test ./internal/tools/ats/` Expected: PASS.

- [ ] **Step 6: Commit** `git add -A && git commit -m "feat(ats): capture department/team"`

### Task B3: Wire shape

**Files:**
- Modify: `internal/tools/jobs.go`

- [ ] **Step 1:** Add `"department": j.Department,` and `"team": j.Team,` to `jobToMap`, and `Department: asString(m["department"]),` / `Team: asString(m["team"]),` to `jobFromMap`.

- [ ] **Step 2: Build.** `go build ./...` Expected: OK.

- [ ] **Step 3: Commit** `git add -A && git commit -m "feat(tools): department/team in job wire shape"`

---

## Part C — Structured matching

### Task C1: Categories + department synonym map

**Files:**
- Create: `internal/tools/categories.go`
- Test: `internal/tools/categories_test.go`

- [ ] **Step 1: Write the failing test** `internal/tools/categories_test.go`:

```go
package tools

import (
	"testing"

	"github.com/gautamsardana/relay/internal/models"
)

func TestMatchesCategory(t *testing.T) {
	eng := models.Job{Title: "Backend Engineer", Department: "Engineering"}
	design := models.Job{Title: "Product Designer", Department: "Design"}
	noDept := models.Job{Title: "Senior Data Scientist"}

	if !matchesCategory(eng, "software_engineering") {
		t.Fatal("engineering dept should match software_engineering")
	}
	if matchesCategory(design, "software_engineering") {
		t.Fatal("design dept must not match software_engineering")
	}
	if !matchesCategory(design, "") {
		t.Fatal("empty category should match all")
	}
	// fallback to title when no department/team
	if !matchesCategory(noDept, "data") {
		t.Fatal("data category should match via title fallback")
	}
	if !IsValidCategory("software_engineering") || IsValidCategory("nonsense") {
		t.Fatal("IsValidCategory wrong")
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (`matchesCategory`, `IsValidCategory` undefined). `go test ./internal/tools/ -run TestMatchesCategory`

- [ ] **Step 3: Implement** `internal/tools/categories.go`:

```go
package tools

import (
	"strings"

	"github.com/gautamsardana/relay/internal/models"
)

// categoryDepartments maps a worker category to department/team name fragments
// (matched whole-word, case-insensitive) found in ATS metadata. Tunable; built
// from the department names observed across the catalog.
var categoryDepartments = map[string][]string{
	"software_engineering": {"engineering", "software", "developer", "infrastructure", "platform", "backend", "frontend", "full stack", "fullstack", "sre", "devops", "security", "mobile", "systems"},
	"data":                 {"data", "machine learning", "ml", "analytics", "data science"},
	"design":               {"design", "ux", "ui", "creative"},
	"product":              {"product management", "product manager", "program management", "product operations"},
	"marketing":            {"marketing", "growth", "brand", "content", "communications"},
	"sales":                {"sales", "account executive", "business development", "revenue", "partnerships"},
	"finance":              {"finance", "accounting", "treasury", "audit"},
	"operations":           {"operations", "people", "human resources", "recruiting", "talent", "legal", "support"},
}

// IsValidCategory reports whether c is a known category.
func IsValidCategory(c string) bool {
	_, ok := categoryDepartments[c]
	return ok
}

// matchesCategory reports whether a job belongs to the given category, matching
// the category's synonyms against the job's department+team (whole-word). Empty
// category matches everything. Falls back to the title when the job has no
// department/team metadata.
func matchesCategory(job models.Job, category string) bool {
	if category == "" {
		return true
	}
	syns := categoryDepartments[category]
	if len(syns) == 0 {
		return true
	}
	matchers := compileKeywordMatchers(syns)

	hay := strings.TrimSpace(job.Department + " " + job.Team)
	if hay == "" {
		hay = job.Title
	}
	for _, re := range matchers {
		if re.MatchString(hay) {
			return true
		}
	}
	return false
}
```

(`compileKeywordMatchers` already exists in `job_search.go`.)

- [ ] **Step 4: Run, expect PASS.** `go test ./internal/tools/ -run TestMatchesCategory`

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(tools): category->department synonym matching"`

### Task C2: ExecutionContext + executor

**Files:**
- Modify: `internal/tools/registry.go`
- Modify: `internal/executor/worker.go`

- [ ] **Step 1: Extend `ExecutionContext`** (in `registry.go`) after `Instructions`:

```go
    Category      string
    Keywords      []string
```

- [ ] **Step 2: Populate it** in `internal/executor/worker.go` `buildExecutionContext`, add to the returned struct:

```go
		Category:      worker.Category,
		Keywords:      worker.Keywords,
```

- [ ] **Step 3: Build.** `go build ./...` Expected: OK.

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(tools): category/keywords in ExecutionContext"`

### Task C3: job_search structured filter

**Files:**
- Modify: `internal/tools/job_search.go`
- Test: `internal/tools/jobs_logic_test.go`

- [ ] **Step 1: Write the failing test** (add to `jobs_logic_test.go`):

```go
func TestFilterJobs(t *testing.T) {
	jobs := []models.Job{
		{JobID: "1", Title: "Backend Engineer", Department: "Engineering", Description: "We use Go."},
		{JobID: "2", Title: "Brand Designer", Department: "Design", Description: "Figma."},
		{JobID: "3", Title: "Platform Engineer", Department: "Engineering", Description: "Rust, k8s."},
	}
	contains := func(js []models.Job, id string) bool {
		for _, j := range js {
			if j.JobID == id {
				return true
			}
		}
		return false
	}
	// category only
	got := filterJobs(jobs, "software_engineering", nil)
	if len(got) != 2 || !contains(got, "1") || !contains(got, "3") {
		t.Fatalf("category filter: %+v", got)
	}
	// category + keyword (must be eng AND mention go)
	got = filterJobs(jobs, "software_engineering", []string{"go"})
	if len(got) != 1 || !contains(got, "1") {
		t.Fatalf("category+keyword filter: %+v", got)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (`filterJobs` undefined). `go test ./internal/tools/ -run TestFilterJobs`

- [ ] **Step 3: Implement.** In `internal/tools/job_search.go`, replace the body of `Execute`'s filtering and the `filterByKeywords` function with `filterJobs`:

```go
func filterJobs(jobs []models.Job, category string, keywords []string) []models.Job {
	kwMatchers := compileKeywordMatchers(keywords)
	out := make([]models.Job, 0, len(jobs))
	for _, job := range jobs {
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
```

Delete the old `filterByKeywords` (and update `TestFilterByKeywords` to call `filterJobs(jobs, "", keywords)` for the keyword-only cases, or remove it in favor of `TestFilterJobs`). In `Execute`, change the filtering call and read from `exec`:

```go
	matched := filterJobs(allJobs, exec.Category, exec.Keywords)
```

and remove the `parseKeywords(input["role_keywords"])` line (keywords now come from `exec`, not plan input). Update `Description()` to say job_search uses the worker's configured category and keywords (no input needed).

- [ ] **Step 4: Run tests + build.** `go test ./internal/tools/ && go build ./...` Expected: PASS, OK.

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(job_search): structured category+keyword filter from worker config"`

### Task C4: score_jobs intent

**Files:**
- Modify: `internal/tools/score_jobs.go`

- [ ] **Step 1: Compose intent** from category + keywords + instructions. In `Execute`, replace the `scoreFit` call:

```go
		scored, err := sj.scoreFit(ctx, buildIntent(exec), exec.ResumeText, jobsToScore(jobs))
```

and add:

```go
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
```

(`scoreFit`'s first param is already the intent/instructions string from the previous change; no signature change needed.)

- [ ] **Step 2: Build + test.** `go build ./... && go test ./internal/tools/` Expected: OK.

- [ ] **Step 3: Commit** `git add -A && git commit -m "feat(score_jobs): score against category+keywords intent"`

---

## Part D — Deterministic job-hunt pipeline

### Task D1: Replace LLM planning with a fixed two-step run

**Files:**
- Modify: `internal/planner/run.go`

- [ ] **Step 1: Rewrite `HandleRun`** to build `job_search → score_jobs` directly instead of calling `p.agent.GeneratePlan`. The two steps:
  - step 1: tool `job_search`, input `{}` (it reads category/keywords from `ExecutionContext`).
  - step 2: tool `score_jobs`, input `{"jobs": "{{steps[1].output.jobs}}"}`.

Replace the plan-generation block with:

```go
	steps := []agentStep{
		{StepNumber: 1, Tool: "job_search", Description: "Find matching new jobs", Input: map[string]any{}},
		{StepNumber: 2, Tool: "score_jobs", Description: "Rank jobs by fit and recency", Input: map[string]any{"jobs": "{{steps[1].output.jobs}}"}},
	}
```

where `agentStep` is a tiny local struct (`StepNumber int; Tool, Description string; Input map[string]any`) — or reuse the existing plan-step shape already used to build `modelSteps`. Keep the rest of `HandleRun` (building `modelSteps` with new UUIDs and `run.RunID`, `InsertSteps`, `UpdateRunStatus(processing)`, mark first step pending, publish first step) unchanged. Remove the `p.agent.GeneratePlan(...)` call and the tool-validation loop (the two tools are known-good).

- [ ] **Step 2: Build + vet.** `go build ./... && go vet ./...` Expected: OK. (`p.agent` may now be unused in `run.go`; that's fine, it's still used elsewhere — do not remove the field.)

- [ ] **Step 3: Manual end-to-end smoke** (requires stack up): create a worker with `category=software_engineering`, `keywords=["golang"]`, run it, confirm the run shows exactly two steps (`job_search`, `score_jobs`) and produces ranked engineering jobs.

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(planner): deterministic job_search->score_jobs run (no LLM planning)"`

---

## Part E — Résumé PDF upload + AI pre-fill (backend)

The user uploads a PDF résumé instead of pasting text. The server extracts the text, asks the LLM to suggest a category + keywords, and returns all three so the create form can pre-fill. The extracted text is still stored in `resume_text` (column unchanged).

### Task E1: PDF text extraction package

**Files:**
- Create: `internal/resume/parse.go`
- Modify: `go.mod` / `go.sum`

- [ ] **Step 1: Add the dependency.** Run: `go get github.com/ledongthuc/pdf`

- [ ] **Step 2: Implement** `internal/resume/parse.go`:

```go
package resume

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText returns the plain text of a (text-based) PDF. Scanned/image-only
// PDFs yield little or nothing (OCR is out of scope).
func ExtractText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("resume: open pdf: %w", err)
	}
	var sb strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue // skip unreadable pages rather than fail the whole résumé
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String()), nil
}
```

- [ ] **Step 3: Build.** `go build ./internal/resume/` Expected: OK.

- [ ] **Step 4: Manual check** with a real résumé PDF (a throwaway test that reads a local PDF and prints the text). Confirm readable text comes out.

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(resume): pdf text extraction"`

### Task E2: planner ParseResume + AI suggestion

**Files:**
- Create: `internal/planner/resume.go`
- Modify: `internal/tools/categories.go`
- Test: `internal/planner/resume_test.go`

- [ ] **Step 1: Add `Categories()`** to `internal/tools/categories.go` (and `"sort"` to its imports):

```go
// Categories returns the known category keys (sorted).
func Categories() []string {
	out := make([]string, 0, len(categoryDepartments))
	for k := range categoryDepartments {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 2: Write the failing test** `internal/planner/resume_test.go`:

```go
package planner

import (
	"reflect"
	"testing"
)

func TestParseResumeSuggestion(t *testing.T) {
	cat, kw := parseResumeSuggestion("```json\n{\"category\":\"software_engineering\",\"keywords\":[\"golang\",\"kubernetes\"]}\n```")
	if cat != "software_engineering" || !reflect.DeepEqual(kw, []string{"golang", "kubernetes"}) {
		t.Fatalf("got cat=%q kw=%v", cat, kw)
	}
	if c, _ := parseResumeSuggestion(`{"category":"wizardry","keywords":[]}`); c != "" {
		t.Fatalf("unknown category should be dropped, got %q", c)
	}
	if c, k := parseResumeSuggestion("not json"); c != "" || k != nil {
		t.Fatalf("garbage should return empty")
	}
}
```

- [ ] **Step 3: Run, expect FAIL.** `go test ./internal/planner/ -run TestParseResumeSuggestion`

- [ ] **Step 4: Implement** `internal/planner/resume.go`:

```go
package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gautamsardana/relay/internal/resume"
	"github.com/gautamsardana/relay/internal/tools"
)

type ResumeParseResult struct {
	Text     string
	Category string
	Keywords []string
}

// ParseResume extracts text from a PDF résumé and asks the LLM to suggest a
// category + keywords for pre-filling the create form.
func (p *Planner) ParseResume(ctx context.Context, pdfData []byte) (ResumeParseResult, error) {
	text, err := resume.ExtractText(pdfData)
	if err != nil {
		return ResumeParseResult{}, err
	}
	category, keywords := p.suggestFromResume(ctx, text)
	return ResumeParseResult{Text: text, Category: category, Keywords: keywords}, nil
}

func (p *Planner) suggestFromResume(ctx context.Context, text string) (string, []string) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	system := fmt.Sprintf(`You are a career assistant. From the résumé, choose the single best category and up to 5 role/skill keywords. The category MUST be exactly one of: %s. Respond ONLY with JSON {"category": string, "keywords": [string]}. No prose.`, strings.Join(tools.Categories(), ", "))
	raw, err := p.agent.Complete(ctx, system, "Résumé:\n"+text)
	if err != nil {
		return "", nil // best-effort; the form still works without suggestions
	}
	return parseResumeSuggestion(raw)
}

func parseResumeSuggestion(raw string) (string, []string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	var s struct {
		Category string   `json:"category"`
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &s); err != nil {
		return "", nil
	}
	if !tools.IsValidCategory(s.Category) {
		s.Category = ""
	}
	return s.Category, s.Keywords
}
```

- [ ] **Step 5: Run tests + build.** `go test ./internal/planner/ -run TestParseResumeSuggestion && go build ./...` Expected: PASS, OK.

- [ ] **Step 6: Commit** `git add -A && git commit -m "feat(planner): parse résumé pdf + AI suggest category/keywords"`

### Task E3: Parse endpoint

**Files:**
- Modify: `internal/api/handler.go`
- Modify: `internal/api/server.go`

- [ ] **Step 1: Handler** in `internal/api/handler.go` (add `"io"` to imports):

```go
func (s *Server) ParseResume(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB
		http.Error(w, "invalid upload", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("resume")
	if err != nil {
		http.Error(w, "missing 'resume' file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}

	result, err := s.planner.ParseResume(r.Context(), data)
	if err != nil {
		slog.Error("api/ParseResume", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"resume_text":        result.Text,
		"suggested_category": result.Category,
		"suggested_keywords": result.Keywords,
	})
}
```

- [ ] **Step 2: Route** in `internal/api/server.go`, after the `/users` routes:

```go
	r.Post("/resumes/parse", s.ParseResume)
```

- [ ] **Step 3: Build + vet.** `go build ./... && go vet ./...` Expected: OK.

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(api): POST /resumes/parse"`

---

## Part F — Frontend

### Task F1: Create-worker form (category, keywords, PDF résumé with AI pre-fill)

**Files:**
- Modify: `web/js/views/workerNew.js`
- Modify: `web/js/api.js`

- [ ] **Step 1: api.js — `parseResume` (multipart) + `normWorker` fields.** Add a multipart upload method (do NOT route through `request`, which forces a JSON content-type; multipart needs the browser to set the boundary):

```js
  async parseResume(file) {
    const fd = new FormData();
    fd.append("resume", file);
    const res = await fetch("/resumes/parse", { method: "POST", body: fd });
    if (!res.ok) throw new Error((await res.text()) || res.statusText);
    const d = await res.json();
    return {
      resume_text: d.resume_text || "",
      suggested_category: d.suggested_category || "",
      suggested_keywords: d.suggested_keywords || [],
    };
  },
```

Add to `normWorker`: `category: w.Category,` and `keywords: w.Keywords || [],`.

- [ ] **Step 2: Replace the résumé textarea with category + keywords + a PDF picker** in `workerNew.js`. Remove the existing `resume` textarea entirely. Add:

```js
  const category = el("select", { class: "input" });
  [
    ["software_engineering", "Software Engineering"],
    ["data", "Data / ML"],
    ["design", "Design / UX"],
    ["product", "Product"],
    ["marketing", "Marketing"],
    ["sales", "Sales"],
    ["finance", "Finance / Accounting"],
    ["operations", "Operations"],
  ].forEach(([v, label]) => category.append(el("option", { value: v }, label)));

  const keywords = el("input", { class: "input", placeholder: "golang, kubernetes (comma separated)" });

  const resumeFile = el("input", { class: "input", type: "file", accept: "application/pdf" });
  const resumeStatus = el("p", { class: "hint" }, "Upload a PDF to auto-fill category and keywords.");
  let resumeText = "";

  resumeFile.addEventListener("change", async () => {
    const f = resumeFile.files && resumeFile.files[0];
    if (!f) return;
    resumeStatus.textContent = "Parsing résumé…";
    try {
      const r = await api.parseResume(f);
      resumeText = r.resume_text;
      if (r.suggested_category) category.value = r.suggested_category;
      if (r.suggested_keywords.length) keywords.value = r.suggested_keywords.join(", ");
      resumeStatus.textContent = "Résumé parsed ✓ — review the suggestions below.";
    } catch (e) {
      resumeStatus.textContent = "Could not parse that PDF: " + e.message;
    }
  });
```

- [ ] **Step 3: Submit payload** — use the parsed `resumeText` (no textarea), and include category/keywords. Keep the existing recency-weight flip line exactly as the current file has it:

```js
    const payload = {
      user_id: user.user_id,
      name: name.value.trim(),
      instructions: instructions.value.trim(),
      interval_hours: parseInt(interval.value, 10),
      resume_text: resumeText,
      recency_weight: 100 - parseInt(slider.value, 10),
      category: category.value,
      keywords: keywords.value.split(",").map((s) => s.trim()).filter(Boolean),
    };
```

Add the new fields to the form card: `field("Category", category)`, `field("Keywords (optional)", keywords, "Comma separated, e.g. golang, kubernetes")`, and a résumé field built inline:

```js
  el("div", { class: "field" }, [
    el("label", { class: "label" }, "Résumé (PDF)"),
    resumeFile,
    resumeStatus,
  ]),
```

- [ ] **Step 4: Verify** `node --check web/js/views/workerNew.js web/js/api.js`. Manual: pick a PDF, watch category/keywords auto-fill, submit, worker created.

- [ ] **Step 5: Commit** `git add -A && git commit -m "feat(web): pdf résumé upload + AI pre-fill + category/keywords on create form"`

### Task F2: Worker lifecycle UI (pause/resume + delete)

**Files:**
- Modify: `web/js/api.js`
- Modify: `web/js/views/worker.js`

- [ ] **Step 1: api.js methods.**

```js
  async setWorkerStatus(id, status) {
    await request("PATCH", `/workers/${id}/status`, { status });
  },
```

(`request` returns null on 204 — it reads the text and JSON-parses only if non-empty.)

- [ ] **Step 2: Buttons on worker detail** (`worker.js`, in `renderDetail`, next to "Run now"): a Pause/Resume button (toggles based on `worker.status`) and a Delete button. "Delete" is a soft delete (archive), which hides the worker from the list:

```js
  const pauseLabel = worker.status === "paused" ? "Resume" : "Pause";
  const pauseBtn = el("button", { class: "btn btn-secondary" }, pauseLabel);
  pauseBtn.addEventListener("click", async () => {
    const next = worker.status === "paused" ? "active" : "paused";
    await api.setWorkerStatus(worker.worker_id, next);
    navigate(`/workers/${worker.worker_id}`); // re-render
  });

  const delBtn = el("button", { class: "btn btn-ghost" }, "Delete");
  delBtn.addEventListener("click", async () => {
    if (!confirm(`Delete worker "${worker.name}"? It will be archived and hidden.`)) return;
    await api.setWorkerStatus(worker.worker_id, "archived");
    navigate("/workers");
  });
```

Add both into the `page-head` action area (alongside `runNow`). `navigate` to the same path forces a re-render (the router supports same-path re-render).

- [ ] **Step 3: Verify** `node --check web/js/api.js web/js/views/worker.js`. Manual: pause toggles the badge; delete returns to the list and the worker is gone from it.

- [ ] **Step 4: Commit** `git add -A && git commit -m "feat(web): pause/resume + archive (soft delete) worker"`

### Task F3: Show category/department on results (optional polish)

**Files:**
- Modify: `web/js/views/run.js`

- [ ] **Step 1:** In `jobRow`, include the department in the sub-line if present: change the sub-line join to `[j.company, j.department || j.location].filter(Boolean).join(" · ")`. Ensure `normalize`/`jobFromMap` already pass `department` through the run output (Task B3 keeps it in `job_search` output; `score_jobs` strips `description` but not `department`, so it survives).

- [ ] **Step 2: Verify** `node --check web/js/views/run.js`. Commit `git add -A && git commit -m "feat(web): show department on job rows"`.

---

## Final verification

- [ ] `go build ./... && go vet ./... && go test ./...` all green.
- [ ] `for f in $(find web/js -name '*.js'); do node --check "$f"; done` all OK.
- [ ] Manual end-to-end (stack up): on the create form, upload a PDF résumé and confirm category + keywords auto-fill; create a Software Engineering worker with keyword "golang", Run now, confirm: two-step run, engineering jobs only, golang-relevant, ranked sensibly; pause hides it from the scheduler; delete archives it and it disappears from the list.

## Notes / decisions baked in

- **Delete = soft delete (archive).** "Delete" sets `status = 'archived'`; `ListWorkersByUser` hides archived workers and `ListDueWorkers` (status = 'active') never runs them. Nothing is destroyed — run history is preserved and a worker could be un-archived later if we add that. No delete endpoint or FK cascade needed.
- **Deterministic pipeline replaces LLM planning for job runs.** This is what makes runs reliable. The LLM is retained only for scoring. `web_search`/`document_read`/`http_request` stay registered but are no longer reachable by job runs, so the wrong-tool and resolver-crash bugs are gone. (If you later re-broaden into a general agent platform, reintroduce LLM planning behind a worker "type".)
- **Lexical filter ceiling.** Category+department matching plus whole-word keywords is far better than title-substring, but still lexical. Semantic (pgvector) retrieval remains the future upgrade when the catalog grows, and pairs with a persistent jobs+embeddings store.
- **Résumé is now PDF-only, parsed server-side.** The textarea is removed; the `resume_text` column is unchanged (it stores the extracted text). PDF parsing handles text-based PDFs; scanned/image résumés need OCR (out of scope) and will simply yield empty text, in which case scoring falls back to category+keywords. The résumé→category/keywords suggestion is best-effort: if the LLM call fails, the form still works (the user picks manually).
- **`/resumes/parse` runs before create.** The form does a two-step flow (upload → parse+suggest → user reviews → create), which is what enables the AI pre-fill. The worker is still created via the normal JSON `POST /workers` with the extracted `resume_text`.
