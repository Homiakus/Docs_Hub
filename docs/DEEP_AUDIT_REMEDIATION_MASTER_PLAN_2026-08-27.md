# Docs_Hub — Deep Audit Remediation Master Plan

**Статус:** active source of truth для remediation после глубокого аудита  
**Дата:** 2026-08-27  
**Аудит-базис:** `main@cd628bd5d0a27d27acbb8ddf527317e885aaac46`  
**Область:** architecture, tenant isolation, authorization, sessions, comments, Domain/Project, query security, modularity, CI, Markdown/Slides, AutoTrace, UX hardening, observability, migrations and repository governance.

> Этот документ не заменяет продуктовые планы `PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`, `SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md` и `COLLAB_PRESENTATIONS_AUTOTRACE_PLAN_2026-08-26.md`. Он задаёт **обязательный порядок исправления фактических дефектов текущего `main`**. Если старый roadmap помечает пункт как выполненный, а текущий код или тест не доказывает требуемую гарантию, источником истины для remediation является этот документ.

---

## 0. Цель и release gate

Цель — довести Docs_Hub до состояния, в котором модель

```text
Organization -> Domain -> Project -> Document
```

является не только моделью данных и UI, но и **доказуемой security boundary** во всех путях чтения/изменения данных.

До закрытия P0 запрещено считать систему production-ready для недоверенных multi-tenant пользователей.

### Неподвижные правила

1. **Fail closed.** Неизвестный actor, organization, workspace, membership, access mode или policy result означает deny.
2. **Никаких production hardcode tenant IDs.** `OrganizationID: 1` допустим только в явно названных test fixtures.
3. **Authorization до данных.** Search/list/graph/backlinks/activity/AI retrieval должны применять scope до ranking, aggregation и `LIMIT`.
4. **SecureAcces — целевой security authority.** Docs_Hub хранит продуктовые metadata и stable bindings, но не создаёт второй долгоживущий независимый движок membership/policy.
5. **Новые handlers не ходят в SQL напрямую.** HTTP -> Application -> Ports -> Repository/Adapters.
6. **Каждый security fix сначала получает отрицательный regression test.**
7. **Никаких фиктивных `[x]`.** Пункт считается выполненным только если рядом зафиксированы commit SHA и команды/тесты, которые доказывают acceptance criteria.
8. **Не ослаблять тесты ради зелёного CI.** Исправляется код/fixture/environment, а не смысл проверки.
9. **Миграции только forward-only.** Уже применённые SQL-файлы не редактировать; следующая миграция начинается с `011_...`.
10. **После каждого завершённого этапа обновлять этот документ:** статус, SHA, evidence, известные остаточные риски.

---

# 1. Подтверждённые дефекты текущего baseline

## AUD-001 — hardcoded active organization — P0

Файл: `internal/httpapp/domains_handlers.go`  
Функция: `(*Server).actorFrom`

Сейчас HTTP actor формируется с `OrganizationID: 1`. Это делает новую application-модель tenant-aware по интерфейсам, но не по реальной identity boundary.

**Риск:** пользователь не получает доказанную active organization из security principal; cross-organization semantics невозможно считать корректными.

## AUD-002 — SecurityAdapter не проверяет organization membership — P0

Файл: `internal/authz/security_adapter.go`  
Функции: `RequireWorkspacePermission`, `ReadWorkspaceScope`

Текущая проверка читает `users.role` и `users.is_active`, но не доказывает membership actor в `actor.OrganizationID`. `ReadWorkspaceScope` затем собирает domain/project workspaces всей организации.

**Риск:** глобальная роль пользователя подменяет tenant-scoped membership; security contract и реализация расходятся.

## AUD-003 — project access mode не является реальной boundary — P0

Файл: `internal/authz/security_adapter.go`  
Функции: `EnsureProjectWorkspace`, `SetProjectAccessMode`, `ReadWorkspaceScope`

`accessMode` частично хранится в process-local map. При вычислении read scope `spaces.access_mode`/effective SecureAcces policy не определяют доступ пользователя.

**Риск:** `restricted` может отображаться как product state без гарантированного ограничения данных.

## AUD-004 — unauthorized comment resolve / IDOR-class mutation — P0

Файл: `internal/httpapp/comment_handlers.go`  
Функция: `apiResolveComment`

Resolve принимает `commentID` и вызывает repository update без проверки `comment -> document -> workspace -> permission`.

**Риск:** аутентифицированный пользователь может менять состояние чужого comment thread по ID.

## AUD-005 — comment parent integrity не доказана — P0/P1

Файлы: `internal/httpapp/comment_handlers.go`, `internal/db/migrations/009_anchored_comments.sql`, comment repository.

`parent_id` имеет FK на `comments(id)`, но схема не доказывает `parent.document_id == child.document_id`.

**Риск:** cross-document thread corruption, неоднозначная moderation/notification semantics.

## AUD-006 — SessionManager не соответствует заявленной модели idle timeout — P0

Файл: `internal/authn/session.go`

`idleTimeout` задан, но `ValidateSession` проверяет только абсолютный `expires_at`; `randomString` игнорирует ошибку `crypto/rand`; IP/UA передаются в `CreateSession`, но не входят в полезный session inventory/audit path.

**Риск:** security documentation/roadmap сильнее фактических гарантий.

## AUD-007 — `main` governance слабее требуемой — P0 operational

На baseline `main` не protected. Последний CI run на baseline завершён `failure`: `Unit & Security Tests` и `Container Build Verification` зелёные, падает `E2E & Responsive Quality Gate` на шаге `Execute Playwright Matrix`.

**Риск:** regression может непосредственно попасть в `main`, а общий status остаётся красным несмотря на зелёный backend gate.

## AUD-008 — roadmap state drift — P1 governance

Существующие roadmap-файлы содержат `[ВЫПОЛНЕНО]` для гарантий, которые текущий baseline не доказывает. Это не дефект runtime, но дефект engineering control plane.

**Риск:** агент/разработчик принимает декларацию за факт и строит новый функционал поверх незакрытой security boundary.

---

# 2. Целевая архитектура после remediation

```text
HTTP / Web / Bot / future API
            |
            v
      Request Principal
            |
            v
+----------------------------+
| Application                |
| DomainProjectService       |
| DocumentService            |
| CommentService             |
| SearchService              |
| WorkflowService            |
+-------------+--------------+
              |
       Security Ports
              |
       +------+-------+
       |              |
       v              v
SecureAcces       Repositories
(authority)       (product data)
       |              |
       +------+-------+
              v
       SQLite/Postgres
```

### Ownership rule

**SecureAcces owns:** authentication principal, account/user identity, memberships, workspace hierarchy security semantics, grants, effective authorization, query-safe access scope, explain-access.

**Docs_Hub owns:** organizations/domains/projects/documents as product metadata, stable keys, mapping to security workspace IDs, document workflow/content, comments, presentation metadata, diagram source, search index payload.

**Bridge owns:** deterministic binding between product resource and SecureAcces workspace/resource ID; no policy duplication.

---

# 3. Порядок реализации

## PHASE P0.0 — восстановить достоверный baseline и зелёный gate

**Цель:** нельзя менять security semantics поверх недостоверного CI baseline.

### P0.0.1 — E2E failure triage

- [ ] Открыть artifact/report последнего `E2E & Responsive Quality Gate`.
- [ ] Зафиксировать точные failing specs, browser(s), expected/actual и screenshot/trace.
- [ ] Классифицировать каждую ошибку: product regression / flaky synchronization / fixture / environment.
- [ ] Исправить root cause без удаления assertion и без увеличения timeout как единственного fix.
- [ ] Если тест flaky, заменить sleep/time-based ожидание на deterministic locator/state/network condition.
- [ ] Добавить regression test, если failure обнаружил продуктовый дефект.
- [ ] Повторить Playwright matrix локально/CI минимум два последовательных раза.

**Gate:** весь CI green, включая Chromium/Firefox/WebKit matrix.

### P0.0.2 — зафиксировать evidence convention

Добавить в этот файл под каждым завершённым phase:

```text
Status: DONE
Commit: <sha>
Evidence:
- go test -race ./...
- <targeted tests>
- <CI run URL/id>
Residual risks: none | ...
```

### P0.0.3 — governance

После восстановления зелёного CI:

- [ ] определить обязательные checks: Unit & Security Tests, Container Build Verification, E2E & Responsive Quality Gate;
- [ ] включить branch/ruleset protection для `main`, если инфраструктура аккаунта позволяет;
- [ ] запретить merge при красных required checks;
- [ ] запретить force-push/delete `main`;
- [ ] сохранить возможность emergency admin bypass только как audited exception;
- [ ] если direct push необходим в текущем автономном цикле, protection включить после завершения P0 и далее работать через PR/merge-to-main.

**Acceptance:** `main` имеет понятный policy и не может считаться green при упавшем E2E.

---

# 4. PHASE P0.1 — настоящий request principal и tenant identity

**Цель:** полностью удалить hardcoded organization selection из production flow.

## P0.1.1 — ввести единственный principal resolver

Создать/нормализовать boundary, например:

```go
type RequestPrincipal struct {
    UserID               int64
    ActiveOrganizationID int64
    // SecureAcces principal/binding identifiers as needed.
}
```

Предпочтительное размещение:

- `internal/security/principal.go`
- `internal/httpapp/principal_middleware.go` или после архитектурного split `internal/web/middleware/principal.go`.

Требования:

- [ ] principal получается из доверенной server-side session/SecureAcces context;
- [ ] client не может назначить себе organization простой подстановкой query/header;
- [ ] смена active organization проходит отдельный authorized use case;
- [ ] membership проверяется до фиксации active organization;
- [ ] деактивированный membership немедленно fail-closed;
- [ ] отсутствие active organization даёт deny/явный selection flow, а не fallback `1`.

## P0.1.2 — удалить hardcode

Изменить:

- `internal/httpapp/domains_handlers.go::actorFrom`;
- все другие `OrganizationID: 1`, `organization_id = 1`, fallback-default paths в production code;
- test fixtures явно переименовать, чтобы default org в тесте не выглядел production assumption.

Обязательный repo-wide search:

```text
OrganizationID: 1
organization_id = 1
organization_id=? ... 1
Default Organization assumptions
```

## P0.1.3 — SecureAcces bridge

Не создавать параллельную долгоживущую таблицу `organization_memberships` в Docs_Hub, если membership уже является responsibility SecureAcces.

Bridge должен уметь как минимум:

```go
ResolvePrincipal(ctx, session) (Principal, error)
RequireOrganizationAccess(ctx, principal, organizationBinding) error
RequireWorkspacePermission(ctx, principal, workspaceID, permission) error
ReadWorkspaceScope(ctx, principal) (AccessScope, error)
ExplainAccess(ctx, principal, workspaceID) (...)
```

Если SecureAcces API временно не даёт нужный query scope, сделать явно помеченный compatibility adapter с expiry condition, но не выдавать его за финальную policy implementation.

## P0.1.4 — отрицательная tenant matrix

Добавить тесты уровня application + HTTP:

- [ ] user A / org A -> domain A = allow;
- [ ] user A / org A -> domain B = deny;
- [ ] user with global local role `admin` but no membership org B -> deny;
- [ ] inactive membership -> deny;
- [ ] deleted/revoked membership -> deny;
- [ ] forged organization request -> deny;
- [ ] no active organization -> deny/selection response;
- [ ] organization switch invalidates/recomputes scope.

Рекомендуемые имена:

```text
TestPrincipalRejectsForgedOrganization
TestDomainQueryCrossOrganizationDenied
TestGlobalRoleDoesNotBypassOrganizationMembership
TestOrganizationSwitchRecomputesAccessScope
```

**Phase gate:** production code не содержит hardcoded active organization, а cross-org tests зелёные.

---

# 5. PHASE P0.2 — Project access mode как настоящая security boundary

**Цель:** `inherit` и `restricted` должны менять effective access, а не только metadata/UI.

## P0.2.1 — убрать process-local authority

Из `internal/authz/security_adapter.go` удалить роль `accessMode map[...]...` как authoritative state.

Допускается cache только если:

- source of truth external/persistent;
- cache имеет invalidation/version;
- stale cache никогда не расширяет permission;
- restart не меняет effective access.

## P0.2.2 — определить semantics

Минимальная модель текущей схемы:

```text
Domain workspace
  |
  +-- Project access_mode=inherit
  |      -> inherits effective Domain access
  |
  +-- Project access_mode=restricted
         -> inheritance broken; only explicit project grants + privileged policy
```

Запретить двусмысленность между local `spaces.access_mode` и SecureAcces policy. `spaces.access_mode` — продуктовая metadata; effective security rule применяется security authority.

## P0.2.3 — saga изменения access mode

`SetProjectAccessMode` должен стать application use case, а не простой mutation памяти.

Порядок:

1. load project metadata;
2. validate actor/manage permission;
3. apply/update SecureAcces workspace inheritance policy idempotently;
4. persist local metadata;
5. audit event;
6. при частичном failure выполнить retry/compensation и не показывать ложное успешное состояние.

Определить idempotency key по stable project key + target mode.

## P0.2.4 — query scope

`ReadWorkspaceScope` не должен делать:

```text
active user -> all workspaces organization
```

Вместо этого:

```text
principal -> SecureAcces AccessScope -> only effective readable workspace IDs
```

Scope применяется в repository query **до** pagination/ranking.

## P0.2.5 — тест-матрица inheritance/restricted

- [ ] domain member -> inherit project = allow;
- [ ] domain member -> restricted project without grant = deny;
- [ ] restricted explicit member -> allow;
- [ ] revoke project grant -> deny immediately;
- [ ] switch inherit -> restricted removes inherited access;
- [ ] switch restricted -> inherit restores domain-derived access;
- [ ] restart process leaves decisions unchanged;
- [ ] search/graph/backlinks/activity do not reveal restricted project metadata;
- [ ] admin semantics are organization-scoped, not global-user-role bypass.

**Phase gate:** никаких process-local permission decisions; restricted project закрыт и в direct read, и в indirect retrieval surfaces.

---

# 6. PHASE P0.3 — Comments authorization и целостность threads

**Цель:** comments становятся полноценным application module с одинаковой permission semantics для read/create/resolve/reopen/delete.

## P0.3.1 — repository capabilities

В `internal/repository/interfaces.go`/comment repository добавить минимальные операции:

```go
GetCommentByID(ctx, id)
GetCommentsByDocument(ctx, documentID)
CreateComment(ctx, comment)
ResolveComment(ctx, id, ...)
ReopenComment(ctx, id, ...)
DeleteComment(ctx, id, ...)
```

Repository не принимает решение «кто имеет право»; он выполняет persistence после разрешённого use case.

## P0.3.2 — создать `CommentService`

Новый файл: `internal/application/comment_service.go`.

Use cases:

```text
ListComments
CreateComment
ReplyComment
ResolveComment
ReopenComment
DeleteComment / ModerateComment
```

Каждый mutation:

1. resolve comment/document;
2. resolve project/workspace security binding;
3. authorize action;
4. validate state transition/invariants;
5. mutate repository;
6. write audit event.

## P0.3.3 — fix resolve IDOR

`internal/httpapp/comment_handlers.go::apiResolveComment` должен вызывать application service и никогда не изменять comment status только по `commentID`.

Negative tests:

```text
TestResolveCommentRequiresDocumentAccess
TestResolveCommentCrossProjectDenied
TestResolveCommentCrossOrganizationDenied
TestResolveCommentUnknownCommentDoesNotLeakMetadata
```

Ответы 403/404 выбрать осознанно так, чтобы endpoint не становился oracle существования недоступных comments.

## P0.3.4 — parent invariant

При `ParentID != nil`:

- load parent;
- parent must exist and not be deleted;
- `parent.DocumentID == target DocumentID`;
- при необходимости запретить reply к неподдерживаемому статусу;
- глубина thread либо не ограничивается, либо явно ограничивается application rule.

Добавить test:

```text
TestCreateCommentRejectsParentFromAnotherDocument
```

DB-level enforcement для same-document parent в SQLite сложно выразить простым FK; основной invariant держать в application transaction, а migration `011_...` использовать для необходимых index/columns/status constraints/triggers только если они действительно упрощают доказательство.

## P0.3.5 — state machine

Определить состояния минимум:

```text
open -> resolved -> open
open/resolved -> deleted (soft delete)
orphaned как anchor state, а не permission bypass
```

Не кодировать status transitions разрозненными SQL updates.

**Phase gate:** ни одна comment mutation не выполняется без document/workspace authorization.

---

# 7. PHASE P0.4 — SessionManager hardening

**Цель:** привести runtime к документированным security guarantees.

## P0.4.1 — secure randomness errors

Изменить:

```go
func randomString(n int) string
```

на:

```go
func randomString(n int) (string, error)
```

Использовать `io.ReadFull(rand.Reader, b)` или эквивалент с обязательной обработкой error.

`CreateSession` fail-closed при невозможности получить entropy.

Тест через injectable random reader или малый internal seam:

```text
TestCreateSessionFailsWhenRandomSourceFails
```

## P0.4.2 — token MAC/compare

Предпочтительно перейти на HMAC-SHA256:

```text
HMAC(sessionSecret, token)
```

и constant-time compare binary digest/hash.

Выбрать migration strategy:

- simplest secure option для pre-release: invalidate existing sessions once;
- либо versioned hash format с коротким dual-read window.

Не усложнять migration без необходимости.

## P0.4.3 — idle timeout

Следующая migration должна добавить или нормализовать:

```text
sessions.last_seen_at
sessions.created_at
sessions.expires_at
optional ip/user_agent metadata for inventory/audit
```

Validation:

1. parse timestamps в `time.Time`, не полагаться на lexical string compare;
2. `now > expires_at` -> revoke;
3. `now - last_seen_at > idleTimeout` -> revoke;
4. touch `last_seen_at` с write-throttling, например не чаще заданного окна;
5. absolute lifetime никогда не продлевается touch-операцией.

## P0.4.4 — session inventory/revocation

- [ ] list active sessions пользователя;
- [ ] revoke one;
- [ ] revoke all other;
- [ ] deactivation -> revoke all;
- [ ] organization/membership revocation не обязана удалять session, но authorization должна отказать немедленно;
- [ ] IP/UA используются как информационный audit signal, не как жёсткий binding по умолчанию.

## P0.4.5 — tests

```text
TestSessionIdleTimeout
TestSessionAbsoluteLifetime
TestSessionTouchDoesNotExtendAbsoluteLifetime
TestSessionTokenMismatchRejected
TestSessionRevocation
TestInactiveUserRejected
```

**Phase gate:** runtime действительно реализует idle + absolute expiry, entropy errors не игнорируются.

---

# 8. PHASE P0.5 — data/model invariants и compatibility exit plan

## P0.5.1 — не редактировать 008/009/010

Новые schema fixes — только новой migration `011_<purpose>.sql` и далее.

## P0.5.2 — Domain/Project invariants

Проверить и зафиксировать:

- [ ] каждый active Project имеет `domain_id`;
- [ ] каждый Domain имеет `organization_id`;
- [ ] project domain и legacy `spaces.organization_id` не расходятся;
- [ ] security workspace IDs unique/non-empty после provisioning;
- [ ] stable keys immutable на application level;
- [ ] document принадлежит ровно одному project (`space_id`) после backfill;
- [ ] orphaned legacy records отсутствуют перед contract phase.

Добавить consistency diagnostics command/test, например:

```text
docshub doctor --security-bindings
```

или внутренний repository validation suite.

## P0.5.3 — compatibility exit

Текущая схема сознательно использует `spaces` как physical project table. Зафиксировать этапы:

```text
expand -> backfill -> dual compatibility -> parity tests -> contract
```

Не делать physical rename ради эстетики до полного code convergence.

**Phase gate:** consistency test может автоматически доказать отсутствие cross-org/domain/project anomalies.

---

# 9. PHASE P1.1 — authorization как query invariant на всех поверхностях

**Цель:** устранить metadata leaks после исправления primary read path.

Составить таблицу всех retrieval surfaces:

| Surface | Must use AccessScope before LIMIT/rank |
|---|---|
| domain list | yes |
| project list | yes |
| document list/tree | yes |
| search/FTS | yes |
| wiki-link autocomplete | yes |
| backlinks | yes |
| knowledge graph | yes |
| activity/recent | yes |
| dashboard counts | yes |
| files/attachments | yes |
| comments list | yes |
| AI/RAG retrieval | yes |
| presentations | yes |
| diagram source/render fetch | yes |

Для каждого surface создать отрицательный тест на скрытый restricted resource.

### Architecture test

Ввести test/code-review rule: запрещён pattern

```text
query N global rows -> LIMIT -> authz post-filter
```

Допустимо:

```text
AccessScope -> SQL predicate/join/temp table -> rank/order -> LIMIT
```

Для больших scopes заранее предусмотреть backend-neutral abstraction, не зашивая giant `IN (...)` как единственный Enterprise path.

---

# 10. PHASE P1.2 — архитектурная конвергенция HTTP -> Application

**Цель:** `server.go` перестаёт быть местом, где смешаны business rules, SQL и security.

## P1.2.1 — dependency rule

Зафиксировать:

```text
httpapp/web -> application -> domain/ports
repository adapters -> domain/repository interfaces
security adapters -> application security ports
```

Запретить новые импорты `internal/db` из handlers, кроме composition/bootstrap concerns.

## P1.2.2 — перенос по вертикальным slices

Порядок с наименьшим риском:

1. Comments;
2. Domain/Project actor/security flows;
3. document read/write;
4. search/suggestions;
5. files;
6. graph/backlinks/activity;
7. admin operations.

После каждого slice удалить дублирующую legacy logic.

## P1.2.3 — split server

Целевая структура без обязательного большого rename за один commit:

```text
internal/httpapp/
  server.go              # composition/routes only
  middleware_*.go
  domains_handlers.go
  projects_handlers.go
  documents_handlers.go
  comments_handlers.go
  search_handlers.go
  files_handlers.go
  admin_handlers.go
```

Далее при стабильности можно перейти к `internal/web/handlers`.

**Metric:** `server.go` не растёт; direct SQL/permission decisions в handlers стремятся к нулю.

---

# 11. PHASE P1.3 — SecureAcces integration parity gate

Следовать `SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md`, но добавить обязательный parity gate перед удалением legacy auth.

## Parity matrix

Сравнить old/new decisions на fixture corpus:

```text
subject x organization x domain x project x resource x action
```

Для каждой строки классифицировать difference:

- intended stricter result;
- intended product change;
- bug;
- legacy bug that must not be preserved.

Удалять production use старых `internal/authn`/`internal/authz` только когда:

- all required actions доступны SecureAcces;
- request principal migrated;
- query AccessScope migrated;
- sessions migrated or intentionally retained as thin client to SecureAcces;
- negative matrix green;
- rollback documented.

Нельзя держать два authority path с логикой `try new, fallback old allow`.

---

# 12. PHASE P1.4 — Markdown/SVG security corpus и presentation semantics

## P1.4.1 — sanitizer corpus

Для `internal/markdownx/markdown.go` добавить table-driven + fuzz tests:

```text
javascript: URLs
SVG onload/onerror
foreignObject
style/url(...)
data: payloads
malformed nested SVG
encoded entities
mixed Markdown + raw HTML
broken tags/attributes
very deep input
```

Invariant: renderer либо удаляет опасный payload, либо безопасно экранирует; никогда не генерирует executable markup.

## P1.4.2 — explicit presentation dialect

Для `internal/markdownx/slides.go` уйти от семантической зависимости только от `---`.

Предпочтительно:

```yaml
---
presentation: true
---
```

и явный slide directive, например `<!-- slide -->` или AST directive.

Backward compatibility с существующим `---` допускается за feature/version flag, но обычный thematic break не должен неожиданно менять document semantics.

Tests:

```text
TestThematicBreakDoesNotSplitNormalDocument
TestExplicitSlideDirective
TestPresentationFrontmatter
```

---

# 13. PHASE P1.5 — AutoTrace: adapter вместо второго routing engine

**Цель:** Docs_Hub не должен независимо догонять AutoTraceLab по алгоритмам.

Текущий встроенный renderer сохранить как fallback/minimal compatibility только если это нужно для offline/basic diagrams.

Целевая граница:

```go
type DiagramEngine interface {
    Render(ctx context.Context, source []byte, opts RenderOptions) (RenderResult, error)
}
```

Adapters:

```text
internal/diagram/autotrace/adapter.go -> reusable AutoTrace core
internal/diagram/simple/...          -> optional fallback
```

Требования:

- [ ] versioned input/output contract;
- [ ] deterministic rendering for same input/options;
- [ ] bounded resource limits;
- [ ] no network access from document-controlled source;
- [ ] render cache key includes engine version/options/source hash;
- [ ] golden SVG/layout tests;
- [ ] malformed graph property/fuzz tests;
- [ ] access checked before source/render retrieval.

Сложные algorithms (obstacle avoidance, channels, ports, crossings, incremental scene, spline refinement) развивать в reusable AutoTrace core, не копировать в Docs_Hub.

---

# 14. PHASE P1.6 — repository governance и stale PR cleanup

Открытые stacked PR по Domain/Project не должны оставаться ложной картой delivery state.

Порядок:

1. enumerate PR #3–#8;
2. для каждого сравнить head с актуальным main;
3. определить уже landed commits/features;
4. выделить residual diff;
5. если residual нужен — перенести/rebase в чистый PR;
6. superseded PR закрыть с комментарием, куда landed функционал;
7. не merge conflicting historical stack вслепую.

После cleanup GitHub PR graph должен снова отвечать на вопрос «что реально осталось внедрить».

---

# 15. PHASE P2.1 — Domain/Project UX после закрытия security boundary

Новые UX возможности разрешены после P0, чтобы UI не обещал security semantics раньше backend.

## Domain navigation

- Home / My Work / Domains / Pinned / Recent;
- Domain home: projects, recent docs, members/access summary;
- Project home: tree, changes/reviews, activity, files, access state;
- breadcrumbs всегда показывают Domain -> Project -> Document.

## Access UX

- default project state: `Inherited from Domain`;
- restricted project — одно явное действие с предупреждением об изменении effective access;
- показывать effective access source (`direct`, `inherited`, `restricted explicit`);
- admin может просмотреть user-centric access map;
- UI никогда не решает permission самостоятельно — только отображает server decision/explain result.

## Error UX

Развести:

```text
not authenticated
no active organization
not a member
resource inaccessible/not found
insufficient action permission
security service unavailable
```

При security service unavailable — fail closed, но UI должен объяснять operational outage без раскрытия закрытых metadata.

---

# 16. PHASE P2.2 — Editor/collaboration reliability

Синхронизировать с `MARKDOWN_EDITOR_MASTER_SPEC.md` и collaboration plan.

Приоритеты:

1. autosave state machine;
2. crash/offline recovery;
3. conflict handling через lock/version;
4. anchored comment rebind/orphan state;
5. comment deep links;
6. mentions/notifications после permission-safe core;
7. Live Preview/CodeMirror 6;
8. accessibility/mobile regression matrix.

Ни один autosave/comment endpoint не должен обходить application authorization boundary.

---

# 17. PHASE P2.3 — observability и auditability

Security-sensitive events:

```text
login/session create/revoke
active organization switch
membership/access decision denial (sampled where noisy)
domain/project create/archive
project access mode change
explicit grant/revoke
comment resolve/reopen/delete
workflow publish/archive
file download/export of protected content
admin changes
```

Audit event должен иметь:

```text
timestamp
actor id
active organization
resource stable id/security binding
action
result
correlation/request id
source (web/api/bot)
non-secret context
```

Не писать session token, CSRF secret, magic-link token, password/secret material.

Добавить metrics:

- authz deny/error rate;
- SecureAcces latency/error rate;
- scope size distribution;
- search latency by scope size;
- session validation/revocation errors;
- comment mutation errors;
- AutoTrace render duration/failure/cache hit.

---

# 18. PHASE P2.4 — performance, allocations, concurrency

После correctness/security выполнить измеримый performance pass.

## Go benchmarks

Добавить benchmarks для:

```text
AccessScope translation/filtering
Domain/Project list queries
FTS search with ACL scope
comment list/re-anchor helpers
Markdown render/sanitize
AutoTrace adapter/render
```

## Allocation targets

Не задавать выдуманные абсолютные числа заранее. Снять baseline `ns/op`, `B/op`, `allocs/op`, затем ставить budgets для hot paths и regression thresholds.

## Concurrency

Обязательно:

```text
go test -race ./...
```

Проверить:

- session touch;
- caches;
- project mode updates;
- concurrent document save;
- comment resolve/reopen race;
- AutoTrace render cache.

---

# 19. PHASE P2.5 — test architecture

Минимальная pyramid:

### Unit

- domain invariants;
- application state transitions;
- permission mapping;
- parser/renderer helpers.

### Integration SQLite

- migrations from clean + previous schema;
- repository transactions;
- query AccessScope;
- tenant matrices.

### HTTP

- authn/authz status semantics;
- CSRF;
- body limits;
- IDOR regressions;
- metadata non-disclosure.

### E2E Playwright

- desktop Chromium/Firefox/WebKit;
- mobile viewport;
- organization switch;
- inherited/restricted project UX;
- comments resolve/reopen;
- editor autosave/conflict;
- presentations;
- diagrams.

### Fuzz/property

- Markdown/SVG sanitizer;
- slide parser;
- comment anchor/re-anchor;
- stable key/slug validation;
- diagram parser/contract.

### Security regression naming

Security defects получают постоянный test с названием по invariant, а не по номеру тикета, например:

```text
TestCrossOrganizationDocumentReadDenied
TestRestrictedProjectExcludedFromSearch
TestCommentResolveRequiresResourcePermission
```

---

# 20. PHASE P3 — release hardening

Перед объявлением production multi-tenant readiness:

- [ ] все P0/P1 закрыты;
- [ ] no known cross-tenant access bug;
- [ ] CI green на нескольких последовательных main builds;
- [ ] backup/restore drill выполнен;
- [ ] migration upgrade test с production-like fixture;
- [ ] dependency/vulnerability scan green;
- [ ] security configuration documented;
- [ ] session invalidation/migration policy documented;
- [ ] SecureAcces outage behavior tested fail-closed;
- [ ] AutoTrace resource limits tested;
- [ ] audit events checked на secret leakage;
- [ ] threat model обновлён;
- [ ] release checklist содержит explicit tenant-isolation sign-off.

---

# 21. Конкретная карта файлов

| Файл/область | Изменение |
|---|---|
| `internal/httpapp/domains_handlers.go` | удалить hardcoded organization; actor только из trusted principal |
| `internal/authz/security_adapter.go` | перестать использовать global role как membership; удалить authoritative in-memory accessMode; получать effective scope из security authority |
| `internal/application/domain_project_service.go` | сохранить business boundary; добавить/уточнить mode-change saga и security failure handling |
| `internal/application/domain_project_query_service.go` | принимать только policy-derived query scope; не расширять scope самостоятельно |
| `internal/httpapp/comment_handlers.go` | thin handlers; resolve/reopen/delete через `CommentService` |
| `internal/application/comment_service.go` | новый module authorization + thread invariants + transitions |
| `internal/repository/interfaces.go` | comment lookup/mutation contracts, scoped query contracts |
| `internal/repository/sqlite/comment_repo.go` | persistence only; atomic mutations; no hidden policy |
| `internal/authn/session.go` | entropy errors, HMAC/constant-time, idle timeout, parsed times, session touch/revocation |
| `internal/db/migrations/011_*.sql` | только необходимые session/comment/security metadata fixes; не менять старые migrations |
| `internal/markdownx/markdown.go` | security corpus/fuzz; maintain strict sanitizer invariants |
| `internal/markdownx/slides.go` | explicit presentation syntax/compatibility |
| `internal/diagram/autotrace/*` | adapter boundary to reusable core; deterministic/golden tests |
| `internal/httpapp/server.go` | постепенно оставить composition/routing; выносить SQL/business/security decisions |
| `.github/workflows/ci.yml` | сохранить race/vuln/container gates; сделать E2E deterministic и required |
| `docs/ROADMAP.md` | после первых remediation phases синхронизировать status с evidence, не заранее |
| `Docs_Hub_Enterprise_Roadmap.md` | исправить устаревшие `[ВЫПОЛНЕНО]` только после фактической parity проверки |

---

# 22. Рекомендуемый порядок атомарных commits

Не смешивать security semantic change с большим UI refactor.

```text
1. test(e2e): reproduce current responsive/e2e failure
2. fix(e2e|ui): restore deterministic green baseline
3. test(security): add cross-organization negative matrix
4. refactor(security): add trusted request principal boundary
5. fix(security): remove hardcoded organization actor
6. test(security): add restricted project scope regressions
7. fix(security): make access scope authority-backed
8. fix(security): persist/apply project inheritance mode
9. test(comments): reproduce unauthorized resolve
10. refactor(comments): add application CommentService
11. fix(comments): enforce resource permission and parent invariant
12. test(authn): add entropy/idle/session expiry regressions
13. fix(authn): harden SessionManager
14. db(migrations): add forward-only schema support
15. test(retrieval): add indirect metadata leak matrix
16. refactor(http): migrate retrieval slices to application layer
17. test(markdown): add sanitizer fuzz corpus
18. refactor(slides): explicit presentation dialect
19. refactor(diagram): introduce AutoTrace engine adapter
20. chore(github): reconcile stale stacked PRs / roadmap evidence
```

Каждый commit должен быть самостоятельным настолько, насколько это возможно, и оставлять repository buildable/testable.

---

# 23. Definition of Done для каждого phase

Phase нельзя отмечать DONE, пока не выполнено всё:

1. implementation завершена;
2. targeted unit/integration tests зелёные;
3. `go test -race ./...` зелёный;
4. `go vet ./...` зелёный;
5. `govulncheck ./...` зелёный или документировано нерелевантное исключение;
6. затронутый Playwright matrix зелёный;
7. no new direct SQL/security decisions in HTTP;
8. documentation отражает реальное состояние;
9. plan обновлён SHA + evidence;
10. changes landed in `main` согласно актуальной repository policy.

Для security phase дополнительно:

11. есть минимум один отрицательный regression test;
12. deny path не раскрывает лишние metadata;
13. restart/process recreation не меняет permission semantics;
14. partial external-security failure fail-closed;
15. rollback/compensation описан для saga/migration.

---

# 24. Stop conditions

Немедленно остановить текущий feature phase и вернуться к defect, если:

- обнаружен cross-organization read/write;
- restricted project появляется в search/graph/autocomplete без permission;
- security authority unavailable приводит к allow;
- migration может необратимо потерять content без backup/rollback strategy;
- CI стал красным после stage и root cause не понятен;
- для исправления предлагается убрать/ослабить regression test;
- roadmap и runtime снова расходятся по security guarantee.

---

# 25. Progress ledger

| Phase | Status | Commit | Evidence | Residual risk |
|---|---|---|---|---|
| P0.0 Baseline/CI | TODO | — | — | E2E gate currently failing |
| P0.1 Principal/Tenant | TODO | — | — | hardcoded org |
| P0.2 Project boundary | TODO | — | — | broad workspace scope |
| P0.3 Comments | TODO | — | — | resolve authorization gap |
| P0.4 Sessions | TODO | — | — | idle timeout/entropy handling |
| P0.5 Invariants | TODO | — | — | compatibility model |
| P1.1 Query invariant | TODO | — | — | indirect metadata leaks need matrix |
| P1.2 HTTP/Application | TODO | — | — | mixed architecture |
| P1.3 SecureAcces parity | TODO | — | — | dual authority risk |
| P1.4 Markdown/Slides | TODO | — | — | sanitizer/parser corpus incomplete |
| P1.5 AutoTrace adapter | TODO | — | — | duplicate simplified engine |
| P1.6 Governance | TODO | — | — | stale PR graph |
| P2.x Product hardening | TODO | — | — | starts after P0/P1 |
| P3 Release gate | TODO | — | — | not production multi-tenant ready |

---

# 26. Главный Pareto-маршрут

Если ресурсов мало, первые изменения должны идти строго так:

```text
1. Green E2E baseline
2. Trusted Principal / remove OrganizationID=1
3. Membership-aware / authority-backed AccessScope
4. Restricted Project enforcement
5. Comment resolve + parent invariant
6. Session entropy + idle timeout
7. Retrieval metadata leak matrix
8. Application boundary convergence
9. SecureAcces parity and legacy removal
10. Only then editor/UX/AutoTrace expansion
```

Именно эти пункты дают максимальный выигрыш по снижению риска и архитектурной неопределённости без переписывания проекта с нуля.
