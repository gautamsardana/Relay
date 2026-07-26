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

## Recent additions (latest first)
- **Description truncation bug (was defeating the whole level gate).** ATS adapters truncated
  descriptions to 1500 chars *at fetch time*, but requirements ("Over 8+ years…") usually sit
  near the END of a posting — so the years gate was reading text that didn't contain the
  requirement. A Middesk "Product Designer" demanding 8 years passed a mid-level search for
  exactly this reason. Now `cleanText` doesn't truncate; `job_search` filters on the full text
  and calls `ats.TruncateDescription` only when writing step output.
- **Profile-based matching (replaces résumé-text scoring).** At creation the LLM extracts a
  full profile from the résumé — category, 12-20 keywords, `level`, `years_experience` — and
  pre-fills the form; the user only reviews/edits. `score_jobs` now scores against that
  compact profile (`buildProfile`) instead of raw résumé text.
- **PDF extraction fixed.** `GetPlainText` dropped spaces on many résumés, producing
  "CenterforDigitalExperiences" and silently degrading every score. `resume.ExtractText` now
  rebuilds word spacing from glyph X-positions (`GetTextByRow`), with a fallback.
- **Level enforcement is two-stage.** Deterministic gate first (cheap, saves tokens): explicit
  senior/staff title words + hard "N+ years" numbers only — no attempt to regex fuzzy phrasing.
  Then `passesLevelBackstop` uses `seniority`/`min_years` that the LLM returns alongside its
  score (no extra call) to drop what the regex can't read ("seasoned", "you'll mentor").
- **Level band fix (was returning ~0 jobs).** Un-suffixed titles ("Product Designer") default to
  rankMid and were being rejected by junior/mid searches. They're level-ambiguous, so every
  level except intern now lets them through. Live design+junior pool went 1 → 18.
- **Keywords no longer hard-filter** when a category is set (exact whole-word phrase matching
  tanked recall: "\bProduct Design\b" misses "Product Designer"). They feed the scorer instead.
- Job rows now show the role's **location**.
- **Experience level** is now a first-class worker field (`level`: intern/junior/mid/
  senior/staff_plus/any). Enforced deterministically in `internal/tools/levels.go`:
  a title-rank ladder (`titleRank`) + a years-of-experience parser (`minYearsRequired`,
  catches "8+ years" in the description) drop roles above the target level *before*
  scoring. Replaces the old hardcoded `overSeniorRe`. Dropdown in the create form.
- **Location filter fixed**: `matchesLocation` splits `location_pref` on commas and
  matches ANY term (multi-city "London, Amsterdam, Berlin" now works; was matched as one
  literal substring → always zero). Catalog is still US-centric, so EU coverage is thin
  until we add European companies (deferred).
- **Fit floor** (`fitFloor=0.4` in score_jobs.go): when scoring runs, a job below the
  floor is dropped regardless of recency — stops fresh-but-poor-fit jobs floating up.
- Scoring prompt/intent now includes the explicit target level + location preference.
- `titleRank` tuned: bare "manager" is NOT a seniority signal (Product Manager = mid); only
  "Engineering/Senior Manager" + Director/VP/Head/Staff/Principal/Lead count as staff. A
  junior/associate modifier caps the rank down ("Associate Product Manager" = junior).
- **Years-parser precision fix**: `minYearsRequired` was matching stray numbers
  ("16-17 year olds", "10 years in the UK") and falsely dropping good mid roles. Now only
  counts anchored requirements (`N+ years`, `N years ... experience`, `at least/minimum N
  years`) with plural "years". Verified live against Monzo's board.
- **Category-precision fix**: `categoryExcludeDepartments` drops non-IC functions that carry
  "engineering" in their dept name (Celonis "Value Engineering" pre-sales, Sales/Solutions
  Engineering, Customer Success) so they stop leaking into a software-engineering search.
- **European companies** added to `companies.json` (now 176): 25 verified EU boards
  (Monzo, Adyen, Qonto, N26, Mollie, Doctolib, Wolt, Pleo, GoCardless, Celonis, …) probed
  live against the ATS APIs. Location search now returns real EU results.
- **Scoring** is now batched + strict: scores up to 100 candidates/run in throttled
  batches (`scoreBudget`/`batchSize`/`batchDelay` in score_jobs.go) to beat the trickle;
  drops anything below 0.5 (`scoreThreshold`); LLM returns a one-line `reason` per job
  (shown italic on each result row).
- **Seniority filter**: `overSeniorRe` in job_search.go hard-excludes Principal/Staff/
  Lead/Director/VP/etc titles (recency was floating low-fit senior roles past the threshold).
- **Location filter**: worker has `location_pref` (new column + form field); job_search drops
  jobs whose location doesn't contain it ("remote" / city / country, substring match).
- **Apply flow**: clicking Apply marks the job pending; on tab-refocus the row shows an inline
  "Applied? Yes/No" confirm; only Yes marks it applied (localStorage by job_id).
- **Static files** served with no-cache headers (was causing stale-UI confusion).
- **Schema note:** `workers` gained `category`, `keywords`, `location_pref` — re-apply schema.

## Still open (build order)
1. **Matches board** — persist every surfaced job to a real table (job + score + reason +
   triage state), one consolidated per-worker view with filter/sort + in-app description +
   server-side Applied/Saved/Dismissed. Today matches live only inside each run's step
   output and `seen_jobs` stores keys only. (Designed, deferred — biggest UX leap.)
2. **Edit worker** (change keywords/category/location/level/slider without recreate).
3. Email notifications (digest of new matches + link to run page).
4. Deploy (Dockerfile + compose or PaaS; needs a migration runner). 5. Commit (nothing committed yet).

## Design docs
- `docs/job-search-subsystem-a-design.md` — the matching design.
- `docs/job-search-structured-matching-plan.md` — the structured-matching + lifecycle + PDF plan
  (already executed).
- `docs/implementation.md` — older phase plan (v1→v2 pivot, scheduler, etc.).
