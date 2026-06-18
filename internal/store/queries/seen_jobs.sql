-- name: ListSeenJobKeys :many
-- Returns the (company_id, job_id) pairs this worker has already been shown,
-- so job_search can filter them out of a new run.
SELECT company_id, job_id
FROM seen_jobs
WHERE worker_id = $1;

-- name: RecordSeenJob :exec
-- Marks a job as shown to this worker. Idempotent via the unique constraint,
-- so a retried score_jobs step can re-record without error.
INSERT INTO seen_jobs (id, worker_id, company_id, job_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (worker_id, company_id, job_id) DO NOTHING;
