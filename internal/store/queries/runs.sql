-- name: CreateRun :one
INSERT INTO workflow_runs (run_id, worker_id, status)
VALUES ($1, $2, $3)
RETURNING run_id, worker_id, status, error, started_at, finished_at;

-- name: GetRunByID :one
SELECT run_id, worker_id, status, error, started_at, finished_at
FROM workflow_runs
WHERE run_id = $1;

-- name: ListRunsByWorker :many
SELECT run_id, worker_id, status, error, started_at, finished_at
FROM workflow_runs
WHERE worker_id = $1
ORDER BY started_at DESC;

-- name: UpdateRunStatus :exec
UPDATE workflow_runs
SET status = $2, error = $3, finished_at = CASE WHEN $2 IN ('success', 'failed') THEN NOW() ELSE NULL END
WHERE run_id = $1;
