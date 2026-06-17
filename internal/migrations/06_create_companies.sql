CREATE TABLE companies (
    company_id  UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    ats         TEXT NOT NULL,            -- 'greenhouse' | 'lever' | 'ashby'
    slug        TEXT NOT NULL,            -- board id on that ATS, e.g. "stripe"
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ats, slug)
);

CREATE INDEX idx_companies_active ON companies(active);
