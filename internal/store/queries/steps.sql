-- name: CreateStep :one
INSERT INTO steps (step_id, run_id, step_number, tool, description, input, output, status, retry_count, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING step_id, run_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at;


-- name: ListStepsByRun :many
SELECT step_id, run_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at
FROM steps
WHERE run_id = $1
ORDER BY step_number ASC;

-- name: UpdateStepStatus :exec
UPDATE steps
SET status = $2, error = $3, updated_at = now()
WHERE step_id = $1;

-- name: ClaimStep :one
UPDATE steps
SET status = 'processing', updated_at = now()
WHERE step_id = $1 AND status = 'pending'
RETURNING step_id, run_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at;

-- name: GetStuckSteps :many
SELECT step_id, run_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at
FROM steps
WHERE status = 'processing' AND updated_at < NOW() - make_interval(secs => sqlc.arg(timeout_seconds)::int);

-- name: CancelUnstartedSteps :exec
UPDATE steps
SET status = 'cancelled', error = $2, updated_at = now()
WHERE run_id = $1 AND status IN ('init', 'pending');

-- name: MarkStepPending :exec
UPDATE steps
SET status = 'pending', updated_at = now()
WHERE step_id = $1;

-- name: ResetStepToPending :exec
UPDATE steps
SET status = 'pending', error = NULL, updated_at = now()
WHERE step_id = $1;

-- name: IncrementStepRetryCount :exec
UPDATE steps
SET retry_count = retry_count + 1, updated_at = now()
WHERE step_id = $1;

-- name: UpdateStepAsCompleted :exec
UPDATE steps
SET status = $2, output = $3, updated_at = now()
WHERE step_id = $1;

-- name: GetStepByRunAndNumber :one
SELECT step_id, run_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at
FROM steps
WHERE run_id = $1 AND step_number = $2
LIMIT 1;
