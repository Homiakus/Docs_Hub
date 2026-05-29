# Docs Hub Next

Новая версия Docs Hub: лёгкая корпоративная wiki в стиле Obsidian, но с серверной моделью доступа, аудитом и нормальной базой данных.

## Что уже заложено

- Go 1.23, один бинарный файл.
- SQLite WAL вместо самописной JSON-БД.
- Markdown pipeline: `goldmark` + GFM + подсветка кода + `bluemonday` sanitizer.
- Wiki-links `[[slug]]` и `[[slug|label]]`.
- Теги `#tag`.
- Backlinks через таблицу `links`, без парсинга на лету.
- Graph API `/api/graph`.
- Article versions через `article_versions`.
- FTS5 search через `article_fts`.
- Users, groups, ACL tables.
- Server-side sessions.
- Argon2id password hashing.
- Embedded templates/static через `embed`.
- Docker Compose.

## Быстрый старт

```bash
cp compose.yaml compose.local.yaml
# обязательно поменяйте ADMIN_PASSWORD и SESSION_SECRET

docker compose up -d --build
```

Локальный запуск:

```bash
ADMIN_PASSWORD='change-me-now' \
SESSION_SECRET='replace-with-random-string' \
go run ./cmd/docshub
```

Миграция старого `storage.json` в SQLite:

```bash
go run ./cmd/migrate-json --from ./data/storage.json --to ./data/docshub.db
```

Открыть:

```text
http://localhost:8080
```

## Важное замечание

Этот пакет — новая кодовая база/скелет продукта, а не прямой patch поверх старого `main.go`. Его правильнее использовать как основу для Docs Hub 2.0, постепенно перенося функции из старой версии.

## Зачем так

Старая версия хороша как MVP, но там слишком много ответственности в одном `main.go`, JSON-БД и ручная логика Markdown/HTML. Здесь архитектура сразу разделена на пакеты и заложены расширяемые точки: БД, Markdown, auth, templates, API, graph.
