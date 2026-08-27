-- Migration 009: Anchored Comments schema for collaborative document annotations

CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id INTEGER NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    author_id INTEGER NOT NULL REFERENCES users(id),
    parent_id INTEGER REFERENCES comments(id) ON DELETE CASCADE,
    base_revision_id INTEGER NOT NULL DEFAULT 1,
    start_offset INTEGER NOT NULL DEFAULT 0,
    end_offset INTEGER NOT NULL DEFAULT 0,
    quote_exact TEXT NOT NULL DEFAULT '',
    quote_prefix TEXT NOT NULL DEFAULT '',
    quote_suffix TEXT NOT NULL DEFAULT '',
    ast_node_kind TEXT NOT NULL DEFAULT '',
    ast_path TEXT NOT NULL DEFAULT '',
    heading_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open', -- 'open', 'resolved', 'orphaned'
    body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_comments_document_id ON comments(document_id, status);
CREATE INDEX IF NOT EXISTS idx_comments_parent_id ON comments(parent_id);
