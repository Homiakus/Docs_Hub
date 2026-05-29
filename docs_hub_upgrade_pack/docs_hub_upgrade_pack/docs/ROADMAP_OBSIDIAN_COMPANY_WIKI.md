# Roadmap: Obsidian-like корпоративная Wiki

## Позиционирование

Не делать “ещё один Confluence”. Лучшее позиционирование:

> Lightweight Obsidian-compatible company wiki: Markdown-first, self-hosted, ACL, audit, import/export vault, graph, search, simple deployment.

## MVP 1.5 — стабилизация фундамента

### Storage

- SQLite first.
- JSON остаётся только как import/export format.
- Миграции через SQL-файлы.
- Article versions в отдельной таблице.

### Markdown

- `goldmark` renderer.
- `bluemonday` sanitizer.
- Extractors: wiki-links, tags, headings, tasks, frontmatter.

### Search

- SQLite FTS5.
- ACL-filtered results.
- Search snippets.

### Files

- `sha256` dedup.
- `files` + `article_files`.
- Storage backend interface: local first, S3/MinIO later.

## Version 2.0 — corporate wiki

### Permissions

- RBAC: read, write, admin, publish.
- Spaces/projects.
- Group inheritance.
- OIDC/SSO.

### Audit

- structured audit events;
- export audit log;
- retention policy;
- immutable event append mode.

### Workflow

- draft/published states;
- review requests;
- comments;
- mentions;
- change diff.

## Version 2.5 — Obsidian-like UX

### Graph

- Global graph.
- Local graph for article.
- Tag graph.
- Orphan pages.
- Broken links.

### Properties and views

- YAML frontmatter.
- Typed properties.
- Query blocks.
- Dynamic tables.

Example:

```markdown
```query
from: articles
where: tag = "onboarding" and status = "active"
select: title, owner, updated_at
```
```

### Import/export

- Import Obsidian vault folder.
- Preserve `[[links]]`, tags, attachments.
- Export as Markdown folder.
- Optional Git sync.

## What not to build too early

- Full plugin marketplace.
- Full real-time CRDT editor.
- Native mobile apps.
- Full Canvas clone.
- Enterprise SAML before OIDC.

