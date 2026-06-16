CREATE TYPE worker_status AS ENUM ('active', 'paused', 'archived');

CREATE TABLE workers (
    worker_id    UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(user_id),
    name         TEXT NOT NULL,
    instructions TEXT NOT NULL,
    schedule     TEXT NOT NULL,
    status       worker_status NOT NULL DEFAULT 'active',
    resume_url   TEXT,
    next_run_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workers_user_id ON workers(user_id);
CREATE INDEX idx_workers_status_next_run ON workers(status, next_run_at);
