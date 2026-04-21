# Relay — AI Orchestrator Design Spec

**Date:** 2026-04-06
**Last updated:** 2026-04-21

## Overview

Relay is an AI orchestrator that accepts natural language commands (e.g. "find 10 latest software engineering jobs in the US and post to Notion"), breaks them into executable steps using Claude, and runs those steps asynchronously via RabbitMQ workers. State is durably maintained in Postgres throughout.

---

## Architecture

### Repo Structure

Monorepo with two deployable binaries:

```
cmd/
  api/        — REST server (workflow submission + status)
  worker/     — RabbitMQ consumer (step execution)

internal/
  planner/    — receives command, calls agent, persists workflow + steps, publishes first step, runs cron jobs
  agent/      — Claude API client; generates structured step plans
  worker/     — step executor; updates step + workflow state, publishes next step to RabbitMQ
  tools/      — pluggable tool registry
  store/      — Postgres queries (workflows, steps)
  models/     — shared types (Workflow, Step, etc.)

web/          — frontend UI
```

### Component Ownership

- **API server, planner, agent** — same binary, logically separated. They scale together and every request flows through all three sequentially.
- **Worker** — separate binary. Scales independently based on step execution load.
- **Cron jobs (reconciler, scheduler)** — run inside the planner binary.

### Request Flow

1. `POST /workflows` — API receives request, planner creates a workflow record in Postgres with status `init`, returns `workflow_id` immediately
2. Planner calls agent (Claude) with the command and available tools → receives an ordered list of steps
3. All steps are inserted into Postgres with status `pending`
4. Planner validates that every tool name Claude returned exists in the tool registry — rejects the plan and marks workflow `failed` if not
5. Planner marks workflow status → `processing`, publishes step 1's event to RabbitMQ queue `relay.steps`
6. A worker consumes the event, atomically claims the step (`UPDATE ... WHERE status='pending'`), marks it `processing`, executes it via the appropriate tool
7. On success → worker marks step `success`, publishes next step's event to RabbitMQ; if no steps remain, marks workflow `success`
8. On failure → worker increments `retry_count`, retries up to max; after max retries, marks step `failed` and workflow `failed`
9. Workflow completes when all steps are marked `success`

---

## Data Model

### `workflows`

| Column | Type | Notes |
|---|---|---|
| workflow_id | UUID | Primary key |
| request | TEXT | Original natural language command |
| status | ENUM | `init`, `processing`, `success`, `failed` |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

**Status transitions:**
- `init` — set by planner on workflow creation (before Claude is called)
- `processing` — set by planner after all steps are inserted and step 1 is published
- `success` — set by worker after last step completes successfully
- `failed` — set by worker after a step exhausts all retries

### `steps`

| Column | Type | Notes |
|---|---|---|
| step_id | UUID | Primary key |
| workflow_id | UUID | FK → workflows |
| step_number | INT | Execution order (1-indexed) |
| tool | TEXT | Tool name (e.g. `web_search`, `notion_write`) |
| description | TEXT | Natural language description of what this step does |
| input | JSONB | Tool input parameters |
| output | JSONB | Tool result |
| status | ENUM | `pending`, `processing`, `success`, `failed` |
| retry_count | INT | Default 0; incremented before each retry attempt |
| error | TEXT | Error message if failed |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

**Status transitions (all worker-owned):**
- `pending` — inserted by planner
- `processing` — set by worker atomically before execution begins
- `success` / `failed` — set by worker after execution

### RabbitMQ

**Queue:** `relay.steps`

**Message payload:**
```json
{
  "workflow_id": "uuid",
  "step_id": "uuid"
}
```

Messages are intentionally thin. Workers fetch full step details from Postgres. Each message is delivered to exactly one worker — RabbitMQ ensures no two workers process the same step. Multiple worker instances can run concurrently, each handling steps from different workflows, scaling horizontally independent of the API server.

---

## Agent

The `internal/agent` package wraps the Claude API.

**Plan generation:** Called by the planner with the user's command and the list of available tools (names, descriptions, and input/output schemas). Claude returns a structured JSON array of steps:

```json
[
  {
    "step_number": 1,
    "tool": "web_search",
    "description": "Search for the 10 latest software engineering jobs in the US",
    "input": {"query": "latest software engineering jobs US 2024"}
  },
  {
    "step_number": 2,
    "tool": "notion_write",
    "description": "Post the filtered job results to Notion",
    "input": {"content": "{{steps[1].output.results}}"}
  }
]
```

**Tool validation:** After receiving the plan, the planner checks every `tool` value against the registered tool registry. If Claude returns an unrecognised tool name, the workflow is marked `failed` immediately rather than letting it blow up at execution time.

**Step input interpolation:** Claude references prior step outputs using a template syntax (e.g. `{{steps[1].output.results}}`). The worker resolves these templates at execution time by reading the relevant step outputs from Postgres. This front-loads all intelligence into the planning prompt — the worker is a dumb template resolver, not a decision-maker. Claude knows the output shape of each tool because tool descriptions include their response schema.

---

## Tools

Tools are pluggable via a Go interface:

```go
type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}
```

A central registry holds all registered tools. At startup, all tools are registered and their names, descriptions, and input/output schemas are passed to Claude during plan generation so it knows what it can use.

Adding a new tool = implement the `Tool` interface and register it at startup. Nothing else changes.

**Tool credentials** (API keys, tokens) are injected via environment variables into tool constructors at startup. Never hardcoded.

**MCP:** Not used. MCP is for when Claude calls tools during inference. In Relay, Claude only generates a plan — workers call tools directly. MCP adds no value here.

### Initial Tools

| Tool | Integration | Notes |
|---|---|---|
| `web_search` | Tavily or Serper API | Built for LLM use cases, returns clean structured results |
| `notion_write` | Notion REST API | Requires integration token |
| `document_read` | HTTP fetch (URL) or file read (path) | No external dependency |
| `http_request` | Go `net/http` | Generic escape hatch; Claude can call any API without a dedicated tool |

**Transform steps:** For cases where a tool returns more data than needed (e.g. 50 job listings when only 10 are wanted), Claude plans an explicit intermediate step using a lightweight `transform` tool — a small focused Claude call that filters/summarises data before passing it to the next tool.

---

## API

**Endpoints:**

| Method | Path | Description |
|---|---|---|
| POST | `/workflows` | Submit a command; returns `workflow_id` |
| GET | `/workflows/:id` | Get workflow details + all steps + statuses |
| GET | `/workflows` | List all workflows |

---

## Frontend

Simple two-view web UI (`web/`):

- **Home** — text input to submit a command; list of past workflows with status
- **Workflow detail** — displays the full step plan, live status per step, and step outputs including errors

The UI polls `GET /workflows/:id` every 3 seconds to reflect live progress. WebSockets can be added later for true real-time updates.

---

## Step Execution

Steps within a single workflow run **sequentially** — each step waits for the previous to complete before its RabbitMQ message is published. The `step_number` field enforces order.

The worker binary runs with a configurable number of goroutines consuming from `relay.steps` concurrently. This allows multiple workflows to make progress in parallel (e.g. goroutine 1 handles workflow A's step 2, goroutine 2 handles workflow B's step 1 simultaneously). The concurrency setting lives in config (e.g. `WORKER_CONCURRENCY=5`).

**Atomic step claiming:** Workers claim a step with a conditional update — `UPDATE steps SET status='processing' WHERE step_id=? AND status='pending'` — and check rows affected before proceeding. If 0 rows updated, another worker already claimed it; abort. This prevents double execution.

---

## Error Handling

- Worker increments `retry_count` in Postgres before each retry attempt
- Workers retry a failed step up to N times (configurable, default 3)
- After retries are exhausted, worker marks step `failed` and workflow `failed`
- No agent recovery call in v1 — see Future Ideas

### Reconciler

A reconciler cron runs inside the planner binary every N seconds to handle the case where a worker process dies before publishing the next step's RabbitMQ event (e.g. OOM kill, crash).

**Logic:**
1. Find workflows where `status = processing` AND no step has `status = processing`
2. Find the lowest `step_number` step with `status = pending`
3. Re-publish that step's event to RabbitMQ

**Key distinctions:**
- Only touches `processing` workflows — `init` workflows are the planner's exclusive territory (Claude may still be generating the plan)
- Resumes from the next `pending` step after the last `success` one — completed steps' outputs are in Postgres and will be interpolated normally by the worker

**Idempotency:** The atomic step claiming (`WHERE status='pending'`) in the worker ensures a reconciler re-publish of an already-claimed step is safely ignored.

---

## Key Design Decisions & Tradeoffs

### Message Queue: RabbitMQ over Kafka
Kafka was the original choice but was replaced with RabbitMQ. Kafka is an event streaming platform built for massive throughput and message replay — neither of which Relay needs. Steps are already durably stored in Postgres, so replay is redundant. RabbitMQ is a simpler task queue that fits the use case exactly: deliver one message to one worker, done. Easier to run locally, lower operational overhead.

### No Queue vs Queue
A queue is justified specifically for **horizontal worker scaling independent of the API server**. The API server is lightweight (validates input, returns workflow ID). Workers are heavy (execute steps that take 5-30 seconds each). A queue lets you run N worker goroutines without touching the API server. Without a queue, you'd manage goroutine pools and backpressure yourself.

### One Binary vs Two
The API server, planner, and agent layer are grouped into one binary because every request flows through all three sequentially — they scale together. Workers are a separate binary because they scale independently based on step execution load.

### Agent Role: Planner Only
The agent (Claude) is responsible for one thing only: generating the initial step plan. It does not execute steps and is not called on failure in v1. Execution is done entirely by workers calling tools directly. This keeps Claude calls minimal (one per workflow creation) and keeps workers fast and deterministic.

### Step Input Resolution: Template Interpolation
Claude front-loads all intelligence at plan-generation time by encoding step dependencies as template references (e.g. `{{steps[1].output.results}}`). Workers resolve these at execution time by reading from Postgres. This avoids a Claude call per step execution, which would be slow and expensive (10 steps = 10 Claude calls). The tradeoff is that Claude must know tool output shapes upfront — enforced by including response schemas in tool descriptions.

### Storage as Source of Truth
All state — workflow status, step inputs, step outputs, errors — lives in Postgres. RabbitMQ messages are intentionally thin (just IDs). This means workers are stateless: they can crash and restart without losing anything. It also means the worker directly updates step state rather than routing through the planner, keeping the architecture simple.

### Workflow vs Step State Ownership
Workers own all step state transitions. Workers also own workflow terminal state transitions (`success`, `failed`) — they check for remaining pending steps after each step completes and mark the workflow accordingly. The planner exclusively owns the `init` → `processing` transition. This gives clear, non-overlapping ownership with no ambiguity.

---

## Future Ideas

### Scheduled Workflows
Allow workflows to be submitted with a cron expression (e.g. "every morning at 10 EST"). A scheduler cron running in the planner binary picks up due schedules and creates a new workflow run. Requires a `schedules` table with cron expression, command, and `next_run_at`. Behaviour when the server was down and a run was missed (catch up vs skip) needs to be decided.

### Document Attachment
Allow users to attach a document (e.g. a resume or profile) when submitting a command, so Claude can use it as context during plan generation. The document content would be injected into the planning prompt at submission time. Requires file upload support on `POST /workflows` and storage (local or S3).

### Job Deduplication
For job-search workflows, add a `seen_jobs` table that tracks job identifiers (URL or hash of title+company) already fetched and posted. Before posting, filter out anything already in `seen_jobs`. This prevents recurring scheduled workflows from reposting the same listings across runs.

### Intelligent Failure Recovery
When a step fails after all retries are exhausted, instead of immediately marking the workflow failed, call the agent with full context — the original command, all completed steps and their outputs, the failed step and its error message — and ask it to generate replacement steps. Open questions to resolve before implementing: how Claude signals it cannot recover, how `step_number` is adjusted for replacement steps, and whether there should be a cap on recovery attempts per workflow.

### Semantic Caching
For repeated similar commands, the planning step (Claude call) could be skipped by caching the generated step plan in Redis and retrieving it via semantic similarity (vector embeddings). Only the plan is cached — workers still execute fresh. A natural next optimisation to reduce latency and cost once v1 is stable.
