# Как применять этот пакет к Docs Hub

## 1. Не переписывать всё сразу

Этот пакет специально сделан как инкрементальная доработка. Сначала добавляются новые пакеты и документы, затем постепенно заменяются старые участки `main.go`.

## 2. Порядок безопасного рефакторинга

### Этап A — Markdown

1. Добавить `internal/markdown`.
2. Добавить зависимости из `patches/go_mod_additions.patch`.
3. Найти текущую функцию `RenderMarkdown` в `main.go`.
4. Заменить тело на вызов:

```go
html, err := markdown.DefaultRenderer().Render(article.Content)
if err != nil {
    // fallback: escaped plaintext
}
```

Лучше не менять сигнатуру сразу. Сделай thin-wrapper в `main.go`, чтобы минимизировать diff.

### Этап B — Модель данных

1. Добавить `internal/store/schema.sql`.
2. Добавить CLI `cmd/migrate-json`.
3. Сначала использовать SQLite только для миграционного теста.
4. После проверки перенести runtime storage.

### Этап C — Разделение main.go

Сначала переносить без изменения поведения:

- auth/session -> `internal/auth`
- markdown -> `internal/markdown`
- storage -> `internal/store`
- article handlers -> `internal/articles`
- templates/static -> `internal/web`

### Этап D — Production simplification

Сделать Docker Compose основным production путём. TUI оставить как optional helper, а не как главный installer.

