-- name: CreateStep :one
INSERT INTO steps (step_id, workflow_id, step_number, tool, description, input, output, status, retry_count, error)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING step_id, workflow_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at;

-- name: GetStepById :one
SELECT step_id, workflow_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at
FROM steps
WHERE step_id = $1;

-- name: ListStepsByWorkflow :many
SELECT step_id, workflow_id, step_number, tool, description, input, output, status, retry_count, error, created_at, updated_at
FROM steps
WHERE workflow_id = $1
ORDER BY step_number ASC;

-- name: UpdateStepStatus :exec
UPDATE steps
SET status = $2, updated_at = now()
WHERE step_id = $1;