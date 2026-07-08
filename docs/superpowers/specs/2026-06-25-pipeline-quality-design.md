# Job Pipeline Quality + Speed — Design

Date: 2026-06-25

## Problem

The job-hunt pipeline returns low-quality results, leaks roles above the seeker's
level, mishandles location, and is slow. Concretely:

1. **Location filter is broken for multi-city input.** `location_pref` is matched as
   one literal substring, so `"London, Amsterdam, Berlin"` matches no job location and
   returns zero results. (`internal/tools/job_search.go`)
2. **No experience-level concept.** The only seniority guard is a title regex that does
   not include "Senior" and cannot see "8+ years" in a description. Mid-level searches
   surface senior / 8-year roles.
3. **Recency floats junk.** Kept = `w·recency + (1-w)·fit ≥ 0.5`, so a fresh poor-fit
   job (fit 0.2, recency 1.0) passes. Bad matches surface because they are recent.
4. **Slow.** Weak filtering sends ~100 jobs to the LLM → up to 5 throttled batches
   (`batchDelay=35s`) → ~140s of pure sleeping per run.

Out of scope (deferred, agreed): matches board, edit-worker, expanding the catalog with
European companies. This change is only about correctness + speed of the jobs returned.

## Changes

### 1. New worker field: `level`

Enum-ish TEXT, default `any`. Values: `intern | junior | mid | senior | staff_plus | any`.
Plumbed like the existing `location_pref`: migration `02_create_workers.sql` (after
`location_pref`), `models.Worker.Level`, sqlc queries (all SELECT/RETURNING + CreateWorker
insert, in table-column order), `adapter.go` both directions, `planner.CreateWorker` param,
API handler request struct + call, `tools.ExecutionContext.Level`, executor
`buildExecutionContext`, and a frontend dropdown in `workerNew.js`.

### 2. Deterministic level enforcement (in `job_search` filter)

Replaces the hardcoded `overSeniorRe`. Two gates, applied only when level != `any`:

**Title-rank ladder.** `titleRank(title)` returns a rank from keywords:
- intern: intern, internship, co-op, new grad/graduate, university grad
- junior: junior, jr, associate, entry level, "engineer i" (roman numeral I)
- mid: (default when no signal)
- senior: senior, sr
- lead/staff: staff, principal, lead, director, distinguished, fellow, vp, svp, evp,
  chief, head of, vice president, manager

Per-level allowed ceiling; drop if `titleRank(title) > ceiling`. For mid+ also drop intern
titles; for senior/staff also drop junior titles (configured per level, see table).

**Years gate.** `minYearsRequired(description)` parses the smallest required years from
patterns: `N+ years`, `N-M years` (take N), `at least N years`, `minimum [of] N years`,
`N years of experience`. Drop if it exceeds the level's ceiling.

| Level | Title ceiling | Also exclude | Years ceiling |
|-------|---------------|--------------|---------------|
| intern | intern | senior, staff, mid-implied higher | 1 |
| junior | junior | senior, staff | 2 |
| mid | mid | senior, staff, intern | 5 |
| senior | senior | staff, intern, junior | 10 |
| staff_plus | staff | intern, junior | none |
| any | (no filtering) | — | — |

### 3. Location fix

`matchesLocation(jobLocation, locationPref)`: empty pref → match all. Otherwise split pref
on commas into lowercased terms; match if the job location contains any term. `remote` is a
term like any other (matches "Remote", "Remote - US", etc). Strict: no implicit remote for a
city search.

### 4. Fit floor

`fitFloor = 0.4`. In `score_jobs`, when scoring ran (RecencyWeight < 100), drop any job with
`fit < fitFloor` before applying the blended `scoreThreshold`. Recency can no longer float a
low-fit job.

### 5. Scoring prompt + speed

- `buildIntent` includes the explicit target level and location preference so the LLM is a
  consistent backup gate, not the primary one.
- No batch-param changes needed for correctness; the smaller post-filter candidate set is the
  speed win (most runs become 1–2 batches). Revisit `batchDelay`/`batchSize` only if still slow.

### 6. Consistency

`normWorker` (web/js/api.js) maps `location_pref` and `level`; worker detail "Search" card
shows them.

## Testing

- Unit tests in `internal/tools`: `titleRank`, `minYearsRequired`, `matchesLocation`, and
  `filterJobs` level/location behavior (mid drops "8+ years" Software Engineer; multi-city
  matches any city; senior drops Staff).
- Fit-floor behavior: a high-recency low-fit job is dropped.
- `go build ./... && go vet ./... && go test ./...`; `node --check` on changed JS.
