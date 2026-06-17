-- name: ListActiveCompanies :many
SELECT company_id, name, ats, slug, active, created_at
FROM companies
WHERE active = TRUE
ORDER BY name;

-- name: UpsertCompany :exec
-- Used to seed the catalog. On conflict we refresh the display name but leave
-- `active` untouched, so a company an admin disabled stays disabled across restarts.
INSERT INTO companies (company_id, name, ats, slug)
VALUES ($1, $2, $3, $4)
ON CONFLICT (ats, slug) DO UPDATE SET name = EXCLUDED.name;
