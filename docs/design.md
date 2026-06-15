# Relay v2 — System Design

**Last updated:** 2026-06-14

---

## Overview

Relay is an AI agent runtime. Users create persistent workers with instructions and a schedule. Workers run automatically, execute multi-step plans via distributed workers, and accumulate memory across runs.

The execution engine from v1 (planner → RabbitMQ → workers → Postgres) is unchanged. What changes is the abstraction above it: instead of one-shot workflows, users define long-lived workers that produce workflow runs on a schedule.

---

## Architecture

### Two Binaries

```
cmd/api/     — HTTP server, planner, scheduler cron, reconciler cron
cmd/worker/  — RabbitMQ consumer, step executor
```

The API binary handles everything related to creating and orchestrating work. The worker binary handles execution. They scale independently.

### Request Flow (v2)

1. User creates a **Worker** with instructions and a cron schedule
2. At the scheduled time, the **Scheduler** (cron inside API binary) creates a **WorkflowRun** for that worker
3. The **Planner** reads the worker's instructions and current `worker_state`, calls Claude to generate a step plan
4. Steps are inserted into Postgres as `pending`, first step published to RabbitMQ
5. **Worker goroutines** consume from the queue, claim steps atomically, execute tools
6. On step success: worker updates step → `success`, publishes next step to RabbitMQ
7. On last step success: worker marks run → `success`, writes updated state back to `worker_state`
8. On failure: retry up to MAX_RETRIES, then mark step and run as `failed`
9. **Reconciler** (cron inside API binary) re-publishes steps stuck in `processing` due to worker crashes

### How Memory Flows Into Planning

Before calling Claude, the planner reads `worker_state.data` and injects it into the planning prompt:

```
System: You are an execution planner...
        Available tools: [tool list with schemas]
        Worker instructions: [worker.instructions]
        Worker memory (from previous runs): [worker_state.data as JSON]
        ...
User: Generate a plan for this run.
```

Claude sees what the worker already knows and generates steps accordingly — e.g. skipping jobs already in memory, narrowing searches based on past results.

At the end of a run, a `state_write` tool call updates `worker_state.data` with new information.

---

## Data Model

### `users`
```sql
user_id     UUID PRIMARY KEY
email       TEXT NOT NULL UNIQUE
created_at  TIMESTAMPTZ
```

### `workers`
```sql
worker_id     UUID PRIMARY KEY
user_id       UUID REFERENCES users
name          TEXT NOT NULL
instructions  TEXT NOT NULL         -- what the worker does
schedule      TEXT NOT NULL         -- cron expression, e.g. "0 9 * * *"
status        ENUM(active, paused, archived)
resume_url    TEXT                  -- optional, for resume-scored job search
created_at    TIMESTAMPTZ
updated_at    TIMESTAMPTZ
```

Represents a permanent AI employee. Owns its own memory and run history.

### `worker_state` _(future, not v1)_

Not built in v1. The job hunter doesn't need it — `seen_jobs` handles deduplication and there is no user feedback loop yet to accumulate preferences.

Will become useful when:
- User feedback exists (ignore this company, prefer these roles) and the worker needs to remember it across runs
- Other domains are added that genuinely need persistent context (competitor monitor tracking last seen release, stock tracker storing moving averages)
- Vector memory (RAG) replaces it entirely for semantic recall

When introduced:
```sql
state_id    UUID PRIMARY KEY
worker_id   UUID REFERENCES workers UNIQUE
data        JSONB NOT NULL DEFAULT '{}'
updated_at  TIMESTAMPTZ
```

### `workflow_runs`
```sql
run_id      UUID PRIMARY KEY
worker_id   UUID REFERENCES workers
status      ENUM(init, processing, success, failed)
error       TEXT
started_at  TIMESTAMPTZ
finished_at TIMESTAMPTZ
```

One execution of a worker. Created by the scheduler or a manual trigger. Replaces the v1 `workflows` table.

### `steps`
```sql
step_id      UUID PRIMARY KEY
run_id       UUID REFERENCES workflow_runs   -- was workflow_id in v1
step_number  INT NOT NULL
tool         TEXT NOT NULL
description  TEXT NOT NULL
input        JSONB NOT NULL DEFAULT '{}'
output       JSONB
status       ENUM(pending, processing, success, failed)
retry_count  INT NOT NULL DEFAULT 0
error        TEXT
created_at   TIMESTAMPTZ
updated_at   TIMESTAMPTZ
```

Same concept as v1. `run_id` replaces `workflow_id`.

### `seen_jobs` (job-hunter specific)
```sql
id          UUID PRIMARY KEY
worker_id   UUID REFERENCES workers
company_id  TEXT NOT NULL     -- ATS company slug
job_id      TEXT NOT NULL     -- ATS job identifier
seen_at     TIMESTAMPTZ
UNIQUE(worker_id, company_id, job_id)
```

Deduplication table. The `job_search` tool queries this table directly and filters out already-seen jobs before returning results — so Claude never sees duplicates and `worker_state` never accumulates job IDs. At the end of a run, newly surfaced job IDs are inserted here. After 100 runs this table grows large; `worker_state` stays lean.

### RabbitMQ

**Queue:** `relay.steps`

**Message:**
```json
{ "run_id": "uuid", "step_id": "uuid" }
```

Thin messages — workers fetch full step details from Postgres. Quorum queue for durability.

---

## Agent

The agent package wraps multiple LLM providers with a primary/fallback pattern.

**Providers:** Claude (primary), GPT-4, Groq

**Plan generation prompt includes:**
- Worker instructions
- Worker memory (current `worker_state.data`)
- Available tools (name, description, input schema, output schema)
- Expected JSON response format

**Response format:**
```json
[
  {
    "step_number": 1,
    "tool": "job_search",
    "description": "Search for remote Go backend jobs on Greenhouse and Lever",
    "input": {"companies": ["stripe", "vercel", "linear"], "role": "backend", "remote": true}
  },
  {
    "step_number": 2,
    "tool": "state_write",
    "description": "Update worker memory with newly seen job IDs",
    "input": {"seen_job_ids": "{{steps[1].output.job_ids}}"}
  }
]
```

**Template interpolation:** `{{steps[N].output.field}}` references are resolved by the worker at execution time by reading prior step outputs from Postgres.

---

## Tools

Go interface — pluggable:

```go
type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}
```

### v1 Tools (already built)
| Tool | What it does |
|---|---|
| `web_search` | Tavily API — general web search |
| `http_request` | Generic HTTP escape hatch |
| `document_read` | Fetch URL or read local file |

### v2 Tools (to build)
| Tool | What it does |
|---|---|
| `job_search` | Aggregates jobs from Greenhouse, Ashby, Lever ATS APIs — filters against `seen_jobs` before returning |
| `score_jobs` | Claude call ranking job list against resume |
| `notify` | Send notification (email, Slack webhook) |
| `state_read` / `state_write` | _(future)_ Read/write `worker_state` — not needed until user feedback or multi-domain support |

### `job_search` Architecture

Generic web search returns job board pages, not job postings. The `job_search` tool hits ATS APIs directly:

```
job_search(companies, filters)
        │
        ├── Greenhouse: boards.greenhouse.io/v1/boards/{slug}/jobs
        ├── Ashby:      api.ashbyhq.com/jobPosting.list
        └── Lever:      api.lever.co/v0/postings/{slug}
                │
        Normalize to common schema
                │
        Deduplicate against seen_jobs table
                │
        Return structured job list
```

Input schema: `{"companies": [string], "role": string, "remote": bool, "min_salary": int}`
Output schema: `{"jobs": [{"id", "company", "title", "url", "location", "salary"}]}`

**Company list:** Maintained as a curated list of tech companies with their ATS slugs. Starts at ~300-500. Expanded by web search or user additions over time.

---

## API

| Method | Path | Description |
|---|---|---|
| POST | `/workers` | Create a worker |
| GET | `/workers` | List all workers for user |
| GET | `/workers/:id` | Get worker detail + run history |
| POST | `/workers/:id/run` | Manually trigger a run |
| GET | `/runs/:id` | Get run detail + steps |
| GET | `/ws/runs/:id` | WebSocket — live step updates for a run |

---

## Scheduler

Cron running inside the API binary. Runs every minute.

Logic:
1. Find all workers where `status = 'active'` and `next_run_at <= NOW()`
2. For each: create a new `workflow_run`, hand it to the planner, update `next_run_at`

The `next_run_at` field on the worker is computed from the cron expression after each run completes.

---

## Reconciler

Cron running inside the API binary every 60 seconds. Handles crash recovery only — normal step progression is done by the worker publishing the next step directly.

Logic:
1. Find steps where `status = 'processing'` AND `updated_at < NOW() - 5 minutes`
2. For each stuck step:
   - Increment `retry_count`
   - If `retry_count >= MAX_RETRIES` → mark step `failed`, mark run `failed`
   - Otherwise → reset to `pending`, re-publish to RabbitMQ

---

## Key Design Decisions

### Worker is the unit, not the workflow
In v1, a workflow was the top-level concept. Users submitted one-shot requests. In v2, the worker is permanent. Workflow runs are ephemeral executions created by the scheduler. This gives users a persistent entity to own, configure, and observe over time.

### Memory via injected state, not agent loop
The worker doesn't "remember" by being kept alive in memory or by making inference calls between runs. It reads a JSON blob from Postgres before planning and writes an updated blob at the end. Simple, durable, inspectable.

### Dedicated job_search over generic web search
Generic web search returns job board index pages. ATS APIs (Greenhouse, Ashby, Lever) return structured job data. The quality difference is large enough to justify a dedicated tool.

### Dedup by (company_id, job_id), not just job_id
Same job can appear on multiple sources. The unique identifier is the combination of ATS company slug and job ID within that ATS.

### Agent has one job: generate the plan
Claude is called once per run to produce a step plan. It does not execute steps, does not make decisions mid-execution, and is not called on failure. Workers are deterministic executors. This keeps Claude calls minimal (one per run) and workers fast and stateless.

### Postgres as source of truth
RabbitMQ carries thin events (just IDs). All state — run status, step inputs, step outputs, errors, worker memory — lives in Postgres. Workers are stateless and can crash safely.

---

## Future Layers (not v1, but planned)

### Vector Memory
Replace `worker_state` JSONB with proper RAG. Embed past run outputs, retrieve semantically relevant context at planning time. "What jobs did I see that were similar to this one?" becomes a vector search, not a JSON lookup.

### Observability
Trace every planning prompt, every tool call input/output, every token used. Store as structured spans in Postgres or a time-series store. Build a dashboard showing agent decisions over time.

### Rate Limiting + Backpressure
When 100 workers fire simultaneously, the system needs to degrade gracefully. Rate limit Claude API calls per user, queue overflow, and handle tool rate limits (Greenhouse API limits).

### Evaluation
How do you know if the job hunter is getting better or worse? Build a lightweight eval framework on top of execution traces — score outputs against expected results, track quality over time.

### Notifications
After a run completes, send output to the user. Email first (simplest), then Slack webhook, then in-app. Implemented as a `notify` tool step at the end of each plan.

### Worker Pause/Resume
Workers have `status: active | paused | archived`. Paused workers skip scheduler triggers but retain all memory and history.
