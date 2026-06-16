CREATE TYPE workflow_status AS ENUM ('init', 'processing', 'success', 'failed');

CREATE TABLE workflow_runs (
    run_id      UUID PRIMARY KEY,
    worker_id   UUID NOT NULL REFERENCES workers(worker_id),
    status      workflow_status NOT NULL DEFAULT 'init',
    error       TEXT,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_workflow_runs_worker_id ON workflow_runs(worker_id);
CREATE INDEX idx_workflow_runs_status ON workflow_runs(status);
