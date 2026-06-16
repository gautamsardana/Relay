-- name: CreateWorker :one
INSERT INTO workers (worker_id, user_id, name, instructions, schedule, status, resume_url, next_run_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING worker_id, user_id, name, instructions, schedule, status, resume_url, next_run_at, created_at, updated_at;

-- name: GetWorkerByID :one
SELECT worker_id, user_id, name, instructions, schedule, status, resume_url, next_run_at, created_at, updated_at
FROM workers
WHERE worker_id = $1;

-- name: ListWorkersByUser :many
SELECT worker_id, user_id, name, instructions, schedule, status, resume_url, next_run_at, created_at, updated_at
FROM workers
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListDueWorkers :many
SELECT worker_id, user_id, name, instructions, schedule, status, resume_url, next_run_at, created_at, updated_at
FROM workers
WHERE status = 'active' AND next_run_at <= NOW();

-- name: UpdateWorkerNextRunAt :exec
UPDATE workers
SET next_run_at = $2, updated_at = NOW()
WHERE worker_id = $1;

-- name: UpdateWorkerStatus :exec
UPDATE workers
SET status = $2, updated_at = NOW()
WHERE worker_id = $1;
