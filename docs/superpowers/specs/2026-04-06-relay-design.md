# Relay — AI Orchestrator Design Spec

**Date:** 2026-04-06

## Overview

Relay is an AI orchestrator that accepts natural language commands (e.g. "find 10 latest software engineering jobs in the US and post to Notion"), breaks them into executable steps using Claude, and runs those steps asynchronously via Kafka workers. State is durably maintained in Postgres throughout.

---

## Architecture

### Repo Structure

Monorepo with two deployable binaries:

```
cmd/
  api/        — REST server (workflow submission + status)
  worker/     — Kafka consumer (step execution)

internal/
  planner/    — receives command, calls agent, persists workflow + steps, publishes first step
  agent/      — Claude API client; generates structured plans and recovery steps
  worker/     — step executor; updates state, publishes next step to Kafka
  tools/      — pluggable tool registry
  store/      — Postgres queries (workflows, steps)
  models/     — shared types (Workflow, Step, ToolCall, etc.)

web/          — frontend UI
```

### Request Flow

1. `POST /workflows` — planner creates a workflow record in Postgres, returns `workflow_id` immediately
2. Planner calls agent (Claude) with the command and available tools → receives an ordered list of steps
3. All steps are inserted into Postgres (`pending`) with positions 1–N
4. Step 1's event is published to Kafka topic `relay.steps`
5. A worker consumes the event, executes the step via the appropriate tool, updates step status in Postgres
6. On success → worker publishes the next step's event to Kafka
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

### Kafka

**Topic:** `relay.steps`

**Event payload:**
```json
{
  "workflow_id": "uuid",
  "step_id": "uuid"
}
```

Events are intentionally thin. Workers fetch full step details from Postgres to avoid large Kafka payloads.

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

Note: Claude describes step inputs in natural language referencing prior steps. The worker resolves actual outputs from Postgres when executing each step.

**Failure recovery:** When a step fails, the agent is called again with:
- The original command
- All completed steps and their outputs
- The failed step and its error message

Claude returns replacement step(s). The worker inserts them into Postgres (adjusting positions) and publishes the first replacement to Kafka.

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

Steps run **sequentially** — each step waits for the previous to complete before its Kafka event is published. The `position` field enforces order.

Future: steps with no dependencies on each other could be parallelized by publishing multiple events at once, but this is out of scope for the initial version.

---

## Error Handling

- Workers retry a failed step up to N times (configurable)
- After retries are exhausted, the agent is invoked to generate a recovery plan
- If the agent cannot recover, the workflow is marked `failed`
