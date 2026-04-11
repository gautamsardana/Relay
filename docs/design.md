# Relay — AI Orchestrator Design Spec

**Date:** 2026-04-06

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
  planner/    — receives command, calls agent, persists workflow + steps, publishes first step
  agent/      — Claude API client; generates structured plans and recovery steps
  worker/     — step executor; updates state, publishes next step to RabbitMQ
  tools/      — pluggable tool registry
  store/      — Postgres queries (workflows, steps)
  models/     — shared types (Workflow, Step, ToolCall, etc.)

web/          — frontend UI
```

### Request Flow

1. `POST /workflows` — planner creates a workflow record in Postgres, returns `workflow_id` immediately
2. Planner calls agent (Claude) with the command and available tools → receives an ordered list of steps
3. All steps are inserted into Postgres (`pending`) with positions 1–N
4. Step 1's event is published to RabbitMQ queue `relay.steps`
5. A worker consumes the event, executes the step via the appropriate tool, updates step status in Postgres
6. On success → worker publishes the next step's event to RabbitMQ
7. On failure → worker calls agent with full context (completed steps, failed step, error) → agent returns replacement step(s) → worker inserts them and publishes the first replacement
8. Workflow completes when no more steps remain

---

## Data Model

### `workflows`

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| command | TEXT | Original user command |
| status | ENUM | `pending`, `running`, `completed`, `failed` |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

### `steps`

| Column | Type | Notes |
|---|---|---|
| id | UUID | Primary key |
| workflow_id | UUID | FK → workflows |
| position | INT | Execution order (1-indexed) |
| status | ENUM | `pending`, `running`, `completed`, `failed` |
| tool | TEXT | Tool name (e.g. `web_search`, `notion_write`) |
| input | JSONB | Tool input parameters |
| output | JSONB | Tool result |
| error | TEXT | Error message if failed |
| retry_count | INT | Default 0 |
| created_at | TIMESTAMP | |
| updated_at | TIMESTAMP | |

### RabbitMQ

**Queue:** `relay.steps`

**Message payload:**
```json
{
  "workflow_id": "uuid",
  "step_id": "uuid"
}
```

Messages are intentionally thin. Workers fetch full step details from Postgres. Each message is delivered to exactly one worker — RabbitMQ ensures no two workers process the same step. This allows multiple worker instances to run concurrently, each handling steps from different workflows, scaling horizontally independent of the API server.

---

## Agent

The `internal/agent` package wraps the Claude API.

**Plan generation:** Called by the planner with the user's command and the list of available tools (names + descriptions). Claude returns a structured JSON array of steps:

```json
[
  {"position": 1, "tool": "web_search", "input": {"query": "software eng jobs US 2024"}},
  {"position": 2, "tool": "notion_write", "input": {"content": "results from step 1"}}
]
```

**Step input interpolation:** Claude references prior step outputs using a template syntax (e.g. `{{steps[0].output.results}}`). The worker resolves these templates at execution time by reading the relevant step outputs from Postgres. This front-loads all intelligence into the planning prompt — the worker is a dumb template resolver, not a decision-maker. Claude knows the output shape of each tool because tool descriptions include their response schema.

**Failure recovery:** When a step fails, the agent is called again with:
- The original command
- All completed steps and their outputs
- The failed step and its error message

Claude returns replacement step(s). The worker inserts them into Postgres (adjusting positions) and publishes the first replacement to RabbitMQ.

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

A central registry holds all registered tools. At startup, all tools are registered and their names + descriptions are passed to Claude during plan generation so it knows what it can use.

**Initial tools:**
- `web_search` — searches the web for a query
- `notion_write` — writes content to a Notion page
- `document_read` — reads a document by URL or path
- `http_request` — generic escape hatch; accepts URL, method, headers, body — allows Claude to call any API without a dedicated tool implementation

Adding a new tool = implement the `Tool` interface and register it at startup. Nothing else changes.

**Transform steps:** For cases where a tool returns more data than needed (e.g. 50 job listings when only 10 are wanted), Claude plans an explicit intermediate step using a lightweight `transform` tool — a small focused Claude call that filters/summarizes data before passing it to the next tool. This avoids bloating downstream tool inputs and keeps each step's responsibility narrow.

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
- **Workflow detail** — displays the full step plan, live status per step (pending / running / completed / failed), and step outputs

The UI polls `GET /workflows/:id` every 3 seconds to reflect live progress. WebSockets can be added later for true real-time updates.

---

## Step Execution

Steps within a single workflow run **sequentially** — each step waits for the previous to complete before its RabbitMQ message is published. The `position` field enforces order.

The worker binary runs on a single node with a configurable number of goroutines consuming from `relay.steps` concurrently. This allows multiple workflows to make progress in parallel (e.g. goroutine 1 handles workflow A's step 2, goroutine 2 handles workflow B's step 1 simultaneously). The concurrency setting lives in config (e.g. `WORKER_CONCURRENCY=5`).

---

## Error Handling

- Workers retry a failed step up to N times (configurable)
- After retries are exhausted, the agent is invoked to generate a recovery plan
- If the agent cannot recover, the workflow is marked `failed`

---

## Key Design Decisions & Tradeoffs

### Message Queue: RabbitMQ over Kafka
Kafka was the original choice but was replaced with RabbitMQ. Kafka is an event streaming platform built for massive throughput and message replay — neither of which Relay needs. Steps are already durably stored in Postgres, so replay is redundant. RabbitMQ is a simpler task queue that fits the use case exactly: deliver one message to one worker, done. Easier to run locally, lower operational overhead.

### No Queue vs Queue
A queue is justified specifically for **horizontal worker scaling independent of the API server**. The API server is lightweight (validates input, returns workflow ID). Workers are heavy (execute steps that take 5-30 seconds each). A queue lets you run N worker goroutines without touching the API server. Without a queue, you'd manage goroutine pools and backpressure yourself.

### One Binary vs Two
The API server, planner, and agent layer are grouped into one binary because every request flows through all three sequentially — they scale together. Workers are a separate binary because they scale independently based on step execution load.

### Agent Role: Planner Only
The agent (Claude) is responsible for two things only: generating the initial step plan, and generating recovery steps on failure. It does not execute steps. Execution is done entirely by workers calling tools directly. This keeps Claude calls minimal (one per workflow creation, one per failure) and keeps workers fast and deterministic.

### Step Input Resolution: Template Interpolation
Claude front-loads all intelligence at plan-generation time by encoding step dependencies as template references (e.g. `{{steps[0].output.results}}`). Workers resolve these at execution time by reading from Postgres. This avoids a Claude call per step execution, which would be slow and expensive (10 steps = 10 Claude calls). The tradeoff is that Claude must know tool output shapes upfront — enforced by including response schemas in tool descriptions.

### Storage as Source of Truth
All state — workflow status, step inputs, step outputs, errors — lives in Postgres. RabbitMQ messages are intentionally thin (just IDs). This means workers are stateless: they can crash and restart without losing anything. It also means the worker directly updates step state rather than routing through the planner, keeping the architecture simple.

### Future: Semantic Caching
For repeated similar commands, the planning step (Claude call) could be skipped by caching the generated step plan and retrieving it via semantic similarity (vector embeddings). Only the plan is cached — workers still execute fresh. This is out of scope for v1 but a natural next optimization to reduce latency and cost.
