CREATE TABLE IF NOT EXISTS attachment_pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    page_number INTEGER NOT NULL,
    extracted_text TEXT NOT NULL DEFAULT '',
    thumbnail_key TEXT NOT NULL DEFAULT '',
    extraction_status TEXT NOT NULL DEFAULT 'pending',
    ocr_used INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(file_id, page_number)
);

ALTER TABLE files ADD COLUMN organization_id INTEGER REFERENCES organizations(id);
ALTER TABLE files ADD COLUMN status TEXT DEFAULT 'ready';
ALTER TABLE files ADD COLUMN width INTEGER DEFAULT 0;
ALTER TABLE files ADD COLUMN height INTEGER DEFAULT 0;
ALTER TABLE files ADD COLUMN duration_sec REAL DEFAULT 0;
ALTER TABLE files ADD COLUMN page_count INTEGER DEFAULT 0;
ALTER TABLE files ADD COLUMN alt_text TEXT DEFAULT '';
ALTER TABLE files ADD COLUMN caption TEXT DEFAULT '';
