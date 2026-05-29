# Глубокий аудит Docs Hub

## Краткий диагноз

Docs Hub уже является рабочим MVP: Markdown-статьи, wiki-links, tags, backlinks, ACL, группы, вложения, backup и деплой. Но текущая архитектура делает слишком много вручную: JSON как база, ручной Markdown-render/sanitize, большой `main.go`, самописный installer/deploy TUI и ZIP-backup всего мира.

Это было оправдано для быстрого старта. Для корпоративной wiki и Obsidian-like продукта нужно перейти от “один файл всё делает” к небольшой модульной архитектуре.

## Где изобретён велосипед

### 1. JSON как база данных

Текущий `storage.json` — удобный MVP, но не production storage для wiki.

Проблемы:

- весь файл переписывается при изменении;
- сложно делать версии статей;
- нет индексов;
- поиск проходит по памяти;
- миграции становятся ручными;
- backup/restore привязан к остановке сервиса;
- concurrency и блокировки будут усложняться.

Рекомендация: сначала SQLite, потом PostgreSQL при необходимости.

### 2. Самописный Markdown renderer/sanitizer

Markdown и HTML sanitizer опасно писать вручную. Особенно если появятся таблицы, HTML, Mermaid, math, embeds, callouts и плагины.

Рекомендация:

- `goldmark` для Markdown;
- `bluemonday` для HTML sanitize;
- wiki-links/tags/headings извлекать отдельно;
- links/tags хранить в БД.

### 3. HTML через строки в Go

Если UI генерируется через `strings.Builder` и ручные `WriteString`, то проект быстро становится трудно поддерживать.

Рекомендация:

- краткосрочно: `html/template` + компоненты;
- лучше: `templ + htmx`;
- full SPA только если Canvas/Graph станут центральными функциями.

### 4. Deploy TUI

TUI installer — красивый, но это частичное дублирование Ansible/Helm/Docker Compose/systemd документации.

Рекомендация:

- официальный production path: Docker Compose + env + reverse proxy;
- systemd unit оставить как пример;
- TUI оставить как optional helper.

### 5. ZIP backup всего приложения

Для JSON-storage это понятно. Но после SQLite/PostgreSQL лучше использовать DB-native backup и manifest для файлов.

## Где дублирование

### 1. Дублирование ответственности в `main.go`

`main.go` фактически содержит:

- HTTP routes;
- auth;
- sessions;
- CSRF;
- users/groups;
- ACL;
- Markdown;
- attachments;
- backup;
- audit;
- UI rendering;
- migrations.

Рекомендация: разнести по `internal/*` без изменения внешнего поведения.

### 2. Дублирование metadata в Article

Поля `Tags`, `AllowedUserIDs`, `AllowedGroupIDs`, `Versions` внутри Article удобны для JSON, но плохо масштабируются.

Рекомендация:

- `article_tags`;
- `article_acl_users`;
- `article_acl_groups`;
- `article_versions`;
- `article_links`.

### 3. Дублирование файлов

Если вложения хранятся по article-id, один и тот же файл может быть загружен много раз.

Рекомендация:

- таблица `files` с `sha256`;
- таблица `article_files` для связей;
- garbage collector orphan-файлов.

## Что можно сделать проще

1. Не делать сразу PostgreSQL. SQLite first.
2. Не делать сразу OpenSearch. SQLite FTS5 first.
3. Не делать сразу real-time collaborative editor. Сначала optimistic locking + versions.
4. Не делать сразу plugin marketplace. Сначала стабильный parser/extractor API.
5. Не копировать Obsidian полностью. Делать Obsidian-compatible corporate wiki.

## Минимальный план версии 1.5

1. Вынести Markdown в `internal/markdown`.
2. Добавить SQLite schema и мигратор JSON -> SQLite.
3. Разрезать `main.go` на пакеты.
4. Добавить table-based versions.
5. Добавить file dedup через `sha256`.
6. Сделать Docker Compose основным production path.

## План версии 2.0

1. OIDC/SSO.
2. Graph по таблице `article_links`.
3. Properties/frontmatter.
4. Views/queries как лёгкий аналог Obsidian Dataview.
5. Review workflow: draft -> review -> published.
6. Import/export Obsidian vault.
7. Comments and mentions.
8. Public/private publish mode.

