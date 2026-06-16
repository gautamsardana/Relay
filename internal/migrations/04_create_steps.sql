-- Active: 1777842247087@@127.0.0.1@5432@postgres
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

DROP TABLE IF EXISTS steps;
DROP TYPE IF EXISTS step_status;

CREATE TYPE step_status AS ENUM ('init', 'pending', 'processing', 'success', 'failed', 'cancelled');

CREATE TABLE steps (
    step_id      UUID PRIMARY KEY,
    run_id       UUID NOT NULL REFERENCES runs(run_id),
    step_number  INT NOT NULL,
    tool         TEXT NOT NULL,
    description  TEXT NOT NULL,
    input        JSONB NOT NULL DEFAULT '{}',
    output       JSONB NOT NULL DEFAULT '{}',
    status       step_status NOT NULL DEFAULT 'init',
    retry_count  INT NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_steps_run_id ON steps(run_id);
CREATE INDEX idx_steps_status ON steps(status);