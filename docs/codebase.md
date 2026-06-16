# Relay — Codebase Reference

**Last updated:** 2026-06-14

This document is a map of the codebase as it exists today. Read this when you want to know where something lives or how the pieces connect.

---

## Directory Structure

```
cmd/
  api/main.go          — boots the API server, planner, agent manager, tool registry
  executor/main.go     — boots the RabbitMQ consumer with N goroutines

internal/
  agent/
    agent.go           — Agent interface + StepPlan struct
    manager.go         — AgentManager: primary/fallback provider pattern
    claude.go          — Claude implementation (Anthropic SDK)
    gpt.go             — GPT implementation (OpenAI SDK)
    groq.go            — Groq implementation
  api/
    server.go          — chi router setup, route registration, static file serving
    handler.go         — HTTP handlers: CreateWorkflow, ListWorkflows, GetWorkflow
    ws.go              — WebSocket handler: streams step updates to frontend
  config/
    config.go          — Config struct, loads from .env and config.yaml
    .env               — secrets (DB URL, API keys) — not committed
    config.yaml        — app settings (AI providers, worker count)
  migrations/
    create_workflows.sql
    create_steps.sql
  models/
    workflow.go        — Workflow struct + WorkflowStatus enum
    step.go            — Step struct + StepStatus enum
  planner/
    planner.go         — Planner struct; CreateWorkflow, GetWorkflow, GetStepsByWorkflow
    logic.go           — HandleWorkflow: calls agent, validates tools, inserts steps, publishes first step
  queue/
    rabbitmq.go        — Dial, QueueManager (New, PublishStep, ConsumeSteps)
    manager.go         — QueueManager struct
    docker-compose.yaml — local RabbitMQ
  store/
    db.go              — Postgres connection (database/sql)
    workflow.go        — CreateWorkflow, GetWorkflow, UpdateWorkflowStatus
    step.go            — InsertSteps, GetStepByID, ListStepsByWorkflow, UpdateStepStatus, UpdateStepAsCompleted, GetNextStep
    adapter.go         — converts between models.* and sqlc.* types
    queries/
      workflows.sql    — raw SQL queries (source for sqlc)
      steps.sql        — raw SQL queries (source for sqlc)
    sqlc/
      db.go            — sqlc-generated DB wrapper
      models.go        — sqlc-generated structs (Step, Workflow)
      workflows.sql.go — sqlc-generated query functions
      steps.sql.go     — sqlc-generated query functions
  tools/
    registry.go        — Tool interface, Registry (Register, Get, All, Names, BuildToolDescriptions)
    web_search.go      — Tavily API integration
    http_request.go    — generic HTTP tool
    document_read.go   — HTTP fetch or local file read

web/
  index.html           — single-page UI
  app.js               — submits workflow, opens WebSocket, renders step updates
  styles.css           — styles

sqlc.yaml              — sqlc config (generates internal/store/sqlc/)
go.mod / go.sum
```

---

## How the Pieces Connect

### API Binary (`cmd/api`)

```
main.go
  │
  ├── store.New()           — Postgres connection
  ├── queue.Dial()          — RabbitMQ connection
  ├── queue.New()           — creates QueueManager (channel + declared queue)
  ├── agent.NewAgentManager() — primary + fallback LLM clients
  ├── tools.NewRegistry()   — registers web_search, http_request, document_read
  ├── planner.New()         — wires store + queue + agent + registry together
  └── api.New(planner)      — chi router, starts HTTP on :8080
```

### Executor Binary (`cmd/executor`)

```
main.go
  │
  ├── store.New()
  ├── queue.Dial()
  ├── tools.NewRegistry()
  └── executor.New() → executor.SpawnExecutors()
        └── N goroutines, each:
              queue.New() → qm.ConsumeSteps(w.HandleStep)
```

### Creating a Workflow (v1 flow, currently implemented)

```
POST /workflow
  → handler.CreateWorkflow
  → planner.CreateWorkflow
      ├── store.CreateWorkflow          (status: init)
      └── go planner.HandleWorkflow
            ├── agent.GeneratePlan      (Claude call)
            ├── validate tool names
            ├── store.InsertSteps       (status: pending)
            ├── store.UpdateWorkflow    (status: processing)
            └── queue.PublishStep       (step 1)
```

### Executing a Step

```
queue.ConsumeSteps → executor.HandleStep(event)
  ├── store.GetStepByID
  ├── store.UpdateStepStatus            (status: processing)
  ├── registry.Get(step.Tool)
  ├── tool.Execute(step.Input)
  ├── store.UpdateStepAsCompleted       (status: success, saves output)
  ├── store.GetNextStep
  │     ├── if found → queue.PublishStep (next step)
  │     └── if none  → store.UpdateWorkflowStatus (success)
  └── on error → store.UpdateStepStatus(failed) + store.UpdateWorkflowStatus(failed)
```

### WebSocket Streaming

```
GET /ws/workflows/:id
  → ws.StreamWorkflowSteps
      └── ticker every 1s:
            ├── planner.GetWorkflow
            ├── planner.GetStepsByWorkflow
            ├── build stepUpdatePayload
            └── conn.WriteMessage (only if payload changed)
```

---

## Store Layer

Relay uses **sqlc** to generate type-safe Go code from raw SQL queries.

The query flow:
1. Write SQL in `internal/store/queries/*.sql`
2. Run `sqlc generate` → produces `internal/store/sqlc/*.go`
3. The `store/` layer wraps sqlc with model conversions via `adapter.go`

**Never edit files in `internal/store/sqlc/` directly** — they are generated.

`adapter.go` handles all conversions between `models.Workflow`/`models.Step` and the sqlc-generated structs. This keeps the rest of the codebase clean of sqlc types.

---

## Agent Layer

`AgentManager` implements a primary/fallback pattern:

```
AgentManager.GeneratePlan
  └── primary.GeneratePlan (Claude)
        ├── success → return plan
        └── error   → secondary.GeneratePlan (GPT / Groq)
```

Providers are configured in `config.yaml`:
```yaml
ai_primary: anthropic
ai_secondary: openai
```

Each provider implements the `Agent` interface:
```go
type Agent interface {
    GeneratePlan(ctx context.Context, request string, tools []tools.Tool) ([]StepPlan, error)
}
```

**Note:** `claude.go`'s `GeneratePlan` currently returns nil — the real prompt/parsing logic has not been implemented yet.

---

## Tool Layer

Tools implement:
```go
type Tool interface {
    Name()        string
    Description() string
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}
```

Tools are registered at startup in `main.go`:
```go
registry.Register(tools.NewWebSearch(config))
registry.Register(tools.NewHTTPRequest())
registry.Register(tools.NewDocumentRead())
```

`registry.All()` is called by the planner to pass tool descriptions to Claude.

`BuildToolDescriptions()` formats tool names and descriptions for inclusion in the prompt.

---

## Config

Two config files, both loaded at startup:

**`internal/config/.env`** — secrets
```
DATABASE_URL=postgres://...
QUEUE_URL=amqp://guest:guest@localhost:5672/
CLAUDE_API_KEY=...
GPT_API_KEY=...
GROQ_API_Key=...
TAVILY_API_KEY=...
```

**`internal/config/config.yaml`** — app settings
```yaml
ai_primary: anthropic
ai_secondary: openai
worker_count: 5
```

---

## What is Not Yet Built

| Component | Status |
|---|---|
| `claude.go` GeneratePlan | Stub — returns nil |
| `gpt.go` GeneratePlan | Stub |
| `groq.go` GeneratePlan | Stub |
| `handler.go` ListWorkflows | Stub — returns empty |
| `handler.go` GetWorkflow | Stub — returns empty |
| Reconciler cron | Not implemented |
| Scheduler cron | Not implemented |
| Worker auth / users table | Not implemented |
| Workers / WorkflowRuns (v2 model) | Not implemented |
| `job_search` tool | Not implemented |
| `state_read` / `state_write` tools | Not implemented |
| `score_jobs` tool | Not implemented |
| `notify` tool | Not implemented |
| Worker memory injection into planner | Not implemented |
| Template interpolation in step inputs | Not implemented |
