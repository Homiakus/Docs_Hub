-- M1 compatibility migration for the target Organization -> Domain -> Project model.
-- `spaces` remains the physical project table during the compatibility window.
-- This migration is additive: legacy ACL/auth tables remain untouched until the
-- SecureAccess migration has been verified.

CREATE TABLE IF NOT EXISTS domains (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    stable_key TEXT NOT NULL UNIQUE,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    security_workspace_id TEXT,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    icon TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL DEFAULT 'migration',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (organization_id, slug)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_domains_security_workspace
    ON domains(security_workspace_id)
    WHERE security_workspace_id IS NOT NULL AND security_workspace_id <> '';
CREATE INDEX IF NOT EXISTS idx_domains_organization_status_sort
    ON domains(organization_id, status, sort_order, name);

-- Seed one compatibility Domain for every existing Organization. Stable keys
-- are immutable and deliberately independent from mutable slugs/names.
INSERT OR IGNORE INTO domains(
    stable_key,
    organization_id,
    security_workspace_id,
    slug,
    name,
    description,
    icon,
    status,
    sort_order,
    created_by,
    created_at,
    updated_at
)
SELECT
    'legacy-domain-' || CAST(o.id AS TEXT),
    o.id,
    NULL,
    CASE WHEN o.id = 1 THEN 'general' ELSE 'general-' || CAST(o.id AS TEXT) END,
    'General',
    'Compatibility domain for existing projects',
    '',
    'active',
    0,
    'migration',
    datetime('now'),
    datetime('now')
FROM organizations o;

-- Keep the old `spaces` table operational while giving every existing row a
-- Domain/Project identity. Columns stay nullable during M1 so legacy writers
-- do not break before the Project repository is switched over.
ALTER TABLE spaces ADD COLUMN domain_id INTEGER REFERENCES domains(id) ON DELETE RESTRICT;
ALTER TABLE spaces ADD COLUMN stable_key TEXT;
ALTER TABLE spaces ADD COLUMN security_workspace_id TEXT;
ALTER TABLE spaces ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'inherit' CHECK (access_mode IN ('inherit', 'restricted'));
ALTER TABLE spaces ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'archived'));
ALTER TABLE spaces ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

UPDATE spaces
SET domain_id = (
        SELECT d.id
        FROM domains d
        WHERE d.organization_id = spaces.organization_id
        ORDER BY d.id
        LIMIT 1
    )
WHERE domain_id IS NULL;

UPDATE spaces
SET stable_key = 'legacy-project-' || CAST(id AS TEXT)
WHERE stable_key IS NULL OR stable_key = '';

CREATE INDEX IF NOT EXISTS idx_spaces_domain_status_sort
    ON spaces(domain_id, status, sort_order, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_spaces_stable_key
    ON spaces(stable_key)
    WHERE stable_key IS NOT NULL AND stable_key <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_spaces_security_workspace
    ON spaces(security_workspace_id)
    WHERE security_workspace_id IS NOT NULL AND security_workspace_id <> '';
