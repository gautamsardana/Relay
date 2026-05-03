-- Active: 1777842247087@@127.0.0.1@5432@postgres
CREATE TYPE workflow_status AS ENUM ('init', 'processing', 'success', 'failed');

CREATE TABLE workflows (
    workflow_id  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request      TEXT NOT NULL,
    status       workflow_status NOT NULL DEFAULT 'init',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflows_workflow_id ON workflows(workflow_id);
CREATE INDEX idx_workflows_status ON workflows(status);