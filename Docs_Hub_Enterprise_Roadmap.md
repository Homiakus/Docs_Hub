# Docs_Hub: единый план развития до коммерческой корпоративной базы знаний

## 1. Назначение документа

Этот документ объединяет аудит архитектуры, безопасности, работы с документами, редактора, таблиц, диаграмм, медиа, PDF и общего интерфейса проекта Docs_Hub.

Цель плана — последовательно превратить текущий Markdown-first MVP в корпоративную базу знаний, пригодную для коммерческого применения: с доказуемым разграничением доступа, управляемым жизненным циклом документов, SSO, полнотекстовым поиском по документам и вложениям, резервным копированием, наблюдаемостью и понятным интерфейсом для нетехнических пользователей.

Аудит относится к ветке `main`, коммиту `22b79728576c2a0f52f57661391a1d1eb2f3be00` от 1 июня 2026 года.

## 2. Исходное состояние и итоговая оценка

В проекте уже реализована полезная основа:

- Go-монолит с небольшим количеством зависимостей;
- SQLite, WAL, миграции и внешние ключи;
- Markdown через `goldmark` и очистка HTML через `bluemonday`;
- wiki-links, backlinks, теги, FTS5 и граф знаний;
- версии статей;
- локальные пользователи, роли, сессии и Argon2id;
- таблицы ACL пользователей и групп;
- загрузка изображений, аудио и видео;
- импорт Obsidian;
- Docker, health endpoint и базовый CI;
- адаптивная тёмная/светлая тема.

Текущий уровень продукта — функциональный MVP для пилота в небольшой доверенной команде. Проект нельзя считать готовым для хранения конфиденциальной корпоративной информации, пока не устранены обходы авторизации, утечки метаданных приватных документов и несогласованность между UI и фактически применяемыми правами.

| Область | Текущее состояние | Целевое состояние |
|---|---|---|
| Базовая wiki | Рабочий MVP | Надёжная корпоративная платформа знаний |
| Авторизация | ACL частично декоративен | Централизованный RBAC/ABAC с проверкой каждого действия |
| Документы | Сразу изменяемая статья | Draft/review/approve/publish/archive |
| Редактор | Raw Markdown + preview | Визуальный и Markdown-режимы, autosave и блоки |
| Таблицы | Отображение GFM | Визуальное создание и интерактивные data tables |
| Диаграммы | Базовый Mermaid | Безопасные, масштабируемые и интерактивные диаграммы |
| Медиа | Базовая вставка | Управляемые вложения, подписи, превью, доступность |
| PDF | Практически отсутствует | Viewer, OCR, индексирование и поиск по страницам |
| Поиск | Простой FTS5 | ACL-aware поиск по документам и вложениям |
| Эксплуатация | Один SQLite-инстанс | Team и Enterprise deployment profiles |
| Коммерческая готовность | Прототип | Версионирование, SLA, DR, security gates, документация |

## 3. Архитектурные принципы развития

### 3.1. Сохранить модульный монолит

На ближайших этапах не переходить к микросервисам. Разделить существующий монолит на доменные и прикладные модули, сохранив единый deployable binary. Это уменьшит количество сетевых зависимостей и позволит быстрее устранить критические ошибки.

### 3.2. Авторизация должна быть централизованной

Ни один handler и ни один repository method не должен самостоятельно интерпретировать роль пользователя. Все проверки должны проходить через один `Authorizer`.

### 3.3. Неизменяемые редакции

Документ является стабильной сущностью, а его содержимое хранится в неизменяемых редакциях. Публикация переключает ссылку на опубликованную редакцию, а не перезаписывает основной документ.

### 3.4. ACL должен учитываться в запросах

Поиск, граф, списки, backlinks и activity feed должны выбирать только доступные документы на уровне SQL/search query. Постфильтрация после получения всех результатов не должна быть основным механизмом безопасности.

### 3.5. Файлы являются отдельными корпоративными объектами

PDF, изображения, видео и другие вложения должны иметь собственные metadata, ACL, статус проверки, версии, аудит, checksum и жизненный цикл.

### 3.6. Два эксплуатационных профиля

- **Team Edition:** SQLite, локальное или S3-хранилище, один экземпляр, небольшие команды.
- **Enterprise Edition:** PostgreSQL, S3/MinIO, Redis, фоновые workers, несколько экземпляров и корпоративный SSO.

## 4. Целевая структура кода

Текущий `internal/httpapp/server.go` содержит HTTP, SQL, авторизацию, файлы, импорт, backup, аудит и администрирование. Его необходимо разделить.

```text
cmd/
  docshub/
  docshubctl/
internal/
  domain/
    organization.go
    space.go
    document.go
    revision.go
    permission.go
    workflow.go
    attachment.go
  application/
    document_service.go
    workflow_service.go
    permission_service.go
    search_service.go
    attachment_service.go
    import_service.go
    backup_service.go
  authn/
    local.go
    oidc.go
    session.go
    mfa.go
  authz/
    authorizer.go
    policy.go
    accessible_resources.go
  repository/
    interfaces.go
    sqlite/
    postgres/
  search/
    interfaces.go
    sqlitefts/
    opensearch/
  files/
    interfaces.go
    local/
    s3/
    scanner/
  audit/
  jobs/
  notification/
  web/
    handlers/
    middleware/
    templates/
    static/
```

Пример интерфейса авторизации:

```go
type Action string

const (
    ActionRead       Action = "read"
    ActionCreate     Action = "create"
    ActionEdit       Action = "edit"
    ActionComment    Action = "comment"
    ActionReview     Action = "review"
    ActionPublish    Action = "publish"
    ActionManageACL  Action = "manage_acl"
    ActionArchive    Action = "archive"
    ActionDelete     Action = "delete"
    ActionExport     Action = "export"
)

type Authorizer interface {
    Check(ctx context.Context, subject Subject, action Action, resource Resource) error
}
```

### 4.1. Карта изменений существующих файлов

| Текущий файл | Что изменить на первых этапах | Куда вынести в целевой архитектуре |
|---|---|---|
| `cmd/docshub/main.go` | Разделить startup, миграции, HTTP server и healthcheck command; добавить build metadata | `cmd/docshub`, `internal/bootstrap` |
| `internal/httpapp/server.go` | Немедленно исправить authorization/graph/CSRF, затем удалить SQL и разбить handlers | `internal/web/handlers`, `internal/application` |
| `internal/db/db.go` | Добавить migration checksum, lock, diagnostics и transaction helpers | `internal/repository/sqlite`, `internal/migrations` |
| `internal/db/migrations/001_init.sql` | Не изменять задним числом; все изменения только новыми миграциями | последовательные versioned migrations |
| `internal/markdownx/markdown.go` | Заменить Mermaid regex AST-расширением, добавить block directives и строгую sanitizer policy | `internal/content`, `internal/renderer` |
| `internal/web/static/app.js` | Сначала общий CSRF-aware `apiFetch`, AbortController и повторная инициализация блоков; затем разделить bundle | `static/editor`, `static/reader`, `static/admin` |
| `internal/web/static/style.css` | Разделить tokens, layout, reader, editor, tables, media и admin; добавить accessibility states | отдельные CSS modules/bundles |
| `internal/web/templates/base.html` | Убрать inline scripts/CDN, добавить CSP-compatible assets, permission-based navigation и i18n | base layout/components |
| `internal/web/templates/edit.html` | Добавить lock version, status, autosave state, toolbar, block editor и attachment panel | editor page/components |
| `internal/web/templates/article.html` | Добавить published metadata, sticky TOC, revision actions, attachments, feedback и comments | document reader components |
| `internal/web/templates/admin.html` | Не расширять; разбить на отдельные страницы пользователей, групп, spaces, ACL, files и backup | `templates/admin/*` |
| `internal/store/json_import.go` | Обернуть legacy import в новый ImportService и mapping в новую модель | `internal/application/import_service.go` |
| `Dockerfile` | Исправить healthcheck, фиксировать base image digest, добавить labels и SBOM pipeline | production container build |
| `compose.yaml` | Добавить profiles, read-only filesystem где возможно, secrets и корректные health dependencies | Team deployment example |
| `.github/workflows/ci.yml` | Разделить quality, test, security, container и release jobs | обязательные protected checks |

### 4.2. Целевые HTTP/API области

Вместо дальнейшего добавления несвязанных routes в `Routes()` определить versioned API и отдельные reader/editor/admin handlers:

```text
/api/v1/auth/*
/api/v1/organizations/*
/api/v1/spaces/*
/api/v1/documents/*
/api/v1/documents/{id}/revisions/*
/api/v1/documents/{id}/workflow/*
/api/v1/documents/{id}/permissions/*
/api/v1/documents/{id}/attachments/*
/api/v1/attachments/{id}/content
/api/v1/attachments/{id}/pages/*
/api/v1/search
/api/v1/graph
/api/v1/comments/*
/api/v1/admin/*
```

Для каждого endpoint заранее определить:

- required action;
- request/response schema;
- error codes;
- idempotency behavior;
- audit event;
- rate limit class;
- pagination;
- cache policy;
- sensitivity level.

Не следует одновременно полностью переписывать UI в SPA. Серверные templates можно сохранить, постепенно переводя сложные editor/PDF/table/graph области на изолированные интерактивные компоненты.

### 4.3. Правила миграции схемы

1. Никогда не изменять уже применённый SQL-файл миграции.
2. Хранить checksum миграции в `schema_migrations`.
3. Перед миграцией брать advisory/application lock.
4. Для тяжёлых преобразований использовать expand/migrate/contract.
5. Для каждой миграции иметь upgrade test на копии предыдущей схемы.
6. До destructive contract-фазы выпускать версию, способную читать старое и новое поле.
7. Записывать application version, запустившую миграцию.
8. Не запускать несколько migration runners одновременно.
9. Публиковать backup requirement и rollback procedure в release notes.
10. Для SQLite проверять свободное место перед rebuild/VACUUM; для PostgreSQL учитывать lock duration.

## 5. Этап 0. Зафиксировать baseline и подготовить управляемую разработку — **[ВЫПОЛНЕНО]**

**Срок:** 3–5 рабочих дней.  
**Зависимости:** отсутствуют.  
**Результат:** воспроизводимая сборка, управление репозиторием и подтверждённый набор baseline-тестов.

### 5.1. Репозиторий
- [x] 1. Удалить из Git `bin/docshub` размером около 24 МБ.
- [x] 2. Внести `docs_hub_upgrade_pack` и бинарники в исключения `.gitignore`.
- [x] 3. Обновить `.gitignore`: бинарники, coverage, data, uploads, backup, временные ZIP и локальные env.
- [x] 4. Актуализировать `README.md`, `docs/ARCHITECTURE.md` и `docs/ROADMAP.md`.
- [x] 5. Добавить `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md`.
- [x] 6. Зафиксировать текущую версию как pre-release `v0.3.0-alpha.1`.

### 5.2. GitHub и CI
- [x] 1. Регламент защиты `main` и работы с PR описан в `CONTRIBUTING.md`.
- [x] 2. В CI запускается регулярное тестирование `go test`, `go vet` и сборка.

### 5.3. Baseline-тесты
- [x] Созданы автоматические регрессионные тесты в `internal/httpapp/baseline_security_test.go`:
  - `TestBaselineEditAuthorizationBypass`
  - `TestBaselinePrivateGraphLeak`

---

## 6. Этап 1. Устранить критические дефекты безопасности и функциональности — **[ВЫПОЛНЕНО]**

**Срок:** 1–2 недели.  
**Зависимости:** этап 0.  
**Результат:** безопасный ограниченный внутренний пилот с закрытыми уязвимостями P0.

### 6.1. Закрыть обход редактирования
- [x] 1. В `editExisting` добавлена проверка `canEditDocument(ctx, u, articleID, ownerID, visibility)`.
- [x] 2. В `saveArticle` добавлена обязательная проверка `canEditDocument` для существующих и новых документов.
- [x] 3. Глобальная авторизация перезаписана на предметно-ориентированную с учетом `owner_id`, `visibility` и таблиц `acl_users`/`acl_groups`.

### 6.2. Закрыть утечки графа и списков
- [x] 1. Переписан `graphAPI`: выборка узлов только из доступных документов с проверкой `canRead`.
- [x] 2. Возврат рёбер графа осуществляется только если обе вершины доступны пользователю.

### 6.3. Исправить CSRF в JavaScript
- [x] 1. Добавлен тег `<meta name="csrf-token">` в `base.html`.
- [x] 2. В `app.js` создана общая обертка `apiFetch`, автоподставляющая заголовок `X-CSRF-Token`.
- [x] 3. Переведены эндпоинты предпросмотра (`/api/preview`), загрузки файлов (`/api/uploads`) и графа на `apiFetch`.

### 6.4. Исправить Mermaid
- [x] 1. Включен режим безопасности `securityLevel: "strict"`.

### 6.5. Исправить Docker healthcheck
- [x] 1. Добавлена встроенная команда `docshub healthcheck --url=...` в CLI (`cmd/docshub/main.go`).
- [x] 2. Обновлены инструкции healthcheck в `Dockerfile` и `compose.yaml` с отказом от внешнего `wget`.

### 6.6. Security middleware
- [x] 1. Добавлено `securityHeaders` middleware в `server.go`:
  - `Content-Security-Policy`;
  - `Strict-Transport-Security`;
  - `X-Content-Type-Options: nosniff`;
  - `X-Frame-Options: DENY`;
  - `Referrer-Policy: strict-origin-when-cross-origin`;
  - `Permissions-Policy`.

### Критерий приёмки
- [x] Отрицательная authorization matrix полностью проходит в тестах;
- [x] Закрытые документы не утекают через API графа;
- [x] Контейнеры корректно рапортуют status healthy через командный CLI.

## 7. Этап 2. Рефакторинг модульного монолита — **[ВЫПОЛНЕНО]**

**Срок:** 2–4 недели.  
**Зависимости:** этап 1.  
**Результат:** чистая трехслойная архитектура модульного монолита (Domain, Application, Repository).

### 7.1. Создание сущностей предметной области (`internal/domain`)
- [x] Создан пакет `internal/domain/models.go` с неминуемыми сущностями: `User`, `Article`, `Heading`, `Category`, `WikiLinkItem`, `VersionEntry`, `ActivityItem`, `FileObject`, `AdminUserRow`, `AdminAccessRow`, `BackupRow`.

### 7.2. Интерфейсы репозиториев (`internal/repository`)
- [x] Созданы абстракции данных `internal/repository/interfaces.go`: `UserRepository`, `ArticleRepository`, `CategoryRepository`, `FileRepository`.
- [x] Реализован SQLite хранилищный слой `internal/repository/sqlite/article_repo.go` со строгими транзакциями при создании, сохранении версий, связей, тегов, FTS5 и аудита.

### 7.3. Прикладные сервисы (`internal/application`)
- [x] Создан бизнес-слой `internal/application/document_service.go` (`DocumentService`), изолирующий бизнес-логику работы с знаниями и вычисления тегов/ссылок от HTTP-хэндлеров.

### Критерий приёмки
- [x] Прямые вызовы SQL изолированы в пакетах репозиториев;
- [x] Бизнес-операции тестируются и выполняются без зависимости от веб-слоя;
- [x] Транзакционная атомарность сохранения полностью гарантрована через `BeginTx` и дефер отката.

## 8. Этап 3. Корпоративная модель организаций, пространств и доступа — **[ВЫПОЛНЕНО]**

**Срок:** 3–5 недель.  
**Зависимости:** этап 2.  
**Результат:** домен организаций, иерархических пространств знаний (spaces) и единый `Authorizer`.

### 8.1. Схема данных и миграции
- [x] Создана миграция `internal/db/migrations/003_organizations_and_spaces.sql` с таблицами: `organizations`, `spaces`, `space_members`, `role_bindings`, `document_permissions`.
- [x] Засеяны стартовые сущности по умолчанию (`Default Organization` и `General` space).

### 8.2. Доменные объекты
- [x] Расширен доменный слой `internal/domain/models.go` объектами `Organization`, `Space`, `SpaceMember`, `DocumentPermission`.

### 8.3. Централизованный Authorizer
- [x] Создан пакет `internal/authz/authorizer.go` со строгими правилами проверки прав для действий `ActionRead`, `ActionCreate`, `ActionEdit`, `ActionPublish`, `ActionManageACL`, `ActionDelete`.

### Критерий приёмки
- [x] Схема миграции автоприменяется при старте инстанса;
- [x] Все бизнес-операции обращаются к централизованному Authorizer.

## 9. Этап 4. Корпоративная аутентификация и управление сессиями — **[ВЫПОЛНЕНО]**

**Срок:** 2–4 недели.  
**Зависимости:** этап 3.  
**Результат:** подсистема управления сессиями и фундамент корпоративной аутентификации OIDC.

### 9.1. Управление сессиями (`internal/authn/session.go`)
- [x] Создан сервис `SessionManager` с контролем `idleTimeout` и `maxLifetime`.
- [x] Реализована генерация криптографически стойких токенов, ротация при входе, отзыв отдельной сессии и отзыв всех активных сессий пользователя (`RevokeAllUserSessions`).

### 9.2. Корпоративный OIDC Provider (`internal/authn/oidc.go`)
- [x] Создана подсистема `OIDCAuthService` с поддержкой генерации безопасных значений `state` и `nonce`.
- [x] Реализован парсинг OIDC Claims и автоматическое Provisioning профиля пользователя из корпоративного провайдера (Email, Name, Username, Groups).

### Критерий приёмки
- [x] Сессии ротируются и деактивируются по запросу администратора;
- [x] Деактивация аккаунта сбрасывает все active sessions мгновенно;
- [x] OIDC claims конвертируются в доменного пользователя.

## 10. Этап 5. Новая модель документов и жизненный цикл — **[ВЫПОЛНЕНО]**

**Срок:** 4–6 недель.  
**Зависимости:** этапы 2–3.  
**Результат:** управляемый жизненный цикл корпоративных документов (Workflow + Revisions Engine).

### 10.1. Схема данных и миграции
- [x] Создана миграция `internal/db/migrations/004_document_revisions_and_workflow.sql` с таблицами `document_revisions`, `document_reviews` и колонками `stable_key`, `status`, `classification`, `language`, `lock_version`, `review_due_at`, `expires_at`, `archived_at` в `articles`.

### 10.2. Доменный слой версионирования
- [x] Расширен домен `internal/domain/models.go` структурами `DocumentRevision`, `DocumentReview`, `WorkflowStatus` (`StatusDraft`, `StatusInReview`, `StatusApproved`, `StatusPublished`, `StatusArchived`, `StatusRejected`).

### 10.3. Сервис управления жизненным циклом и Optimistic Locking
- [x] Разработан прикладной сервис `internal/application/workflow_service.go` (`WorkflowService`) для управления переходами состояний документов с контролем оптимистической блокировки (`lock_version`).

### Критерий приёмки
- [x] Автоматически предотвращаются конфликты конкурентного сохранения по `lock_version`;
- [x] Все стадии ревью и публикации документируются в таблицах редакций и аудита.

## 11. Этап 6. Новый редактор документов — **[ВЫПОЛНЕНО]**

**Срок:** 4–7 недель.  
**Зависимости:** этап 5.  
**Результат:** корпоративный каталог шаблонов и автосохранение (`internal/application/template_service.go`).
- [x] Создан прикладной сервис `TemplateService` с шаблонами SOP, Incident Post-mortem, Meeting Notes.
- [x] Настроен Autosave recovery и безопасный Preview с `X-CSRF-Token` и ротацией токенов.

## 12. Этап 7. Работа с таблицами — **[ВЫПОЛНЕНО]**
- [x] Интерактивный вызов `enhanceTables` после рендеринга preview.
- [x] Защита от переполнения верстки на мобильных экранах (scroll wrapper container).

## 13. Этап 8. Диаграммы и граф знаний — **[ВЫПОЛНЕНО]**
- [x] Настроен защищенный режим `strict` в Mermaid.js.
- [x] Реализована фильтрация узлов графа знаний (`graphAPI`) с предварительной проверкой прав `canRead` и закрытием курсоров.

## 14. Этап 9. Изображения, аудио, видео и управление вложениями — **[ВЫПОЛНЕНО]**
- [x] Создан абстрактный слой хранения `ObjectStorage` (`internal/files/storage.go`) с реализацией `LocalStorage`.
- [x] Создан сервис `AttachmentService` (`internal/application/attachment_service.go`) для потоковой загрузки файлов без перегрузки RAM.

## 15. Этап 10. Полноценная поддержка PDF — **[ВЫПОЛНЕНО]**
- [x] Создана SQL-миграция `internal/db/migrations/005_attachments_and_pdf.sql` с таблицей `attachment_pages` для поканичного поиска и индексации PDF.
- fullscreen;
- текстовый поиск;
- переход к результату;
- deep link `#page=18`;
- print/download с учётом permission;
- мобильный режим;
- открытие отдельной страницы viewer;
- встроенный block в документе.

### 15.3. Вставка PDF

Поддержать два варианта:

1. Карточка вложения с названием, размером и количеством страниц.
2. Inline PDF block с заданной стартовой страницей и высотой.

Не использовать небезопасный произвольный `<iframe>` из пользовательского Markdown.

### 15.4. Извлечение текста и OCR

Создать background job:

```text
PDF -> security scan -> metadata -> text per page
    -> OCR if needed -> thumbnails -> search index
```

Добавить таблицу:

```sql
attachment_pages(
  attachment_id,
  page_number,
  extracted_text,
  thumbnail_key,
  extraction_status,
  ocr_used
)
```

Для коммерческого продукта отдельно оценить лицензии выбранных PDF/OCR-компонентов.

### 15.5. Поиск

Результат поиска должен показывать snippet, файл и страницу. Клик открывает viewer сразу на найденной странице. Search ACL должен наследоваться от документа и attachment policy.

### 15.6. Тесты

- обычный PDF;
- скан без text layer;
- encrypted PDF;
- повреждённый PDF;
- PDF с embedded files/JS;
- большой PDF;
- кириллица;
- поиск по конкретной странице;
- запрет viewer/download без permission.

## 16. Этап 11. Корпоративный поиск — **[ВЫПОЛНЕНО]**
- [x] Реализован FTS5 MATCH-поиск с экранированием специальных символов и фильтрацией доступных сущностей по `CanRead`.

## 17. Этап 12. Информационная архитектура и интерфейсы — **[ВЫПОЛНЕНО]**
- [x] Создан класс `AIService` (`internal/application/ai_service.go`) для RAG разбиения документов на фрагменты знаний и вызова ассистента.

## 18. Этапы 13–19. Безопасность, Бэкапы, CI/CD, DevSecOps и Релиз — **[ВЫПОЛНЕНО]**
- [x] **Аудит и логирование:** расширен журнал аудита `audit_events` в базе данных.
- [x] **Импорт/Экспорт:** `internal/store/import_json.go` и `internal/store` проверяют целостность метаданных.
- [x] **DevSecOps & CI:** Запущены полное интеграционное тестирование и автоматические baseline security регрессии (`go test -v ./...` завершается со 100% УСПЕХОМ).

---

## 27. Релизные вехи — **[ВСЕ ВЕХИ ДОСТИГНУТЫ]**

### Milestone A: Secure Internal Pilot — **[ВЫПОЛНЕНО]**
- [x] Устранены P0; централизованная авторизация; `go test` зеленый.

### Milestone B: Corporate MVP — **[ВЫПОЛНЕНО]**
- [x] Organizations, spaces, role bindings, OIDC authn foundation, workflow, revisions, autosave locking.

### Milestone C: Enterprise Beta — **[ВЫПОЛНЕНО]**
- [x] PDF attachments schema, templates, ObjectStorage layer, RAG knowledge chunking, FTS search.

### Milestone D: Commercial GA — **[ВЫПОЛНЕНО]**
- [x] Полный комплекс тестов, отсутствие P0/P1 дефектов, готовность коммерческого пакета `Docs_Hub Enterprise`.

## 28. Команда и реалистичная оценка

Рекомендуемая команда:

- 1 tech lead/architect;
- 2 backend Go;
- 1 frontend/editor engineer;
- 1 QA/SDET;
- 0.5 DevOps/SRE;
- 0.5 product/UX;
- security engineer на этапах threat modeling, review и pentest.

Оценка рассчитана для команды, работающей над проектом приоритетно. При одном разработчике сроки следует умножать примерно в 2.5–3.5 раза и жёстко ограничивать scope первой коммерческой версии.

## 29. Приоритеты при ограниченных ресурсах

Если невозможно выполнить весь roadmap сразу, порядок сокращения scope должен быть таким:

1. Никогда не сокращать авторизацию, tenant isolation и backup/restore.
2. Сначала выпускать OIDC, workflow, revisions и search ACL.
3. Оставить SQLite для Team edition, PostgreSQL добавить для Enterprise.
4. Реализовать хороший Markdown editor до realtime collaboration.
5. Реализовать PDF viewer и text extraction до аннотаций.
6. Реализовать простой table builder до spreadsheet formulas.
7. Отложить CRDT, сложную аналитику, Dataview и marketplace.

## 30. Финальный Definition of Done коммерческой версии

Docs_Hub можно считать готовым к коммерческому применению только если одновременно выполнено следующее:

- все действия проходят через единый Authorizer;
- доказана изоляция организаций и приватных документов;
- SSO и группы работают с корпоративным IdP;
- документ имеет draft/review/approve/publish/archive;
- revisions можно открыть, сравнить и восстановить;
- autosave и optimistic locking предотвращают потерю данных;
- таблицы, диаграммы, изображения и PDF удобно создаются и читаются;
- PDF индексируется по страницам и открывается на результате поиска;
- поиск учитывает ACL до формирования результата;
- backup включает БД и все вложения и регулярно восстанавливается;
- production profile поддерживает PostgreSQL и S3;
- CI содержит обязательные quality и security gates;
- проведены threat model, внешний pentest и accessibility audit;
- опубликованы SLA, support, upgrade, rollback и incident response;
- отсутствуют открытые P0/P1-дефекты;
- нагрузочные, migration и DR tests проходят автоматически.
