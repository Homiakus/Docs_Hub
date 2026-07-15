# Архитектура Docs_Hub

## Целевое визионирование

Docs_Hub эволюционирует из минимального Markdown-first MVP в модульный монолит бизнес-класса. Архитектура разработана с целью обеспечения максимальной скорости и безопасности, исключая преждевременную сложность микросервисов.

## Принципы архитектуры

1. **Модульный монолит**: Единый исполняемый файл с четким разграничением доменных границ и прикладных сервисов.
2. **Централизованный Authorizer**: Ни один handler или SQL-запрос не принимает решение об авторизации самостоятельно; все действия проверяются единой службой контроля доступа.
3. **Два профиля развертывания**:
   - **Team Edition**: SQLite (WAL mode), локальное хранилище вложений, 1 инстанс.
   - **Enterprise Edition**: PostgreSQL, S3/MinIO, Redis для распределенных сессий/очередей, фоновые workers.

## Целевая структура пакетов (`internal/`)

```text
cmd/
  docshub/               # Точка входа веб-сервера и CLI
  docshubctl/            # CLI для бэкапа, гидратации и административных задач
internal/
  domain/                # Чистые сущности: Document, Revision, Organization, Space, Permission
  application/           # Прикладные сервисы: DocumentService, PermissionService, SearchService
  authn/                 # Аутентификация: Local, OIDC, Sessions
  authz/                 # Централизованный Authorizer и RBAC/ABAC политики
  repository/            # Интерфейсы репозиториев и реализации (SQLite/PostgreSQL)
  search/                # Абстракция поиска (SQLite FTS5 / OpenSearch)
  files/                 # Хранилище объектов (Local / S3)
  audit/                 # Журналирование событий безопасности
  web/                   # HTTP хэндлеры, middleware, шаблоны и статика
```

## Хранение данных

Основные концептуальные таблицы:

- `organizations`, `spaces`, `space_members`
- `users`, `groups`, `group_members`, `role_bindings`
- `documents`, `document_revisions`, `document_permissions`, `document_reviews`
- `tags`, `document_tags`, `links`
- `files`, `document_files`, `attachment_pages`
- `sessions`, `audit_events`, `document_fts`
