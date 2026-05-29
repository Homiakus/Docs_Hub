# Docs Hub Upgrade Pack

Этот пакет — набор практических доработок для `Homiakus/Docs_Hub`, подготовленный после аудита архитектуры.

Важно: прямое `git clone` из sandbox не прошёл из-за DNS (`Could not resolve host: github.com`), поэтому изменения оформлены как применимый upgrade-kit: новые файлы, SQL-схема, Markdown-рендерер, документы архитектурных решений и план интеграции.

## Что входит

1. `docs/AUDIT_DEEP.md` — глубокий аудит: где велосипед, где дублирование, где упростить.
2. `docs/ROADMAP_OBSIDIAN_COMPANY_WIKI.md` — план превращения в Obsidian-like корпоративную wiki.
3. `docs/ADR/*` — архитектурные решения.
4. `internal/markdown/*` — замена самописного Markdown-рендера на `goldmark + bluemonday`.
5. `internal/store/schema.sql` — нормальная SQLite-first схема.
6. `internal/store/json_import.go` — заготовка миграции из текущего `storage.json`.
7. `cmd/migrate-json/main.go` — CLI для миграции JSON -> SQLite.
8. `patches/go_mod_additions.patch` — зависимости, которые надо добавить в `go.mod`.
9. `PATCH_README.md` — порядок применения.

## Быстрый порядок интеграции

```bash
# 1. Скопировать файлы из этого пакета в корень репозитория Docs_Hub
rsync -av docs_hub_upgrade_pack/ /path/to/Docs_Hub/

# 2. Добавить зависимости
cd /path/to/Docs_Hub
go get github.com/yuin/goldmark@latest
go get github.com/yuin/goldmark/extension@latest
go get github.com/microcosm-cc/bluemonday@latest
go get modernc.org/sqlite@latest

# 3. Проверить новый Markdown-пакет
go test ./internal/markdown

# 4. Сгенерировать SQLite из текущего JSON
DATA_FILE=./data/storage.json go run ./cmd/migrate-json --from ./data/storage.json --to ./data/docs-hub.db
```

