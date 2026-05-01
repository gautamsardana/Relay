# Relay v1 Implementation Plan

**Goal:** Build a working AI orchestrator that accepts natural language commands, breaks them into steps via Claude, and executes them asynchronously via RabbitMQ workers with full state in Postgres.

**Architecture:** Two binaries — `cmd/api` (API server + planner + agent) and `cmd/worker` (step executor). Postgres is the source of truth; RabbitMQ carries thin step events. The planner binary also runs reconciler and scheduler crons.

**Tech Stack:** Go, PostgreSQL, RabbitMQ, Claude API (claude-sonnet-4-6), Tavily API (web search), Notion API

---

## File Structure

```
cmd/
  api/
    main.go              — boots HTTP server, planner, cron jobs
  worker/
    main.go              — boots RabbitMQ consumer with N goroutines

internal/
  models/
    workflow.go          — Workflow struct + status enum
    step.go              — Step struct + status enum
  store/
    db.go                — Postgres connection setup
    workflow_store.go    — workflow CRUD queries
    step_store.go        — step CRUD queries
  agent/
    agent.go             — Claude API client, plan generation prompt + parsing
  planner/
    planner.go           — orchestrates: create workflow, call agent, insert steps, publish step 1
    reconciler.go        — cron: find stuck processing workflows, re-publish next pending step
  worker/
    worker.go            — consumes from RabbitMQ, claims step, executes tool, publishes next
  tools/
    registry.go          — central tool registry, Name→Tool map
    web_search.go        — Tavily API integration
    notion_write.go      — Notion API integration
    document_read.go     — HTTP fetch / file read
    http_request.go      — generic HTTP escape hatch
  queue/
    rabbitmq.go          — RabbitMQ connection, publish, consume helpers
  api/
    server.go            — HTTP server setup, route registration
    handlers.go          — POST /workflows, GET /workflows/:id, GET /workflows

web/                     — frontend (built separately)

migrations/
  001_create_workflows.sql
  002_create_steps.sql
```

---

## Phase 1 — Foundation

### Task 1: Repo scaffold

- [ ] Initialise Go module: `go mod init github.com/yourusername/relay`
- [ ] Create the full directory structure above (`mkdir -p` for each path)
- [ ] Add a `.env.example` with all required env vars:
  ```
  DATABASE_URL=postgres://...
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/
  ANTHROPIC_API_KEY=
  TAVILY_API_KEY=
  NOTION_API_KEY=
  WORKER_CONCURRENCY=5
  MAX_STEP_RETRIES=3
  RECONCILER_INTERVAL_SECONDS=30
  ```
- [ ] Commit: `feat: initial repo scaffold`

---

### Task 2: Models

**File:** `internal/models/workflow.go`
```go
type WorkflowStatus string

const (
    WorkflowStatusInit       WorkflowStatus = "init"
    WorkflowStatusProcessing WorkflowStatus = "processing"
    WorkflowStatusSuccess    WorkflowStatus = "success"
    WorkflowStatusFailed     WorkflowStatus = "failed"
)

type Workflow struct {
    WorkflowID string
    Request    string
    Status     WorkflowStatus
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

**File:** `internal/models/step.go`
```go
type StepStatus string

const (
    StepStatusPending    StepStatus = "pending"
    StepStatusProcessing StepStatus = "processing"
    StepStatusSuccess    StepStatus = "success"
    StepStatusFailed     StepStatus = "failed"
)

type Step struct {
    StepID      string
    WorkflowID  string
    StepNumber  int
    Tool        string
    Description string
    Input       map[string]any  // JSONB
    Output      map[string]any  // JSONB
    Status      StepStatus
    RetryCount  int
    Error       string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

- [ ] Write models
- [ ] Commit: `feat: add workflow and step models`

---

### Task 3: Postgres migrations

**File:** `migrations/001_create_workflows.sql`
```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE workflow_status AS ENUM ('init', 'processing', 'success', 'failed');

CREATE TABLE workflows (
    workflow_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request      TEXT NOT NULL,
    status       workflow_status NOT NULL DEFAULT 'init',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**File:** `migrations/002_create_steps.sql`
```sql
CREATE TYPE step_status AS ENUM ('pending', 'processing', 'success', 'failed');

CREATE TABLE steps (
    step_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES workflows(workflow_id),
    step_number  INT NOT NULL,
    tool         TEXT NOT NULL,
    description  TEXT NOT NULL,
    input        JSONB NOT NULL DEFAULT '{}',
    output       JSONB,
    status       step_status NOT NULL DEFAULT 'pending',
    retry_count  INT NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_steps_workflow_id ON steps(workflow_id);
CREATE INDEX idx_steps_status ON steps(status);
```

- [ ] Write migrations
- [ ] Run them against a local Postgres instance
- [ ] Commit: `feat: add database migrations`

---

### Task 4: Store layer

**File:** `internal/store/db.go` — Postgres connection using `pgx` or `database/sql` + `lib/pq`. Accept `DATABASE_URL` from env.

**File:** `internal/store/workflow_store.go` — implement:
- `CreateWorkflow(ctx, request string) (*models.Workflow, error)`
- `GetWorkflow(ctx, workflowID string) (*models.Workflow, error)`
- `ListWorkflows(ctx) ([]*models.Workflow, error)`
- `UpdateWorkflowStatus(ctx, workflowID string, status models.WorkflowStatus) error`

**File:** `internal/store/step_store.go` — implement:
- `InsertSteps(ctx, steps []*models.Step) error` — batch insert
- `GetStepsByWorkflow(ctx, workflowID string) ([]*models.Step, error)`
- `GetStep(ctx, stepID string) (*models.Step, error)`
- `ClaimStep(ctx, stepID string) (bool, error)` — atomic: `UPDATE steps SET status='processing' WHERE step_id=? AND status='pending'`, returns true if claimed
- `UpdateStepSuccess(ctx, stepID string, output map[string]any) error`
- `UpdateStepFailed(ctx, stepID string, errMsg string) error`
- `IncrementRetryCount(ctx, stepID string) error`
- `GetNextPendingStep(ctx, workflowID string) (*models.Step, error)` — lowest step_number where status='pending'
- `GetStuckSteps(ctx, timeout time.Duration) ([]*models.Step, error)` — steps where status='processing' AND updated_at < NOW() - timeout
- `ResetStepToPending(ctx, stepID string) error` — sets status back to 'pending', updates updated_at

- [ ] Implement store layer (use `pgx/v5` — simpler API than `database/sql` for Postgres)
- [ ] Commit: `feat: add store layer`

---

## Phase 2 — API + Planner (stub agent)

### Task 5: Tool registry + stub tools

**File:** `internal/tools/registry.go`
```go
type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry
func (r *Registry) Register(t Tool)
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) All() []Tool   // used to build the Claude prompt
func (r *Registry) Names() []string
```

- [ ] Implement registry
- [ ] Add a `stub` tool that just returns `{"ok": true}` — useful for testing the full flow without real API calls
- [ ] Commit: `feat: add tool registry with stub tool`

---

### Task 6: RabbitMQ helpers

**File:** `internal/queue/rabbitmq.go`

```go
type StepEvent struct {
    WorkflowID string `json:"workflow_id"`
    StepID     string `json:"step_id"`
}

func Connect(url string) (*amqp.Connection, error)
func PublishStep(ch *amqp.Channel, event StepEvent) error
func ConsumeSteps(ch *amqp.Channel, handler func(StepEvent) error) error
```

Queue name: `relay.steps`. Use `amqp091-go` library (`github.com/rabbitmq/amqp091-go`).

- [ ] Implement
- [ ] Commit: `feat: add RabbitMQ helpers`

---

### Task 7: Stub agent

**File:** `internal/agent/agent.go`

```go
type StepPlan struct {
    StepNumber  int            `json:"step_number"`
    Tool        string         `json:"tool"`
    Description string         `json:"description"`
    Input       map[string]any `json:"input"`
}

type Agent interface {
    GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error)
}
```

Implement a `StubAgent` that returns 2 hardcoded steps using the `stub` tool. This lets you test the full end-to-end flow without a real Claude call.

- [ ] Implement stub agent
- [ ] Commit: `feat: add stub agent for local testing`

---

### Task 8: Planner

**File:** `internal/planner/planner.go`

```go
type Planner struct {
    store    *store.Store
    agent    agent.Agent
    queue    *queue.Queue
    registry *tools.Registry
}

func (p *Planner) HandleRequest(ctx context.Context, request string) (string, error)
```

Logic:
1. `CreateWorkflow` → get `workflow_id`
2. Call `agent.GeneratePlan`
3. Validate all tool names exist in registry — if not, `UpdateWorkflowStatus(failed)` and return error
4. `InsertSteps`
5. `UpdateWorkflowStatus(processing)`
6. `PublishStep` for step 1
7. Return `workflow_id`

- [ ] Implement planner
- [ ] Commit: `feat: add planner`

---

### Task 9: API server + handlers

**File:** `internal/api/server.go` — set up `net/http` or `chi` router, register routes

**File:** `internal/api/handlers.go`

```
POST /workflows     → { "workflow_id": "uuid" }
GET  /workflows/:id → { workflow + steps array }
GET  /workflows     → [ workflow list ]
```

- [ ] Implement
- [ ] Boot `cmd/api/main.go` — wire together store, registry, stub agent, planner, HTTP server
- [ ] Manually test: `curl -X POST /workflows -d '{"request":"test"}'` — should return workflow_id
- [ ] Commit: `feat: add API server and handlers`

---

### Task 10: Worker (stub execution)

**File:** `internal/worker/worker.go`

```go
func (w *Worker) ProcessStep(ctx context.Context, event queue.StepEvent) error
```

Logic:
1. `ClaimStep` — if returns false, another worker got it, return nil
2. Load step from store
3. Resolve template inputs (replace `{{steps[N].output.X}}` with actual values from Postgres)
4. Get tool from registry, call `Execute`
5. On success: `UpdateStepSuccess`, call `GetNextPendingStep` — if found, publish it to RabbitMQ immediately; if none left, mark workflow `success`
6. On failure: `IncrementRetryCount`, if `retry_count < MAX_RETRIES` re-publish same step to RabbitMQ; else `UpdateStepFailed`, mark workflow `failed`

- [ ] Implement worker
- [ ] Boot `cmd/worker/main.go` — wire together store, registry, queue consumer, N goroutines
- [ ] End-to-end test with stub agent + stub tool: submit workflow via API, watch it complete in Postgres
- [ ] Commit: `feat: add worker`

---

## Phase 3 — Real Agent

### Task 11: Claude agent

Replace `StubAgent` with a real implementation in `internal/agent/agent.go`.

- Use `github.com/anthropics/anthropic-sdk-go`
- Model: `claude-sonnet-4-6`
- Build the planning prompt:
  - System: explain the task, available tools (name + description + input schema + output schema), and the expected JSON response format
  - User: the natural language request
- Parse Claude's JSON response into `[]StepPlan`
- Return error if JSON is malformed or any required field is missing

- [ ] Implement real Claude agent
- [ ] Swap into `cmd/api/main.go` (keep stub agent available via env flag for local testing)
- [ ] Test: submit a real request, verify Claude returns a sensible plan
- [ ] Commit: `feat: add Claude agent`

---

## Phase 4 — Real Tools

### Task 12: `web_search` tool

**File:** `internal/tools/web_search.go`

- Call Tavily API (`POST https://api.tavily.com/search`)
- Input schema: `{"query": string}`
- Output schema: `{"results": [{"title": string, "url": string, "content": string}]}`
- Auth: `TAVILY_API_KEY` from env

- [ ] Implement
- [ ] Commit: `feat: add web_search tool`

---

### Task 13: `notion_write` tool

**File:** `internal/tools/notion_write.go`

- Call Notion API to append blocks to a page
- Input schema: `{"page_id": string, "content": string}`
- Output schema: `{"success": bool, "url": string}`
- Auth: `NOTION_API_KEY` from env

- [ ] Implement
- [ ] Commit: `feat: add notion_write tool`

---

### Task 14: `document_read` tool

**File:** `internal/tools/document_read.go`

- If input URL starts with `http` — HTTP GET, return body
- Otherwise — read from local file path
- Input schema: `{"source": string}`
- Output schema: `{"content": string}`

- [ ] Implement
- [ ] Commit: `feat: add document_read tool`

---

### Task 15: `http_request` tool

**File:** `internal/tools/http_request.go`

- Generic HTTP call
- Input schema: `{"url": string, "method": string, "headers": object, "body": string}`
- Output schema: `{"status_code": int, "body": string}`

- [ ] Implement
- [ ] Commit: `feat: add http_request tool`

---

## Phase 5 — Reconciler

### Task 16: Reconciler cron

**File:** `internal/planner/reconciler.go`

The reconciler handles only crash recovery — steps stuck in `processing` because the worker died mid-execution. Normal step progression is handled by the worker publishing directly. The reconciler runs every 60 seconds.

Logic:
1. Call `GetStuckSteps(ctx, 5*time.Minute)` — steps where `status='processing'` AND `updated_at < NOW() - 5 minutes`
2. For each stuck step:
   - `IncrementRetryCount`
   - If `retry_count >= MAX_RETRIES` → `UpdateStepFailed`, `UpdateWorkflowStatus(failed)`
   - Otherwise → `ResetStepToPending`, re-publish its event to RabbitMQ using the same `step_id`

- [ ] Implement
- [ ] Wire into `cmd/api/main.go` as a goroutine with a ticker
- [ ] Commit: `feat: add reconciler cron`

---

## Phase 6 — Frontend

> **This phase is built by Claude.** When you reach here, hand it over.

- Home view: text input to submit a command, list of past workflows with status badges
- Workflow detail view: step list with status per step, outputs, errors
- Polls `GET /workflows/:id` every 3 seconds

---

## Implementation Order Summary

| Phase | What | Who |
|---|---|---|
| 1 | Scaffold, models, migrations, store | You |
| 2 | API, planner, stub agent, worker | You |
| 3 | Real Claude agent | You (ask for help on prompt) |
| 4 | Real tools | You (one at a time) |
| 5 | Reconciler | You |
| 6 | Frontend | Claude |

**Rule of thumb:** After Phase 2 you have a fully working system with fake data. After Phase 3 you have a real AI planner. After Phase 4 you have real execution. Ship in that order — each phase is independently testable.
