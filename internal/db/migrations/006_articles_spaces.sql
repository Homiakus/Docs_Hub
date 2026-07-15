ALTER TABLE articles ADD COLUMN organization_id INTEGER DEFAULT 1;
ALTER TABLE articles ADD COLUMN space_id INTEGER DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_articles_space ON articles(space_id);
CREATE INDEX IF NOT EXISTS idx_articles_org ON articles(organization_id);
