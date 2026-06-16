# Relay v2 — Implementation Plan

**Last updated:** 2026-06-14

**Goal:** Evolve Relay from a one-shot workflow engine into an AI agent runtime with persistent workers, scheduled runs, and accumulated memory. Ship a working Job Hunter worker as the first flagship use case.

---

## What Is Already Built (v1 Foundation)

The execution engine is complete and working:

- [x] Postgres store with sqlc-generated queries
- [x] RabbitMQ quorum queue (publish + consume)
- [x] Worker binary — N goroutines consuming steps concurrently
- [x] Planner — creates workflow, calls agent, inserts steps, publishes first step
- [x] Agent manager — primary/fallback LLM provider pattern (Claude, GPT, Groq)
- [x] Tool registry — pluggable Tool interface
- [x] Tools: `web_search` (Tavily), `http_request`, `document_read`
- [x] chi HTTP server with routes
- [x] WebSocket streaming — live step updates to frontend
- [x] Frontend — submit workflow, watch steps update in real time

**What is stubbed / incomplete:**
- Agent `GeneratePlan` implementations return nil (no real prompts yet)
- `ListWorkflows` and `GetWorkflow` handlers return empty
- ~~No reconciler~~ (done)
- ~~No scheduler~~ (done — interval-based, Phase 3)
- No template interpolation in step inputs
- No retry logic in worker

---

## Migration Strategy

The v1 `workflows` table becomes `workflow_runs`. A new `workers` table sits above it. This is an additive migration — the execution engine (steps, queue, worker binary) barely changes.

---

## Phase 1 — Complete the v1 Engine

These are things that should have been in v1 but weren't finished. Do these before touching the v2 model.

### 1.1 Real Agent Planning (Claude)

**File:** `internal/agent/claude.go`

Implement `GeneratePlan`:
- Build system prompt: explain the task, list available tools with name/description/input schema/output schema, specify JSON response format
- Build user prompt: the workflow request string
- Call Anthropic SDK, parse JSON response into `[]StepPlan`
- Return error if JSON is malformed or required fields missing

Test: submit a real request, verify Claude returns a sensible plan that the planner accepts.

### 1.2 Template Interpolation

**File:** `internal/worker/worker.go`

Before calling `tool.Execute(step.Input)`, resolve template references:

```go
// step.Input might contain: {"content": "{{steps[1].output.results}}"}
// Replace with actual value from prior step's output in Postgres
```

Logic:
1. Scan `step.Input` values for `{{steps[N].output.field}}` patterns
2. Fetch step N's output from Postgres
3. Replace the template value with the actual output value

### 1.3 Retry Logic

**File:** `internal/worker/worker.go`

Currently `failStep` immediately marks the step and workflow failed. Add retry:

1. On tool failure: `IncrementRetryCount`
2. If `retry_count < MAX_RETRIES` → re-publish same step to RabbitMQ (not failed yet)
3. If `retry_count >= MAX_RETRIES` → call existing `failStep`

Add `IncrementRetryCount` to the store layer.

### 1.4 Reconciler

**File:** `internal/planner/reconciler.go`

Cron running every 60 seconds. Handles steps stuck in `processing` because a worker crashed.

```
GetStuckSteps(5 * time.Minute)   // status='processing' AND updated_at < NOW()-5min
  → IncrementRetryCount
  → if retry_count >= MAX_RETRIES: mark step+workflow failed
  → else: ResetStepToPending, re-publish to RabbitMQ
```

Wire into `cmd/api/main.go` as a goroutine with `time.Ticker`.

### 1.5 Finish REST Handlers

**File:** `internal/api/handler.go`

Implement `GetWorkflow` and `ListWorkflows`. These need the store queries added to `queries/workflows.sql` + sqlc regenerated.

---

## Phase 2 — v2 Data Model

Introduce the Worker abstraction without breaking the execution engine.

### 2.1 Migrations

**New migrations:**

```sql
-- create_users.sql
CREATE TABLE users (
    user_id    UUID PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- create_workers.sql
CREATE TYPE worker_status AS ENUM ('active', 'paused', 'archived');

CREATE TABLE workers (
    worker_id    UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(user_id),
    name         TEXT NOT NULL,
    instructions TEXT NOT NULL,
    schedule     TEXT NOT NULL,        -- cron expression
    status       worker_status NOT NULL DEFAULT 'active',
    resume_url   TEXT,
    next_run_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- create_workflow_runs.sql (replaces workflows table for new code)
CREATE TABLE workflow_runs (
    run_id      UUID PRIMARY KEY,
    worker_id   UUID NOT NULL REFERENCES workers(worker_id),
    status      workflow_status NOT NULL DEFAULT 'init',   -- reuse existing enum
    error       TEXT,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

-- create_seen_jobs.sql
CREATE TABLE seen_jobs (
    id         UUID PRIMARY KEY,
    worker_id  UUID NOT NULL REFERENCES workers(worker_id),
    company_id TEXT NOT NULL,
    job_id     TEXT NOT NULL,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(worker_id, company_id, job_id)
);

-- alter steps to reference workflow_runs
ALTER TABLE steps ADD COLUMN run_id UUID REFERENCES workflow_runs(run_id);
-- (migrate data, then drop workflow_id from steps once all code is updated)
```

### 2.2 Models + Store

Add `models/worker.go`, `models/run.go`.

Add store files: `store/worker.go`, `store/run.go`, `store/worker_state.go`.

Add sqlc queries for each.

### 2.3 Update Planner

`planner.CreateRun(ctx, workerID string) (string, error)` — creates a `workflow_run`, reads `worker_state`, calls agent with memory injected, inserts steps referencing `run_id`.

The planning prompt now includes:
```
Worker instructions: {worker.instructions}
Worker memory: {worker_state.data as JSON}
```

### 2.4 Update Worker

`HandleStep` currently uses `workflow_id`. Update to use `run_id`. On last step success: update run status → `success`, call `state_write` if the plan included that step.

### 2.5 New API Routes

```
POST   /workers              → create worker
GET    /workers              → list workers
GET    /workers/:id          → worker detail + recent runs
POST   /workers/:id/run      → manually trigger a run
GET    /runs/:id             → run detail + steps
GET    /ws/runs/:id          → WebSocket live step updates
```

---

## Phase 3 — Scheduler ✅ (interval-based)

**File:** `internal/planner/scheduler.go`

**Done.** Shipped as a simple **interval scheduler**, not cron. Calendar
scheduling ("Mondays at 9am", timezones) is deferred — when added, introduce a
`schedule_kind` discriminator + a cron column and keep interval workers working
untouched. Because scheduling is interval-only, `robfig/cron/v3` was **not**
needed.

**Data model change:** `workers.schedule TEXT` → `workers.interval_seconds INT NOT NULL`
(model field `IntervalSeconds int`). Stored in seconds for future flexibility
(minutes later is free); the API currently accepts `interval_hours`.

A ticker runs inside the API binary every 60 seconds:
1. `ListDueWorkers` — `SELECT * FROM workers WHERE status='active' AND next_run_at <= NOW()`
2. For each worker: `planner.CreateRun(workerID)`
3. Advance `next_run_at = now + interval_seconds` (done even if `CreateRun`
   errors, so a failing worker doesn't hot-loop every tick)

Wired into `cmd/api/main.go` next to `StartReconciler()`.

**Validation:** minimum interval enforced at **1 hour** (`minIntervalSeconds = 3600`)
in `planner.CreateWorker` — the frontend enforces it for UX, the backend
re-checks for bad actors (returns 400).

**On create:** `next_run_at = now + interval`, so the first *automatic* run is one
interval out. Immediate feedback comes from the manual **Run Now** trigger
(`POST /workers/{id}/run` → `CreateRun`), which fires a run and never touches
`next_run_at` — fully decoupled from the schedule.

**Known limitation:** assumes a **single API instance**. Multiple instances would
double-fire due workers until we add an atomic claim
(`UPDATE ... WHERE next_run_at <= NOW() ... RETURNING` / `FOR UPDATE SKIP LOCKED`).

---

## Phase 4 — Job Hunter Tools

This is the first real use case. Build the tools that make the job hunter work.

### 4.1 `job_search` Tool

**File:** `internal/tools/job_search.go`

Aggregates jobs from ATS platforms:

- **Greenhouse:** `GET https://boards.greenhouse.io/v1/boards/{slug}/jobs?content=true`
- **Ashby:** `POST https://api.ashbyhq.com/jobPosting.list` with `{"jobBoardIdentifier": slug}`
- **Lever:** `GET https://api.lever.co/v0/postings/{slug}?mode=json`

Input schema:
```json
{
  "companies": ["stripe", "vercel", "linear"],
  "role_keywords": ["backend", "golang", "infrastructure"],
  "remote_only": true
}
```

Output schema:
```json
{
  "jobs": [
    {
      "company_id": "stripe",
      "job_id": "123456",
      "company": "Stripe",
      "title": "Backend Engineer, Infrastructure",
      "url": "https://stripe.com/jobs/...",
      "location": "Remote",
      "ats": "greenhouse"
    }
  ]
}
```

Ship with a curated list of ~100 tech companies and their ATS slugs as a starting point.

### 4.2 `score_jobs` Tool

**File:** `internal/tools/score_jobs.go`

Takes a list of jobs and a resume URL, calls Claude to score and rank them.

Input: `{"jobs": [...], "resume_url": "...", "top_n": 10}`
Output: `{"ranked_jobs": [...], "reasoning": [...]}`

---

## Phase 5 — Frontend v2

The UI needs to match the new worker model.

**Home:** List of workers with status, schedule, last run time. Button to create a new worker.

**Create Worker:** Form with name, instructions, cron schedule, optional resume URL.

**Worker Detail:** Worker info + run history list. Each run shows status and timestamp.

**Run Detail:** Step-by-step execution view with real-time WebSocket updates. Same as current workflow detail but scoped to a run.

---

## Phase 6 — Observability

Once the happy path is solid, add visibility into what the agent is actually doing.

- Log every planning prompt and response to a `planning_logs` table
- Log tool input/output for every step execution (already partially in `steps.input` / `steps.output`)
- Build a simple "trace view" in the UI: see the exact prompt Claude received, the plan it returned, each tool call

This teaches you how production AI systems are instrumented and debugged.

---

## Phase 7 — Rate Limiting + Backpressure

When multiple workers fire simultaneously:

- Claude API has per-minute token limits — need a rate limiter in the agent layer
- Tool APIs (Greenhouse, etc.) have per-second request limits — need per-tool rate limiting
- Worker goroutines should shed load rather than queue unbounded work

Implement token bucket rate limiting per provider. Add metrics (request count, error rate, queue depth) exposed on `/metrics` for Prometheus.

---

## Implementation Order Summary

| Phase | What | Priority |
|---|---|---|
| 1 | Complete v1 engine (real Claude planning, retry, reconciler, template interpolation) | High — foundation |
| 2 | v2 data model (workers, runs, memory) | High — the pivot |
| 3 | Scheduler | High — makes workers autonomous |
| 4 | Job hunter tools (job_search, state_read/write, score_jobs) | High — first real use case |
| 5 | Frontend v2 | Medium |
| 6 | Observability | Medium |
| 7 | Rate limiting + backpressure | Lower |

After Phase 4 you have a working job hunter that runs every morning, deduplicates results, and gets smarter over time. That's the target.
