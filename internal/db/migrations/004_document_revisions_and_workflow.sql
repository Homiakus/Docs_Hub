CREATE TABLE IF NOT EXISTS document_revisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    revision_no INTEGER NOT NULL,
    source_format TEXT NOT NULL DEFAULT 'markdown',
    content TEXT NOT NULL,
    rendered_html TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    change_summary TEXT NOT NULL DEFAULT '',
    created_by INTEGER NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS document_reviews (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    revision_id INTEGER NOT NULL REFERENCES document_revisions(id) ON DELETE CASCADE,
    reviewer_id INTEGER NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'approved', 'rejected'
    comment TEXT NOT NULL DEFAULT '',
    decided_at TEXT
);

-- Add workflow columns to articles table if missing
ALTER TABLE articles ADD COLUMN stable_key TEXT DEFAULT '';
ALTER TABLE articles ADD COLUMN status TEXT DEFAULT 'published';
ALTER TABLE articles ADD COLUMN classification TEXT DEFAULT 'internal';
ALTER TABLE articles ADD COLUMN language TEXT DEFAULT 'ru';
ALTER TABLE articles ADD COLUMN lock_version INTEGER DEFAULT 1;
ALTER TABLE articles ADD COLUMN review_due_at TEXT;
ALTER TABLE articles ADD COLUMN expires_at TEXT;
ALTER TABLE articles ADD COLUMN archived_at TEXT;
