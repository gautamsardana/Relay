-- name: CreateRun :one
INSERT INTO runs (run_id, worker_id, status)
VALUES ($1, $2, $3)
RETURNING run_id, worker_id, status, error, started_at, finished_at;

-- name: GetRunByID :one
SELECT run_id, worker_id, status, error, started_at, finished_at
FROM runs
WHERE run_id = $1;

-- name: ListRunsByWorker :many
SELECT run_id, worker_id, status, error, started_at, finished_at
FROM runs
WHERE worker_id = $1
ORDER BY started_at DESC;

-- name: UpdateRunStatus :exec
UPDATE runs
SET status = sqlc.arg(status)::run_status,
    error = sqlc.arg(error),
    finished_at = CASE WHEN sqlc.arg(status)::run_status IN ('success', 'failed') THEN NOW() ELSE NULL END
WHERE run_id = sqlc.arg(run_id);
