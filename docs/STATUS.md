# Relay — Current State (handoff)

Snapshot so any new session can continue. Relay is a "distributed runtime for persistent AI
workers"; the live use case is a **job hunter**.

## What works (end to end)
A user creates a **worker** (category + keywords + PDF résumé + interval + recency/fit slider).
On a schedule (or "Run now") the worker fires a **run** = a deterministic two-step pipeline:
`job_search → score_jobs`, executed asynchronously over RabbitMQ. Results show as ranked job
rows (logo, title, company/department, match score, Apply) in the web UI.

## Architecture / packages
- `internal/planner` — `CreateRun` → `HandleRun` builds the **deterministic** `job_search → score_jobs`
  steps (no LLM planning — that fixed the wrong-tool/resolver-crash bugs). Also `CreateWorker`,
  worker status (pause/resume/archive), `ParseResume` (PDF→text + AI category/keyword suggestion).
  Reconciler + interval **scheduler** crons live here.
- `internal/executor` — pulls step events off RabbitMQ, runs the tool, builds `ExecutionContext`
  (worker id, category, keywords, résumé text, recency weight, instructions) from run→worker.
- `internal/tools` — `Tool` interface (`Execute(ctx, input, ExecutionContext)`), `Registry`,
  `BuildRegistry`. `job_search` (catalog → 3 ATS adapters → category+keyword filter → dedup) and
  `score_jobs` (LLM fit + code recency blend, token-capped). `categories.go` = category→department
  synonym map.
- `internal/tools/ats` — Greenhouse/Lever/Ashby adapters (title, location, **department/team**,
  description, posted_at) + `Probe` (cheap board existence/count).
- `internal/discovery` — **Phase 2**, 6h cron that grows the catalog. Sources: `domain_search`
  (Tavily `include_domains` on the board hosts → slug from URL) and `yc_directory` (YC list →
  slug guesses). Pipeline: dedup → confirm via `ats.Probe` → `UpsertCompany`. Cap 300/run, shuffled.
- `internal/catalog` — `companies.json` (151 verified boards) embedded + seeded on startup (upsert).
- `internal/store` (sqlc, Postgres), `internal/queue` (RabbitMQ), `internal/agent` (LLM:
  Groq/GPT/Claude; `GeneratePlan` + `Complete`), `internal/resume` (PDF text via ledongthuc/pdf).
- `web/` — vanilla ES-module SPA (login, workers list, create form, worker detail, run detail).
  Served by the Go file server with `no-cache` headers.

## Key decisions baked in
- **Matching = two-stage funnel.** Stage 1 (cheap): category→department + whole-word keywords over
  title+description, from the worker config via `ExecutionContext`. Stage 2: LLM scores fit vs
  (category+keywords+résumé), blended with recency in code; user's slider sets the blend
  (100 = all résumé match, 0 = all recency; stored as the complementary `recency_weight`).
- **Job-hunt run is deterministic** (no LLM planning). LLM is used only for scoring + résumé suggestion.
- **Delete = soft delete** (status=archived, hidden from list). Pause/resume/archive all via
  `PATCH /workers/{id}/status`. No hard delete, no FK cascade.
- **Résumé = PDF upload** (no textarea); parsed server-side, stored as `resume_text`; AI pre-fills
  category/keywords at create (`POST /resumes/parse`).
- **score_jobs token caps** (Groq free = 12k TPM): `maxJobsToScore=30`, description≤400 chars,
  résumé≤3000 chars in the prompt. Tunable in `score_jobs.go`.

## To run
1. Apply DB schema to Postgres (migrations in `internal/migrations`, applied to a fresh DB).
2. RabbitMQ + Postgres up; config via env (see `internal/config`).
3. **`ai_primary = openai` or `groq`** — Claude's `GeneratePlan`/`Complete` are stubbed.
4. `TAVILY_API_KEY` set for the discovery `domain_search` source (YC source needs no key).
5. Run `cmd/api` (API + crons) and `cmd/executor` (workers). UI at http://localhost:8080/.
   Verify: `go build ./... && go vet ./... && go test ./...`.

## Open items / gaps (not bugs, known)
- **Claude planning stubbed** — use openai/groq as primary. OpenAI key was out of quota during
  testing (the 429); Groq is the working provider.
- **Tavily quota**: domain_search ≈ 12 calls/run × 4/day ≈ 1.4k/month — may exceed free tier; trim
  `searchQueries` or disable that source (YC carries discovery).
- **Discovery re-probes failed candidates** across runs (no persistent "tried" record). Bounded by
  cap+shuffle. Future: a `discovered` table to remember misses.
- **YC slug-guessing is low hit-rate** (expected for guessing); domain_search is higher yield.
- **Semantic/pgvector retrieval + fan-out scoring** = future, for when catalog is huge / on a paid
  LLM tier. Not needed at current scale.
- A few catalog display names are plain title-case (e.g. "Circleci", "Clickup"); cosmetic.

## Design docs
- `docs/job-search-subsystem-a-design.md` — the matching design.
- `docs/job-search-structured-matching-plan.md` — the structured-matching + lifecycle + PDF plan
  (already executed).
- `docs/implementation.md` — older phase plan (v1→v2 pivot, scheduler, etc.).
