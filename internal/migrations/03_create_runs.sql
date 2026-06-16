CREATE TYPE run_status AS ENUM ('init', 'processing', 'success', 'failed');

CREATE TABLE runs (
    run_id      UUID PRIMARY KEY,
    worker_id   UUID NOT NULL REFERENCES workers(worker_id),
    status      run_status NOT NULL DEFAULT 'init',
    error       TEXT,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_runs_worker_id ON runs(worker_id);
CREATE INDEX idx_runs_status ON runs(status);
