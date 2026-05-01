CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE step_status AS ENUM ('pending', 'processing', 'success', 'failed');

CREATE TABLE steps (
    step_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES workflows(workflow_id),
    step_number  INT NOT NULL,
    tool         TEXT NOT NULL,
    description  TEXT NOT NULL,
    input        JSONB NOT NULL DEFAULT '{}',
    output       JSONB,
    status       step_status NOT NULL DEFAULT 'pending',
    retry_count  INT NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_steps_workflow_id ON steps(workflow_id);
CREATE INDEX idx_steps_status ON steps(status);