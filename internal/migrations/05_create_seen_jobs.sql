CREATE TABLE seen_jobs (
    id         UUID PRIMARY KEY,
    worker_id  UUID NOT NULL REFERENCES workers(worker_id),
    company_id TEXT NOT NULL,
    job_id     TEXT NOT NULL,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(worker_id, company_id, job_id)
);

CREATE INDEX idx_seen_jobs_worker_id ON seen_jobs(worker_id);
