-- The original index was contentless, which prevents reliable UPDATE/DELETE
-- operations during autosave. Rebuild it as a regular FTS5 table so each
-- document can be atomically re-indexed together with its source row.
DROP TABLE IF EXISTS article_fts;

CREATE VIRTUAL TABLE article_fts USING fts5(title, slug, content, tags);

INSERT INTO article_fts(rowid, title, slug, content, tags)
SELECT
  a.id,
  a.title,
  a.slug,
  a.content,
  coalesce(group_concat(t.name, ' '), '')
FROM articles a
LEFT JOIN article_tags at ON at.article_id = a.id
LEFT JOIN tags t ON t.id = at.tag_id
WHERE a.deleted_at IS NULL
GROUP BY a.id, a.title, a.slug, a.content;
