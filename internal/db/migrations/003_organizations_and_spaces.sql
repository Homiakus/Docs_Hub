CREATE TABLE IF NOT EXISTS organizations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    settings_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS spaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    parent_id INTEGER REFERENCES spaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    owner_group_id INTEGER,
    default_visibility TEXT NOT NULL DEFAULT 'space_members',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS space_members (
    space_id INTEGER NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL, -- 'user' or 'group'
    subject_id INTEGER NOT NULL,
    role TEXT NOT NULL, -- 'space_owner', 'space_admin', 'publisher', 'editor', 'contributor', 'reader'
    PRIMARY KEY (space_id, subject_type, subject_id)
);

CREATE TABLE IF NOT EXISTS role_bindings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    scope_type TEXT NOT NULL, -- 'organization', 'space', 'document'
    scope_id INTEGER NOT NULL,
    subject_type TEXT NOT NULL, -- 'user' or 'group'
    subject_id INTEGER NOT NULL,
    role TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS document_permissions (
    document_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL, -- 'user' or 'group'
    subject_id INTEGER NOT NULL,
    permission TEXT NOT NULL, -- 'read', 'edit', 'comment', 'review', 'publish', 'manage_acl', 'archive', 'delete', 'export'
    effect TEXT NOT NULL DEFAULT 'allow', -- 'allow' or 'deny'
    PRIMARY KEY (document_id, subject_type, subject_id, permission)
);

-- Seed default organization and default space if empty
INSERT OR IGNORE INTO organizations(id, name, slug, settings_json, created_at)
VALUES (1, 'Default Organization', 'default', '{}', datetime('now'));

INSERT OR IGNORE INTO spaces(id, organization_id, parent_id, name, slug, description, default_visibility, created_at, updated_at)
VALUES (1, 1, NULL, 'General', 'general', 'General workspace for company knowledge base', 'space_members', datetime('now'), datetime('now'));
