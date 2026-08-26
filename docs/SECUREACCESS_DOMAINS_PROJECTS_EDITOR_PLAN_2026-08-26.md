# Docs_Hub — SecureAccess, Domains, Projects & Editor Master Plan

**Дата:** 2026-08-26  
**Docs_Hub baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`  
**SecureAcces baseline:** `main@827abb1add11a9fcbd0a9944e65efbd20c675739`  
**Статус:** master implementation plan  
**Связанный документ:** [Pareto Implementation Plan](PARETO_IMPLEMENTATION_PLAN_2026-08-26.md)

---

## 1. Цель

Перестроить Docs_Hub вокруг простой для пользователя и строгой для системы модели:

```text
Organization
└── Domain                    ← тематическая область + основная граница доступа
    ├── Project               ← рабочий контекст внутри домена
    │   ├── Document
    │   ├── Document
    │   └── Attachments
    └── Project
        └── Documents
```

При этом:

1. **Docs_Hub не реализует собственную авторизацию.** Аутентификация, сессии, постоянные memberships, effective permissions, revoke и security audit принадлежат `github.com/Homiakus/SecureAcces`.
2. **Domain — первая пользовательская единица навигации и доступа.** Примеры: Engineering, Regulatory, Clinical, Quality, Operations, Product.
3. **Project — рабочая единица внутри Domain.** Пользователь по умолчанию получает доступ к проектам через домен; отдельные проекты можно сделать ограниченными.
4. **Документ принадлежит ровно одному Project.** Тематика, перекрёстные связи и дополнительные классификации выражаются tags/wiki-links, а не несколькими владельцами доступа.
5. **Markdown остаётся каноническим форматом хранения**, но редактор становится визуально удобным: live preview, slash commands, быстрые ссылки, drag-and-drop, автосохранение, история и change-review flow.
6. **Права применяются до SQL ranking/LIMIT/aggregation.** Search, graph, backlinks, activity, files и dashboard никогда не получают запрещённые объекты для последующей фильтрации.
7. **Сложность скрыта от обычного пользователя.** Пользователь видит 5–6 понятных ролей, домены и проекты; битовые permissions, security workspace IDs и policy inheritance остаются внутри системы.

---

# 2. Что берём из лучших современных продуктов

План не копирует один продукт целиком. Он сочетает сильные стороны нескольких моделей.

## 2.1. Notion — Teamspaces и progressive disclosure

Берём:

- отдельный «дом» для каждой команды/темы;
- понятные режимы доступа наподобие open / closed / private;
- чистую writing surface, где второстепенные controls уходят из поля зрения;
- slash-команды, keyboard shortcuts, контекстное форматирование;
- быстрые mentions/links и навигацию через command palette.

Не берём:

- бесконечную вложенность страниц как основную модель доступа;
- смешивание структуры информации и случайной page-sharing логики без чёткой доменной границы.

Reference:
- https://www.notion.com/help/intro-to-teamspaces
- https://www.notion.com/help/keyboard-shortcuts
- https://www.notion.com/help/guides/using-slash-commands

## 2.2. GitBook — Collections/Spaces, inheritance и change requests

Берём:

- cascading permission model: родительский уровень задаёт default, дочерний обычно наследует;
- отдельный content workspace/project;
- role presets `read/comment/edit/review/admin` вместо ручной настройки десятков флагов;
- change-request workflow для безопасного редактирования опубликованной документации;
- Changes / Preview / Review как разные представления одного изменения;
- централизованный список изменений и ревью.

Reference:
- https://gitbook.com/docs/account-management/member-management/permissions-and-inheritance
- https://gitbook.com/docs/creating-content/content-structure/collection
- https://gitbook.com/docs/creating-content/content-structure/space
- https://gitbook.com/docs/collaboration/change-requests

## 2.3. Confluence — predictable hierarchy of permissions

Берём:

- крупная граница доступа выше документа;
- документ не должен внезапно расширять доступ за пределы родительской области;
- отдельное ограничение контента используется как исключение, а не как default;
- администратор должен видеть, откуда именно пришёл effective access.

Reference:
- https://support.atlassian.com/confluence-cloud/docs/what-are-space-permissions/
- https://support.atlassian.com/confluence-cloud/docs/manage-permissions-on-the-page-level/

## 2.4. Diátaxis — структура типов документации

Берём как optional template taxonomy:

- Tutorial;
- How-to;
- Reference;
- Explanation.

Это не заменяет Domain/Project, а помогает выбирать структуру нового документа.

Reference: https://diataxis.fr/

---

# 3. Главные продуктовые решения

## 3.1. Domain — и тема, и security boundary

Domain не является обычным tag.

Domain задаёт:

- тему/направление;
- набор участников;
- default role;
- владельцев/менеджеров;
- проекты;
- default content templates;
- lifecycle/archive state;
- настройки публикации и экспорта;
- security workspace в SecureAccess.

Пользователь не должен выбирать «категорию», «space», «ACL» и «visibility» отдельно при каждом создании документа. Если он находится в `Engineering → HP4`, новый документ автоматически принадлежит этому проекту и получает его effective access.

## 3.2. Project — рабочая область внутри Domain

Project содержит:

- документы;
- review queue;
- activity;
- attachments;
- участников, только если доступ отличается от Domain;
- project templates;
- project status: active / paused / archived.

По умолчанию Project **наследует Domain access**.

Restricted Project — явное исключение, которое разрывает наследование и требует прямого membership.

## 3.3. Document-level ACL не является основным UX

Default:

```text
Domain access → Project access → Document access
```

Документ наследует Project.

Для специальных случаев допускается `Restricted document`, но это advanced action. В security model он получает отдельный дочерний SecureAccess Workspace. Таким образом Docs_Hub не придумывает собственный resource ACL.

Это позволяет мигрировать существующие `private` документы без потери семантики, но не заставляет пользователей настраивать ACL для каждого документа.

---

# 4. Целевая модель SecureAccess

Текущий SecureAccess уже предоставляет:

- global Account;
- tenant-local User;
- Workspace с `ParentID`;
- Membership;
- PermissionBits;
- session authentication;
- request-scoped Principal;
- `Service.Authorize`;
- `httpauth.Middleware.Protect`;
- server-side ResourceResolver;
- durable Axiom/Pebble adapter.

Однако текущий `DefaultAuthorizer` проверяет точное совпадение `WorkspaceID`; сам `ParentID` пока не даёт permission inheritance. Для Domain → Project это нужно реализовать **в SecureAccess**, а не в Docs_Hub.

## 4.1. Mapping

| Docs_Hub | SecureAccess |
|---|---|
| Organization | Tenant |
| Organization security root | root Workspace |
| Domain | child Workspace of root |
| Project | child Workspace of Domain |
| Normal Document | Resource in Project Workspace |
| Restricted Document | child Workspace of Project + Resource |
| Attachment | Resource в том же effective Workspace, что документ |
| Authenticated user | Account + tenant User |
| Domain/Project membership | Membership |
| Role preset | Role string + PermissionBits |

## 4.2. Запрещённая архитектура

Не делать:

```text
Docs_Hub DB ACL
       +
SecureAccess ACL
       +
handler role checks
```

Должно быть:

```text
Identity / Session / Membership / Permission / Revocation
                         │
                         ▼
                    SecureAccess
                         │
             Principal + AccessScope
                         │
                         ▼
Docs metadata → ResourceResolver → Authorize → application use-case
                         │
                         ▼
               ACL-scoped repository query
```

---

# 5. Изменения, необходимые в SecureAcces

Эти изменения выполняются в `Homiakus/SecureAcces` отдельными PR и выпускаются как новая совместимая версия до удаления legacy auth из Docs_Hub.

## SA-01 — Hierarchical Workspace authorization

### Добавить

```go
type WorkspaceAccessMode string

const (
    WorkspaceInherit  WorkspaceAccessMode = "INHERIT"
    WorkspaceIsolated WorkspaceAccessMode = "ISOLATED"
)
```

В `Workspace`:

```go
AccessMode WorkspaceAccessMode
```

### Семантика

- `INHERIT`: permissions прямых memberships + permissions разрешённых ancestors объединяются.
- `ISOLATED`: ancestor chain обрывается на этом workspace.
- прямой membership на child работает независимо от parent membership;
- permission resolution остаётся fail-closed;
- revoked/suspended/expired membership никогда не наследуется;
- tenant boundary никогда не пересекается.

### Тесты

- root → domain → project inheritance;
- isolated domain;
- isolated project;
- direct membership после isolation;
- revoked ancestor membership;
- expired ancestor membership;
- cross-tenant parent injection;
- cycle/corrupted hierarchy protection;
- race tests при grant/revoke во время Authorize.

## SA-02 — Query-safe `AccessScope`

Docs_Hub не должен получать тысячу документов и фильтровать их после `LIMIT`.

Добавить API примерно такого уровня:

```go
type AccessScope struct {
    TenantID ID
    Workspaces []ID
}

func (s *Service) Scope(
    ctx context.Context,
    p Principal,
    tenantID ID,
    permission PermissionBits,
) (AccessScope, error)
```

Требования:

- fresh session validation так же, как в `Authorize`;
- учитывается hierarchy/isolation;
- возвращаются только effective workspace IDs;
- deterministic sorted output;
- никакой выдачи memberships/secrets;
- benchmark 10 / 100 / 1k / 10k workspaces;
- fuzz/corruption tests;
- revoke должен отражаться на следующем request scope.

## SA-03 — Content workflow permission bits

Текущих `View/Download/Upload/Edit/Delete/ManageMembers/ManageWorkspace` недостаточно, если вся авторизация действительно принадлежит SecureAccess.

Добавить generic bits:

```text
PermComment
PermReview
PermPublish
PermArchive
PermManageAccess
```

Если `PermManageAccess` семантически дублирует `PermManageMembers`, оставить один канонический bit и зафиксировать это ADR.

## SA-04 — Standard role presets

Role string остаётся display/audit metadata, но SecureAccess должен иметь helper presets, чтобы разные хосты не создавали несовместимые битовые маски.

Предлагаемые presets:

| Role | Permissions |
|---|---|
| Reader | View, Download |
| Commenter | Reader + Comment |
| Editor | Commenter + Upload, Edit |
| Reviewer | Editor + Review |
| Publisher | Reviewer + Publish, Archive |
| Manager | Publisher + Delete, ManageMembers, ManageWorkspace |

Docs_Hub показывает пользователю эти роли понятными названиями.

## SA-05 — Human management authorization

Сейчас default `SystemOnlyManagementAuthorizer` означает, что обычный domain manager не может безопасно выполнять `GrantPersistent` без host-side policy.

Нужно добавить в SecureAccess production implementation управления workspace memberships:

- `PermManageMembers` разрешает membership management в effective workspace;
- `PermManageWorkspace` разрешает настройки scope;
- inherited manager permission работает до isolation boundary;
- manager не может выдать permission выше собственного effective ceiling;
- manager не может переместить workspace в tenant/domain, которым не управляет;
- нельзя удалить/отозвать последнего manager без explicit recovery flow.

Docs_Hub после этого **не проверяет роль manager самостоятельно**.

## SA-06 — Workspace/member management API для UI

Добавить service methods:

```text
ListTenantUsers
ListWorkspaceMembers
ListWorkspaceChildren
UpdateWorkspaceAccessMode
MoveWorkspace
UpdateWorkspaceMetadata/Name (если имя хранится в security audit)
BulkGrantMemberships
BulkRevokeMemberships
```

Bulk операции должны иметь чёткую atomicity/idempotency semantics.

## SA-07 — Effective access explanation

Для хорошего UX нужен ответ на вопрос: «Почему у пользователя есть доступ?»

Добавить manager-only API:

```text
ExplainAccess(user, workspace, permission)
→ direct membership / inherited from Domain / inherited from Organization / isolated / denied
```

UI не должен вычислять это самостоятельно.

## SA-08 — Stable external binding / idempotent provisioning

Docs и security storage могут коммититься независимо. Нужен безопасный retry.

Добавить stable host binding:

```text
Tenant + ExternalKey → Workspace
```

или idempotent `EnsureWorkspace`.

ExternalKey генерируется Docs_Hub как immutable stable key, а не из изменяемого slug.

Acceptance:

- повтор одного create request не создаёт второй security workspace;
- retry после process crash безопасен;
- rename Domain/Project не меняет security identity.

## SA-09 — Optional Teams/Groups

Не блокирует первый релиз Domain/Project, но является следующим высоким UX-выигрышем для организаций >20–30 пользователей.

Добавить:

```text
Team
TeamMember
WorkspaceTeamMembership
```

Тогда администратор выдаёт доступ `Engineering Team`, а не 18 людям по одному.

До реализации Teams UI должен поддерживать быстрый multi-select и bulk role assignment.

---

# 6. Persistence strategy SecureAccess

## Рекомендованный первый production profile

Docs_Hub сохраняет:

```text
SQLite      → content metadata, documents, revisions, search index
Axiom/Pebble → SecureAccess state
```

Причины:

- уже существующий и проверенный adapter SecureAccess;
- security domain остаётся физически и логически отделён;
- Docs_Hub не получает direct persistence access к auth state;
- runtime всё равно остаётся single-process / single deployable binary.

## 6.1. Cross-store consistency

Не пытаться изображать распределённую ACID-транзакцию.

Использовать fail-closed provisioning saga.

### Создание Domain

```text
1. Docs generates immutable domain stable_key.
2. SecureAccess EnsureWorkspace(external_key=domain stable_key).
3. Получаем security_workspace_id.
4. В одной SQLite transaction создаём Domain + binding.
5. Domain становится ACTIVE.
```

Если шаг 4 падает, остаётся security workspace без content — это безопасный orphan. Reconciler удаляет/переиспользует его позже.

### Создание Project

Аналогично, parent = Domain security workspace.

### Если SecureAccess недоступен

- новый Domain/Project не создаётся;
- access-changing mutation не коммитится;
- чтение защищённого content fail-closed;
- UI показывает понятный `Security service temporarily unavailable`, а не silently allows.

## 6.2. Backup

Production backup manifest должен содержать согласованный набор:

```text
manifest.json
content/docshub.db
security/secureaccess-pebble/
uploads/
checksums.sha256
version.json
```

Restore drill обязан проверять:

- документы;
- domain/project mappings;
- memberships;
- revocation;
- restricted project/doc access;
- FTS rebuild.

---

# 7. Новая Docs_Hub domain model

## 7.1. Domain

```go
type Domain struct {
    ID                  int64
    StableKey           string
    OrganizationID      int64
    SecurityWorkspaceID string
    Slug                string
    Name                string
    Description         string
    Icon                string
    Status              DomainStatus
    DefaultProjectID    *int64
    SortOrder           int
    CreatedBy           string
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

`StableKey` никогда не меняется. `Slug` можно менять с redirects.

## 7.2. Project

```go
type Project struct {
    ID                  int64
    StableKey           string
    DomainID            int64
    SecurityWorkspaceID string
    Slug                string
    Name                string
    Description         string
    Status              ProjectStatus
    AccessMode          AccessMode // inherit | restricted
    SortOrder           int
    CreatedAt           time.Time
    UpdatedAt           time.Time
}
```

## 7.3. Document

Документ хранит `ProjectID`.

Для нормального документа:

```text
SecurityWorkspaceID = Project.SecurityWorkspaceID
```

Для restricted document:

```text
Document.SecurityWorkspaceID = dedicated child workspace
```

Не хранить два независимых ACL-механизма.

---

# 8. Миграция существующих `spaces`

Текущий `spaces` уже широко используется. Резкий rename таблицы создаст много риска без пользовательской пользы.

## Этап M1 — compatibility

- добавить `domains`;
- создать default Domain `General`;
- добавить `domain_id` в `spaces`;
- все существующие spaces привязать к General;
- domain model в Go называть `Project`, но repository пока может читать physical table `spaces`;
- UI полностью переименовать Space → Project.

## Этап M2 — code convergence

- `SpaceRepository` → `ProjectRepository`;
- `Space` view models → `Project`;
- routes `/spaces/*` получают redirects на `/domains/{domain}/projects/*`;
- search filters переходят на Domain + Project.

## Этап M3 — physical schema cleanup

Только когда `grep`/architecture test подтверждает отсутствие production dependencies на старое имя:

- `spaces` → `projects` migration;
- `articles.space_id` → `project_id`;
- compatibility views на один release при необходимости;
- удалить views в следующем major/minor migration window.

---

# 9. Миграция legacy ACL/auth Docs_Hub

Удаление старой auth системы происходит последним, не первым.

## 9.1. Inventory

Зафиксировать:

- users;
- sessions;
- `acl_users`;
- `space_members`;
- `role_bindings`;
- `document_permissions`;
- article visibility;
- owner IDs;
- workflow permissions.

## 9.2. Mapping пользователей

Создать таблицу только для migration/binding:

```text
legacy_user_id → SecureAccess tenant_user_id → account_id
```

После успешной миграции user profile Docs_Hub хранит только product metadata; authentication identity принадлежит SecureAccess.

## 9.3. Legacy private documents

Если старый документ имеет уникальный ACL, который нельзя выразить Project membership:

1. создать dedicated restricted child workspace;
2. перенести разрешённых пользователей туда;
3. связать document с этим workspace;
4. проверить matrix tests;
5. только затем удалить старый ACL.

## 9.4. Dual verification period

В тестовом режиме временно вычислять:

```text
legacy_decision
secureaccess_decision
```

Если решения отличаются — тест/diagnostic event падает.

В production не делать `allow if either allows`; SecureAccess становится единственным enforcement path только после migration gate.

---

# 10. Целевая информационная архитектура UI

## 10.1. Sidebar

```text
Search
Home
My work

DOMAINS
▾ Engineering
   • Overview
   ▾ HP4
      Docs
   ▾ CAD Automation
      Docs
▸ Regulatory
▸ Clinical

Pinned
Recent
```

Правила:

- показываются только доступные Domain/Project;
- domain/project collapse state сохраняется локально;
- максимум 1–2 уровня постоянной навигации;
- document tree показывается только внутри текущего Project, а не глобально;
- favorites/recent не меняют security scope;
- mobile sidebar превращается в drawer.

## 10.2. Home

Не «все статьи подряд».

Главные блоки:

1. Continue editing.
2. Reviews for me.
3. Recent documents.
4. My Domains.
5. Pinned Projects.
6. Stale/needs attention — только для owners/managers.

## 10.3. Domain home

Показывает:

- название/описание/owners;
- Projects;
- недавно обновлённые документы;
- review activity;
- Domain members для managers;
- New Project / New Document.

## 10.4. Project home

Показывает:

- concise project overview;
- document tree/list;
- templates;
- open changes/reviews;
- recent activity;
- files;
- access indicator: `Inherited from Engineering` или `Restricted`.

---

# 11. UX управления доступом

## 11.1. Domain access dialog

Открывается из `Share / Access`.

Не показывать raw permission bits.

```text
Engineering access

Access mode: Members only

[ Add people ]

Alice   Publisher    Direct
Bob     Editor       Direct
Carol   Reader       Direct

Projects inherit this access by default.
```

## 11.2. Add people

Один flow:

1. search/multi-select users;
2. choose role preset;
3. optional expiration;
4. summary;
5. Apply.

Без отдельного form submit на каждого пользователя.

## 11.3. Project access

По умолчанию:

```text
Access: Inherited from Engineering
```

Кнопка:

```text
Make restricted
```

Перед изменением показывается preview:

```text
24 users currently inherit access.
After restriction only 4 explicitly selected users will retain access.
```

Это предотвращает случайное закрытие проекта.

## 11.4. Effective access explanation

В user/member row:

```text
Editor · inherited from Engineering
```

или:

```text
Publisher · direct project access
```

или:

```text
No access · project is restricted
```

Источник решения приходит от SecureAccess `ExplainAccess`, а не вычисляется frontend.

## 11.5. User-centric access page

У администратора/manager должен быть обратный view:

```text
User: Alice
Domains
  Engineering   Publisher
  Regulatory    Reader
Projects
  Clinical / LBC   Editor (direct)
Sessions
  Android Chrome
  Desktop Firefox
```

Это значительно удобнее расследования «почему человек видит/не видит документ».

---

# 12. Route model

Canonical routes:

```text
/domains
/domains/{domain}
/domains/{domain}/projects/{project}
/domains/{domain}/projects/{project}/docs/{slug}
/domains/{domain}/projects/{project}/new
/domains/{domain}/access
/domains/{domain}/projects/{project}/access
/me/work
/reviews
/search
```

Существующие `/a/{slug}` можно оставить как stable resolver route, который делает redirect на canonical path после ACL-aware lookup.

Это сохраняет wiki-links и старые URLs.

---

# 13. Security integration внутри Docs_Hub

## 13.1. Composition root

Новый `internal/bootstrap` собирает:

```text
SecureAccess Store
SecureAccess Service
SecureAccess httpauth middleware
Docs repositories
Docs application services
Resource resolvers
HTTP handlers
```

`internal/httpapp` больше не создаёт session/ACL policy.

## 13.2. Новый пакет bridge

```text
internal/security/
  service.go
  resource_resolver.go
  scope_provider.go
  bindings.go
  errors.go
```

Он **не содержит policy**.

Он только:

- переводит Docs resource → SecureAccess Resource;
- запрашивает Scope;
- переводит security workspace IDs → project IDs;
- нормализует SecureAccess errors в HTTP/application errors.

## 13.3. ResourceResolver

Нельзя доверять `domain_id`, `project_id` или workspace ID от клиента.

Для `/docs/{slug}`:

```text
slug
 ↓
Docs repository lookup
 ↓
document.project_id
 ↓
project.security_workspace_id
 ↓
SecureAccess Resource
```

Для restricted document используется его dedicated security workspace.

## 13.4. Permission mapping

| Action | SecureAccess permission |
|---|---|
| Open document | View |
| Search result visibility | View |
| Download attachment | Download |
| Upload attachment | Upload |
| Edit draft/change | Edit |
| Comment | Comment |
| Request/perform review | Review |
| Publish | Publish |
| Archive | Archive |
| Delete | Delete |
| Manage access | ManageMembers |
| Change Domain/Project security settings | ManageWorkspace |

---

# 14. Query-level access scope

После authentication handler получает Principal.

Для list/search:

```text
SecureAccess.Scope(principal, PermView)
              ↓
allowed security workspace IDs
              ↓
map to allowed Project IDs
              ↓
SQL JOIN/CTE before WHERE ranking/LIMIT
```

Пример концептуально:

```sql
WITH allowed_projects(project_id) AS (...)
SELECT ...
FROM article_fts f
JOIN articles a ON a.id = f.rowid
JOIN allowed_projects ap ON ap.project_id = a.project_id
WHERE article_fts MATCH ?
ORDER BY bm25(article_fts)
LIMIT ?;
```

Тот же принцип применяется к:

- suggestions;
- recent activity;
- backlinks;
- graph nodes/edges;
- dashboard counters;
- document mentions;
- attachment search;
- AI/RAG retrieval.

Post-filter после LIMIT запрещён architecture test.

---

# 15. Редактор: продуктовая цель

Текущий production editor остаётся `<textarea> + preview`, несмотря на более сильную master-spec. Следующий этап должен реально довести редактор до уровня Obsidian/Notion/GitBook, сохранив Markdown-first архитектуру.

Главный принцип:

> Пользователь должен думать о содержании, а не о Markdown-синтаксисе, metadata и ACL.

---

# 16. Выбор editor engine

## Решение: CodeMirror 6 + Markdown AST + server canonical renderer

Почему:

- Markdown остаётся source of truth;
- минимальный риск round-trip corruption;
- прекрасная keyboard ergonomics;
- incremental parsing;
- extensions/decorations;
- можно построить Obsidian-style live preview;
- легко реализовать `[[`, `/`, autocomplete, lint diagnostics;
- не требует превращать storage в proprietary block JSON.

Не переходить сейчас на полностью block-JSON editor.

## Build model

Допускается Node только как **build-time dependency** UI bundle.

Runtime остаётся:

```text
one Go binary + data
```

Assets после сборки embed в Go binary.

Frontend build должен быть воспроизводимым и pin-нутым lockfile.

---

# 17. Editor modes

## Default — Live Preview

Синтаксис скрывается/смягчается вне активного блока.

Пользователь видит:

- headings как headings;
- links как links;
- images inline;
- callouts visually;
- task lists interactive;
- Mermaid preview;
- tables readable;
- code blocks with language label.

## Source mode

Для power users — чистый Markdown.

## Split mode

Markdown + canonical server preview.

Mode сохраняется per user/device.

---

# 18. Editor interaction model

## 18.1. Slash command palette

`/` открывает:

```text
Text
Heading 2
Heading 3
Bulleted list
Numbered list
Checklist
Callout
Code block
Table
Image
File
PDF
Mermaid
Wiki link
Template section
Divider
```

Результаты сортируются:

1. exact query;
2. recent commands;
3. common commands;
4. остальное.

Полностью keyboard-operable.

## 18.2. Selection bubble toolbar

При выделении текста:

```text
B  I  Code  Link  Comment
```

Не держать огромный toolbar постоянно.

## 18.3. Wiki link autocomplete

`[[` вызывает ACL-aware suggest API.

Показывать:

```text
Title
Domain / Project
short context
```

Никогда не показывать недоступный документ даже как suggestion title.

## 18.4. Command palette

`Ctrl/Cmd+K`:

- open document;
- switch Domain;
- switch Project;
- create document;
- insert block;
- publish/request review;
- view changes;
- attach file.

Контекстные commands сверху.

## 18.5. Drag/drop/paste

- paste image из clipboard → upload → insertion;
- drag PDF/image/file → upload → insertion;
- progress inline;
- retry;
- failed upload не ломает document state;
- alt text prompt для images;
- filename sanitization server-side.

---

# 19. Снижение визуальной нагрузки редактора

Текущий editor всегда показывает отдельный блок Templates, toolbar, split preview и раскрытые Properties. Это полезно как функциональный baseline, но перегружает регулярное редактирование.

Новая модель:

## При создании

- templates показываются в compact start chooser или command palette;
- после выбора исчезают;
- Domain/Project уже известны из контекста;
- title + canvas получают почти весь экран.

## При редактировании

Header:

```text
Engineering / HP4 / Pump Controller       Saved
                                     [Review] [•••]
```

Canvas — центр.

Properties открываются side sheet по кнопке, а не занимают постоянную колонку.

На desktop Outline может быть слева, но auto-hide при узкой ширине.

---

# 20. Autosave и защита данных

## 20.1. State machine

```text
CLEAN
 → DIRTY
 → SAVING
 → SAVED

SAVING → CONFLICT
SAVING → OFFLINE
OFFLINE → RETRYING
```

UI всегда показывает состояние коротким текстом/icon.

## 20.2. Local recovery journal

До подтверждения server save сохранять local draft snapshot:

- document stable key;
- lock version;
- timestamp;
- content hash;
- content.

После подтверждённого server save старый local snapshot очищается.

При аварийном закрытии предлагается:

```text
Recovered unsaved changes from 22:14
[Compare] [Restore] [Discard]
```

Никакого silent overwrite.

## 20.3. Conflict UX

При optimistic lock conflict:

```text
This document changed elsewhere.

[Compare changes]
[Keep my version as change request]
[Reload latest]
```

Нельзя просто показывать raw HTTP 409.

---

# 21. Change Request model для опубликованных документов

Главное улучшение безопасности редактирования.

## Правило

- новый Draft можно редактировать напрямую;
- опубликованный документ по кнопке **Edit** создаёт Change Request;
- пользователь работает внутри change branch;
- original Published revision не меняется;
- затем Review → Merge/Publish.

## Сущности

```text
change_requests
  id
  document_id
  base_revision_id
  working_revision_id
  status: draft | in_review | approved | rejected | merged | abandoned
  title
  summary
  created_by
  created_at
  updated_at

change_request_reviewers
change_request_comments
```

## Views

- Editor;
- Changes;
- Preview;
- Overview/Review.

Это берёт лучший GitBook/GitHub workflow, но обычному пользователю показывается простая последовательность:

```text
Edit → Save automatically → Request review → Publish
```

---

# 22. Review UX

Reviewer видит:

- title/summary;
- author;
- affected document;
- changed sections;
- rendered diff;
- comments;
- preview;
- Approve / Request changes.

Не заставлять reviewer читать raw unified diff по Markdown, если он не хочет.

Два режима diff:

1. rendered block diff — default;
2. source diff — advanced.

---

# 23. Domain/Project-aware editor

Editor header всегда показывает context breadcrumb.

При Move document:

1. выбрать target Domain;
2. список Project уже ACL-filtered;
3. показать, изменится ли audience;
4. потребовать соответствующий permission на source и target;
5. если target restricted — показать warning;
6. move выполняется атомарно в Docs metadata;
7. security resource resolver автоматически использует новый project workspace.

Перемещение не должно копировать ACL вручную.

---

# 24. Templates

Templates становятся контекстными.

Уровни:

```text
Global templates
Domain templates
Project templates
```

Приоритет рекомендаций:

```text
Project > Domain > Global
```

Template содержит:

- Markdown skeleton;
- recommended document type;
- default tags;
- validation rules;
- optional review requirement.

Не хранить access policy в template.

---

# 25. Search UX после Domains/Projects

Global search ищет только по доступному scope.

Filters:

```text
Domain
Project
Status
Type
Updated
Owner
```

Default UI показывает только 2–3 самых полезных фильтра; остальные — `More filters`.

Search result:

```text
Document title
Engineering / HP4
matching snippet with highlight
Updated 2d ago · Published
```

Keyboard navigation Up/Down/Enter.

Command palette использует тот же SearchService, но другой result projection.

---

# 26. Dashboard `My Work`

Один персональный экран:

```text
Continue editing
Review requested
Drafts
Recently viewed
Pinned projects
```

Все списки ACL-scoped.

Это полезнее, чем заставлять человека искать работу по всем Domain вручную.

---

# 27. Mobile UX

На ширине compact mode:

- sidebar → drawer;
- editor → одна pane;
- properties → bottom sheet;
- slash palette → bottom command palette;
- save/review actions → safe-area sticky bottom bar;
- preview переключается tab;
- keyboard opening не закрывает primary action;
- touch target ≥44 CSS px;
- horizontal page overflow = 0.

Не делать desktop split editor на телефоне.

---

# 28. Accessibility

Обязательные gates:

- WCAG 2.2 AA;
- full keyboard navigation;
- visible focus;
- dialog focus trap + restore;
- combobox/listbox semantics для search/mentions/access picker;
- status announcements для autosave/upload/review;
- reduced-motion;
- 200% zoom without content loss;
- high contrast for editor diagnostics;
- screen-reader labels для access inheritance.

Playwright + axe должны быть blocking, без whitelist «до 10 violations».

---

# 29. Файловая карта изменений Docs_Hub

## Composition/security

```text
cmd/docshub/main.go
internal/bootstrap/app.go                 NEW
internal/security/service.go              NEW
internal/security/resource_resolver.go    NEW
internal/security/scope_provider.go       NEW
internal/security/bindings.go             NEW
```

Убирается production dependency на:

```text
internal/auth/*
internal/authn/*
internal/authz/*
```

после migration gate.

## Domain/project application

```text
internal/domain/domain.go                 NEW
internal/domain/project.go                NEW
internal/application/domain_service.go    NEW
internal/application/project_service.go   NEW
internal/repository/domain_repository.go  NEW
internal/repository/project_repository.go NEW
internal/repository/sqlite/domain_repo.go NEW
internal/repository/sqlite/project_repo.go NEW
```

## Search

```text
internal/application/search_service.go
internal/repository/sqlite/search_repo.go NEW/EXTRACT
```

Все list/search queries принимают security scope.

## HTTP

```text
internal/httpapp/handlers/domains.go      NEW
internal/httpapp/handlers/projects.go     NEW
internal/httpapp/handlers/access.go       NEW
internal/httpapp/handlers/documents.go
internal/httpapp/handlers/reviews.go      NEW
```

После convergence старый monolithic `server.go` уменьшается до wiring/legacy redirects и затем удаляется.

## UI

```text
internal/web/templates/domains/index.html
internal/web/templates/domains/show.html
internal/web/templates/projects/show.html
internal/web/templates/access/domain.html
internal/web/templates/access/project.html
internal/web/templates/editor.html
internal/web/templates/reviews/index.html
internal/web/static/js/editor/*
internal/web/static/js/access/*
internal/web/static/css/components/editor.css
internal/web/static/css/components/access.css
```

---

# 30. Миграции Docs_Hub

Предлагаемый порядок новых SQL migration files:

```text
008_domains.sql
009_domain_project_bindings.sql
010_security_binding_state.sql
011_change_requests.sql
012_editor_recovery_metadata.sql     // только если server metadata нужна
013_legacy_acl_migration.sql
014_legacy_auth_cleanup.sql          // только после release gate
015_spaces_to_projects.sql           // optional late cleanup
```

Ни одну уже применённую migration 001–007 не изменять.

---

# 31. API contracts

## Domains

```text
GET    /api/v1/domains
POST   /api/v1/domains
GET    /api/v1/domains/{id}
PATCH  /api/v1/domains/{id}
POST   /api/v1/domains/{id}/archive
```

## Projects

```text
GET    /api/v1/domains/{domain}/projects
POST   /api/v1/domains/{domain}/projects
GET    /api/v1/projects/{id}
PATCH  /api/v1/projects/{id}
POST   /api/v1/projects/{id}/archive
```

## Access

Docs API не принимает raw permission bits.

```text
GET    /api/v1/domains/{id}/members
POST   /api/v1/domains/{id}/members:bulkGrant
POST   /api/v1/domains/{id}/members:bulkRevoke
GET    /api/v1/projects/{id}/members
POST   /api/v1/projects/{id}/access-mode
GET    /api/v1/users/{id}/access
```

Handlers вызывают SecureAccess management service.

## Change requests

```text
POST   /api/v1/documents/{id}/changes
GET    /api/v1/changes/{id}
PUT    /api/v1/changes/{id}/draft
POST   /api/v1/changes/{id}/review
POST   /api/v1/changes/{id}/approve
POST   /api/v1/changes/{id}/request-changes
POST   /api/v1/changes/{id}/merge
```

---

# 32. Architecture tests

Добавить automated rules:

1. package `internal/httpapp` не импортирует `database/sql` после migration;
2. `internal/httpapp` не содержит role string comparisons;
3. Docs code не создаёт/валидирует session tokens;
4. Docs code не хеширует user passwords;
5. access decisions идут через SecureAccess facade;
6. search/list SQL принимает AccessScope;
7. `LIMIT` не появляется до access scope join в sensitive queries;
8. client workspace IDs не используются как authoritative security identifiers;
9. legacy ACL tables запрещены в production code после cleanup gate.

---

# 33. Security test matrix

Минимум:

| Scenario | Expected |
|---|---|
| User без Domain | Domain/document отсутствуют в UI/search/graph | Deny |
| Domain Reader | Project inherited docs | View only |
| Domain Editor | inherited Project | Edit |
| Restricted Project | Domain Editor без direct grant | Deny |
| Restricted Project direct Reader | project docs | View only |
| Restricted Document | project editor без doc grant | Deny |
| Revoked Domain membership | следующий request | Deny |
| Expired membership | следующий request | Deny |
| Search suggestion | inaccessible title | Never returned |
| Backlink from private doc | metadata | Never leaked |
| Attachment direct URL | inaccessible parent | Deny |
| Move document | no target edit permission | Deny |
| Publish | editor without publish bit | Deny |
| Manage members | publisher without manage bit | Deny |

Плюс mutation/race tests при revoke во время active edit/save.

---

# 34. Editor test matrix

## Functional

- type Markdown;
- live decorations;
- slash menu;
- keyboard selection;
- `[[` autocomplete;
- paste image;
- drag file;
- table builder;
- Mermaid;
- autosave;
- restore local draft;
- conflict;
- request review;
- diff;
- merge/publish.

## Browsers/devices

- Chromium;
- Firefox;
- WebKit;
- desktop 1440;
- iPad Mini portrait/landscape;
- Android-sized viewport;
- iPhone-sized viewport.

## Performance

Measure:

- keypress → paint;
- slash palette opening;
- autocomplete response;
- 10k / 50k / 100k-character documents;
- large tables;
- many code blocks;
- 20+ images;
- Mermaid heavy page.

No hard performance promise without baseline, but editor input p95 must stay within one animation frame on target desktop for ordinary documents.

---

# 35. UX acceptance criteria

## Domain/Project

- пользователь понимает, где находится, по breadcrumb без открытия settings;
- новый документ из Project не спрашивает Domain/Project повторно;
- доступ к Domain для нескольких пользователей выдаётся одной bulk operation;
- inherited/direct access различим одним взглядом;
- restricted Project создаётся не более чем через 2 explicit actions после открытия access panel;
- человек без access не видит название restricted content в search/suggest/activity;
- mobile navigation не требует горизонтального scroll.

## Editor

- после `New document` курсор в title/content без blocking modal;
- 90% форматирования доступны через shortcuts, `/` или selection toolbar;
- autosave state всегда понятен;
- закрытие вкладки после failed save не приводит к silent data loss;
- published content не меняется до merge change request;
- обычный автор не видит raw ACL/permission bits/security IDs.

---

# 36. Metrics

## Security

- unauthorized metadata leaks: 0;
- host-side authorization decisions: 0;
- stale session surviving revoke on next request: 0;
- legacy auth enforcement paths after migration: 0.

## Information architecture

- documents without Domain/Project after migration: 0;
- orphan security bindings: 0 after reconciler;
- projects without Domain: 0;
- duplicated active slugs inside same scope: 0.

## Editor

- autosave loss incidents in automated crash tests: 0;
- unresolved lock conflict silently overwritten: 0;
- horizontal overflow at tested mobile widths: 0;
- WCAG blocking violations: 0.

## Product

После telemetry baseline измерять:

- time to create first document;
- time to grant domain access;
- search → successful open rate;
- review cycle time;
- stale document rate;
- abandoned drafts/change requests.

Не собирать sensitive document text в analytics.

---

# 37. Поэтапная реализация

## Phase 0 — freeze contracts

1. Зафиксировать ADR `SecureAccess is the sole authorization authority`.
2. Зафиксировать hierarchy Organization → Domain → Project → Document.
3. Зафиксировать default inheritance semantics.
4. Зафиксировать role presets.
5. Characterization tests текущего auth/search/editor.
6. Снять baseline performance.

**Gate:** никаких новых ACL-фич в старой Docs authorization system.

## Phase 1 — SecureAccess vNext

1. Workspace access mode.
2. Hierarchical effective permissions.
3. Scope API.
4. Workflow permission bits.
5. Human management authorizer.
6. Workspace member listing/bulk operations.
7. Stable external key/idempotent ensure.
8. ExplainAccess.
9. Memory/Axiom adapter changes.
10. fuzz/race/benchmark.
11. release SecureAccess vNext.

**Gate:** Docs_Hub не начинает deletion legacy auth до опубликованной tested версии.

## Phase 2 — Docs security composition

1. `internal/bootstrap`.
2. SecureAccess store/service init.
3. Security bridge.
4. request session middleware.
5. ResourceResolver.
6. compatibility user mapping.
7. dual-decision diagnostics in tests.

## Phase 3 — Domains

1. migration 008;
2. Domain repository/service;
3. security workspace provisioning;
4. Domain list/home;
5. Domain access panel;
6. default General migration;
7. tests.

## Phase 4 — Projects

1. add Domain binding to spaces;
2. rename product concept to Project;
3. security child workspace;
4. inheritance/restricted mode;
5. project home/navigation;
6. legacy routes redirect;
7. tests.

## Phase 5 — ACL-scoped queries

В таком порядке:

1. direct document read;
2. attachments;
3. search;
4. suggestions;
5. project/domain listing;
6. backlinks;
7. graph;
8. activity;
9. dashboard;
10. AI retrieval.

После каждого vertical slice — leak regression tests.

## Phase 6 — editor engine

1. frontend build pipeline;
2. CodeMirror 6 baseline;
3. Markdown syntax/theme;
4. server-compatible preview;
5. slash commands;
6. selection toolbar;
7. `[[` autocomplete;
8. attachments paste/drop;
9. outline;
10. diagnostics;
11. Source/Split modes;
12. mobile layout.

## Phase 7 — autosave/recovery

1. unified draft endpoint;
2. local recovery journal;
3. sequential save queue;
4. conflict state machine;
5. compare/restore UX;
6. crash tests.

## Phase 8 — change requests/reviews

1. schema;
2. application service;
3. edit-published flow;
4. Changes view;
5. Preview;
6. reviewers;
7. approve/request changes;
8. merge/publish;
9. review dashboard.

## Phase 9 — access UX polish

1. bulk user picker;
2. role presets;
3. inherited/direct labels;
4. ExplainAccess;
5. user-centric access page;
6. access-change impact preview;
7. optional membership expiration.

## Phase 10 — legacy cleanup

После security parity gate:

1. disable old session creation;
2. remove old password login path или оставить только через SecureAccess provider adapter;
3. remove old authorizer;
4. remove direct ACL reads;
5. archive/drop legacy ACL tables only through migration;
6. delete dead code.

## Phase 11 — hardening

1. restore drills content + security state;
2. load tests;
3. access-scale benchmarks;
4. editor large-document tests;
5. Playwright full matrix;
6. accessibility;
7. security fuzzing;
8. production canary.

---

# 38. Рекомендуемая последовательность небольших PR

```text
DH-01 ADR + characterization tests
SA-01 Workspace inheritance model
SA-02 AccessScope + benchmarks
SA-03 permissions + management authorizer
SA-04 member/bulk/explain/idempotent provisioning
SA-05 release SecureAccess vNext
DH-02 bootstrap + SecureAccess bridge
DH-03 Domain schema/service
DH-04 Domain UI + access UI
DH-05 Project model over existing spaces
DH-06 Project inheritance/restricted access
DH-07 protected document/file routes
DH-08 ACL-aware search/suggest
DH-09 graph/backlinks/activity/dashboard scope
DH-10 editor build + CodeMirror baseline
DH-11 slash/wiki-link/attachment UX
DH-12 autosave recovery/conflict UX
DH-13 change requests schema/service
DH-14 review/diff/preview UI
DH-15 legacy ACL migration
DH-16 legacy auth removal
DH-17 backup/restore + observability
DH-18 full E2E/performance/accessibility hardening
DH-19 optional SecureAccess Teams/Groups integration
```

Каждый PR должен быть самостоятельно reviewable и иметь rollback path.

---

# 39. Что сознательно не делать сейчас

Не включать в ближайший critical path:

- microservices;
- обязательный PostgreSQL;
- Redis только ради sessions;
- отдельный vector DB без измеренного retrieval выигрыша;
- CRDT как prerequisite для хорошего editor UX;
- полный SPA rewrite;
- per-block ACL;
- произвольные deny rules на каждом документе;
- nested Domain глубже одного уровня;
- project nesting;
- десятки custom roles в UI.

Сначала сделать простую модель предсказуемой и доказуемой.

---

# 40. Definition of Done

Эта программа считается завершённой, когда одновременно выполняется следующее:

1. SecureAccess является единственным authority для login/session/membership/authorization.
2. Docs_Hub не имеет production password/session/ACL policy implementation.
3. Каждый документ принадлежит Project, каждый Project — Domain.
4. Domain и Project имеют stable SecureAccess workspace binding.
5. Domain access наследуется в Project по default; restricted Project работает fail-closed.
6. Search/suggest/graph/backlinks/activity/files фильтруются до ranking/LIMIT.
7. Пользователь управляет доступом через понятные roles, а не permission bits.
8. Пользователь видит источник effective access.
9. Editor реально использует CodeMirror 6 live editing, а не textarea-only surface.
10. Slash commands, wiki-link autocomplete, attachments и keyboard workflow работают на desktop/mobile.
11. Autosave имеет local recovery и conflict UX без silent overwrite.
12. Опубликованный документ редактируется через change request и не меняется до merge.
13. Review имеет rendered diff + preview.
14. Backup/restore восстанавливает content и security state согласованно.
15. WCAG/security/access regression tests являются blocking CI gates.
16. Старые Spaces/ACL/auth сущности либо мигрированы, либо явно остаются только как compatibility layer с датой удаления.

Итоговая цель — не просто добавить ещё несколько сущностей, а сделать Docs_Hub системой, где пользователь всегда понимает три вещи без инструкции: **где находится документ, кто его видит и что произойдёт с его изменениями**.
