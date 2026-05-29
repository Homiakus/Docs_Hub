# Architecture

## Цель

Docs Hub Next — не очередной клон Confluence. Это лёгкая self-hosted Markdown-first wiki для команд, совместимая по мышлению с Obsidian: wiki-links, backlinks, graph, tags, Markdown, импорт vault.

## Почему SQLite-first

SQLite даёт простоту одного файла, но решает проблемы JSON-хранилища:

- транзакции;
- индексы;
- FTS5;
- WAL;
- нормальные миграции;
- версии статей без раздувания одного JSON.

PostgreSQL можно добавить позже через storage-интерфейс, но начинать с него не обязательно.

## Пакеты

```text
cmd/docshub          точка входа
internal/config      env-конфиг
internal/db          SQLite и миграции
internal/auth        Argon2id password hashing
internal/markdownx   Markdown, sanitizer, wiki-links, tags
internal/httpapp     HTTP routes, handlers, ACL checks
internal/web         embedded templates/static
```

## Данные

Основные таблицы:

- `users`, `groups`, `group_members`;
- `articles`;
- `article_versions`;
- `tags`, `article_tags`;
- `links`;
- `acl_users`, `acl_groups`;
- `files`, `article_files`;
- `sessions`;
- `audit_events`;
- `article_fts`.

## Что делать дальше

1. Добавить OIDC/SAML.
2. Добавить нормальный UI управления группами и ACL.
3. Добавить импорт Obsidian vault.
4. Добавить вложения через content-addressed storage.
5. Добавить comments/review workflow.
6. Добавить WebSocket/CRDT только после стабилизации обычного editor flow.
