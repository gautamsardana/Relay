-- Active: 1777597968626@@127.0.0.1@5432@postgres
-- name: CreateWorkflow :one
INSERT INTO workflows (workflow_id, request, status)
VALUES ($1, $2, $3)
RETURNING workflow_id, request, status, created_at, updated_at;

-- name: GetWorkflowById :one
SELECT workflow_id, request, status, created_at, updated_at
FROM workflows
WHERE workflow_id = $1;

-- name: ListWorkflows :many
SELECT workflow_id, request, status, created_at, updated_at
FROM workflows
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateWorkflowStatus :exec
UPDATE workflows
SET status = $2, updated_at = now()
WHERE workflow_id = $1;