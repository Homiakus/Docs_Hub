CREATE TABLE categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  slug TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  nav_order INTEGER NOT NULL DEFAULT 100,
  is_visible INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

ALTER TABLE articles ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL;

INSERT OR IGNORE INTO categories(name, slug, description, nav_order, is_visible, created_at, updated_at)
SELECT name, name, '', row_number() OVER (ORDER BY name) * 10, 1, datetime('now'), datetime('now')
FROM tags;

UPDATE articles
SET category_id = (
  SELECT c.id
  FROM article_tags at
  JOIN tags t ON t.id = at.tag_id
  JOIN categories c ON c.slug = t.name
  WHERE at.article_id = articles.id
  ORDER BY t.name
  LIMIT 1
)
WHERE category_id IS NULL;

CREATE INDEX idx_articles_category ON articles(category_id);
CREATE INDEX idx_categories_visible_order ON categories(is_visible, nav_order, name);
