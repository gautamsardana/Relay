# Job Search — Subsystem A Design

**Date:** 2026-06-17
**Status:** Approved design, ready for implementation plan
**Scope:** Subsystem A only. Automated company discovery (Subsystem B) is a separate, later spec.

---

## 1. Goal

The job hunter is Relay's first real use case and flagship. A user creates a worker
("find me backend/infra roles, score against my resume"), it runs on a schedule, and
each run surfaces **new** matching jobs, ranked. This spec covers everything needed to
deliver that end to end with a hand-seeded company list.

The two things that make or break it: **good source jobs** (clean ATS feeds) and
**deduplication** (never show the same job twice). Both are covered here.

---

## 2. Scope

**In scope (Subsystem A):**
- A curated company catalog (table, seeded from a checked-in file).
- `job_search` tool: crawl 3 ATS board types, filter, dedup → new matches.
- `score_jobs` tool: rank new matches by a user-controlled blend of resume-fit + recency.
- The execution-context plumbing tools need to read the catalog / dedup / resume.

**Out of scope (separate specs / later):**
- **Subsystem B — automated discovery** (BFS crawler / dataset enrichment that grows the
  catalog). Purely additive: it just `INSERT`s catalog rows; startup seeding only touches
  rows present in the JSON, so discovered rows are never wiped. **Revisit the seeding
  strategy when B ships** (e.g. seed only if the table is empty, or treat the JSON as a
  one-time bootstrap) so the static JSON can't clobber the `name` of an overlapping
  discovered/edited company.
- **Workday and YC** boards (Workday has no clean public API; YC is a discovery concern).
- **PDF resume upload** (v1 takes plain-text resume; PDF→text parsing is a later upgrade).
- **Global jobs cache / Redis** (we chose live freshness; see §9).
- **Per-worker company filters** (user choosing/excluding companies) — later, lives on the
  worker, distinct from the catalog `active` flag.
- **First-run flood cap** (when everything is "new," scoring input can be large) — minor,
  later. Keyword filter trims most of it.

---

## 3. Key Decisions (summary)

1. **Company universe = global curated catalog**, user filters by role keywords. (Not a
   user-typed company list.)
2. **Live fetch per run** (freshness), no jobs cache. Redis deferred to a different use case
   (semantic step cache — see project memory).
3. **ATS platforms v1: Greenhouse, Lever, Ashby.** Each a small adapter behind one interface.
4. **LLM-planned flow**, not a fixed pipeline. Plan is `job_search → score_jobs`. Dedup is
   folded inside `job_search` so the plan stays two steps.
5. **Show all new matches, ranked** (no hard top-N that hides matches). "Found = shown."
6. **Mark jobs seen at the end** (in `score_jobs`), idempotently. Crash-safe (see §8).
7. **Resume = plain text**, stored on the worker, captured once at setup.
8. **Ranking = user-controlled blend of resume-fit (LLM) + recency (code).**

---

## 4. Data Model Changes

### 4.1 New table: `companies` (the catalog)

```sql
CREATE TABLE companies (
    company_id  UUID PRIMARY KEY,
    name        TEXT NOT NULL,            -- display, e.g. "Stripe"
    ats         TEXT NOT NULL,            -- 'greenhouse' | 'lever' | 'ashby'
    slug        TEXT NOT NULL,            -- board id on that ATS, e.g. "stripe"
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ats, slug)
);
CREATE INDEX idx_companies_active ON companies(active);
```

- `ats` is TEXT (not an enum) so adding Workday/etc. later is a data change, not a migration.
- `active`: **operational/health switch, global to the system.** Turn off when a board
  breaks, a company stops hiring, or discovery adds junk — without deleting the row. It is
  **not** the user's per-worker filter (that's separate, later). The crawler reads
  `WHERE active = TRUE`.

**Seeding:** a checked-in file (e.g. `internal/tools/data/companies.json`) of ~100 companies
with `{name, ats, slug}`. Seeded into the table on startup (idempotent upsert on
`(ats, slug)`). Subsystem B later adds rows the same way.

### 4.2 Worker changes

- `resume_url` → **`resume_text TEXT`** (a few KB; storing it is trivially cheap). Captured
  once at worker setup as pasted plain text.
- Add **`recency_weight INT NOT NULL DEFAULT 50`** (0–100). User-facing slider. Validated
  0–100 in `planner.CreateWorker`.

### 4.3 `seen_jobs` (already exists, unchanged)

```
UNIQUE(worker_id, company_id, job_id)
```
- `company_id` = catalog `slug`; `job_id` = ATS-provided job id. Both stable, no ids to invent.
- Per-worker (different users care about different jobs).

---

## 5. Tool Architecture — how stateful tools get their dependencies

Two kinds of dependency, handled separately.

### 5.1 Static deps → constructor injection (like the existing API-key pattern)

```go
func NewJobSearch(store *store.Store) *JobSearch        // store: read catalog + seen_jobs
func NewScoreJobs(store *store.Store, agent *agent.AgentManager) *ScoreJobs
```

- `job_search` needs the store (catalog read, seen_jobs read).
- `score_jobs` needs the agent (LLM fit scoring) **and** the store (write seen_jobs).
- Existing stateless tools keep their current constructors untouched.

### 5.2 Per-run context → `ExecutionContext` passed to `Execute`

`worker_id` / `run_id` / `resume_text` / `recency_weight` vary per run and can't be known at
construction (and the LLM can't supply them). They're system-provided via a typed param:

```go
type ExecutionContext struct {
    RunID         string
    WorkerID      string
    ResumeText    string
    RecencyWeight int    // 0..100
}

type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, input map[string]any, exec ExecutionContext) (map[string]any, error)
}
```

- The 3 existing tools add the param and ignore it (one-line change each).
- The executor builds it once per step from the run → worker:
  ```go
  run, _    := store.GetRunByID(ctx, step.RunID)
  worker, _ := store.GetWorkerByID(ctx, run.WorkerID)
  exec := ExecutionContext{
      RunID: step.RunID, WorkerID: run.WorkerID,
      ResumeText: worker.ResumeText, RecencyWeight: worker.RecencyWeight,
  }
  tool.Execute(ctx, resolvedInput, exec)
  ```
  Two indexed PK reads per step — negligible; collapsible to one join later.

**Why a typed struct** (over reserved keys in the input map, or `context.Value`): keeps
system run-metadata cleanly separate from LLM/plan input, makes the contract explicit and
greppable, and avoids key-collision footguns.

---

## 6. `job_search` Tool (step 1)

Registered tool the LLM sees. Internally composed of smaller **non-registered** Go pieces
(the LLM never orchestrates these — they're deterministic plumbing):

**Common job shape:**
```go
type Job struct {
    CompanyID string    // catalog slug
    Company   string    // display name
    JobID     string    // ATS-provided id
    Title     string
    URL       string
    Location  string
    ATS       string    // greenhouse | lever | ashby
    PostedAt  time.Time // normalized from each board's date field
}
```

**ATS adapters behind one interface:**
```go
type atsAdapter interface {
    Fetch(ctx context.Context, slug string) ([]Job, error)
}
```
- **Greenhouse:** `GET https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true`
  — date from `updated_at`.
- **Lever:** `GET https://api.lever.co/v0/postings/{slug}?mode=json` — date from `createdAt`.
- **Ashby:** `POST https://api.ashbyhq.com/posting-api/job-board/{slug}` — date from
  `publishedAt` (exact field confirmed when wiring the adapter).

**Date caveat:** the three timestamps don't mean exactly the same thing (created vs updated
vs published). Good enough as a recency *signal*; not perfectly apples-to-apples.

**Execute flow:**
1. Read active companies: `store.ListActiveCompanies()`.
2. Fan out concurrently with a bounded worker pool (e.g. 10 at a time); dispatch each row to
   the adapter for `row.ats`.
3. Normalize all results into `[]Job`.
4. Keyword/location filter against the worker's role keywords (from step input) — cheap.
5. Dedup **read**: drop jobs already in `seen_jobs` for `exec.WorkerID`. What remains = new
   matches. (Read-only here — no write, keeps this step idempotent.)
6. Return `{ "jobs": [ ...new matches... ] }`.

A failed adapter call for one company is logged and skipped, not fatal to the run.

---

## 7. `score_jobs` Tool (step 2, last step)

Input: the new matches from step 1 (via template interpolation `{{steps[1].output.jobs}}`).
Reads `exec.ResumeText` and `exec.RecencyWeight`.

**Flow:**
1. If `RecencyWeight == 100`: **skip the LLM** entirely; rank purely by `PostedAt`.
2. Otherwise: ask the LLM for a **fit score** per job (0–100) against `ResumeText`.
3. Compute scores in code and blend:
   - `fit = llm_score / 100`            (0..1)
   - `recency = clamp(1 - age_days / WINDOW, 0, 1)`, `WINDOW = 30 days`
   - `w = RecencyWeight / 100`
   - `final = w*recency + (1-w)*fit`
4. Sort by `final` descending → ranked list (all of them; ranking decides order, not visibility).
5. **Mark seen:** insert the ranked jobs into `seen_jobs` (`worker_id, company_id=slug,
   job_id`), `ON CONFLICT DO NOTHING` (idempotent).
6. Return `{ "ranked_jobs": [...] }` → saved as the run's output.

---

## 8. Dedup Timing & Crash Safety

- Dedup **read** is in step 1 (`job_search`); seen **write** is in step 2 (`score_jobs`).
- Because the read and write are in different steps, a retry of `score_jobs` cannot hide
  jobs: step 1's output already holds the new matches, and `score_jobs` re-runs against that
  same output. The `seen_jobs` unique constraint + `ON CONFLICT DO NOTHING` make the write
  repeat-proof.
- If the run dies before `score_jobs` records, nothing is marked seen; the retry redoes it
  and the user still gets the jobs.
- "Found = shown" (we show all new matches), so there's no leftover unsurfaced pile to reason
  about. In the rare crash exactly at the recording step, a user might miss a few jobs once —
  acceptable for v1; not worth exactly-once machinery for an hourly feed.

---

## 9. Run History & Freshness

- Every run is its own `runs` row with its own steps/outputs keyed by `run_id`. **All runs
  are preserved forever**; the frontend lists a worker's run history and can open any past
  run to see exactly what it produced. (API already exists: worker detail lists runs;
  `GET /runs/{id}` returns run + steps.)
- Each run's output is the **delta** (new jobs from that run), not a cumulative total. A
  combined "all jobs ever" view would be a separate aggregation, later.
- Freshness: ATS feeds are hit live per run, so results are current. The only near-static
  thing is the catalog. Runs are scheduled (hourly+), so the few-seconds fan-out latency is a
  non-issue.

---

## 10. End-to-End Flow

```
scheduler fires run  →  planner.HandleRun  →  agent plans: [job_search, score_jobs]
   step 1  job_search:  catalog → 3 ATS adapters (parallel) → normalize → keyword filter
                        → dedup read (seen_jobs) → new matches
   step 2  score_jobs:  LLM fit + code recency → blend by recency_weight → ranked
                        → write seen_jobs (idempotent)
   run output = ranked new matches, saved under run_id (viewable anytime)
```

Manual **Run Now** (`POST /workers/{id}/run`) works the same, independent of the schedule.

---

## 11. Files Touched (anticipated)

- `internal/migrations/`: new `companies` table; worker `resume_url`→`resume_text` +
  `recency_weight`.
- `internal/models/`: `Job`, worker field changes.
- `internal/store/`: catalog queries (`ListActiveCompanies`, upsert-seed), seen_jobs
  write/read, worker field plumbing; adapter + sqlc regen.
- `internal/tools/`: `Tool` interface gains `ExecutionContext`; `job_search.go`,
  `score_jobs.go`, `ats/` adapters, `data/companies.json`.
- `internal/executor/worker.go`: build + pass `ExecutionContext`.
- `internal/agent/`: planning prompt awareness of the two new tools (descriptions).
- `internal/api/` + `internal/planner/`: worker create accepts `resume_text` +
  `recency_weight`; validation.
- `cmd/api/main.go`: register new tools; run catalog seeding on startup.

---

## 12. Open Items / Assumptions

- Ashby's exact date field name confirmed during adapter wiring.
- Recency `WINDOW` (30 days) and concurrency cap (10) are system constants for v1; tunable.
- Keyword filter is simple substring/word match for v1; semantic matching is the LLM's job in
  `score_jobs`, not the filter.
- Claude `GeneratePlan` is still stubbed; for real runs the primary provider must be
  openai/groq, or implement Claude planning (tracked separately).
