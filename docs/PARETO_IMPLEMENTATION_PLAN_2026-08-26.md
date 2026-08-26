# Docs Hub Next — Pareto implementation plan

**Дата:** 2026-08-26  
**Baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`  
**Цель:** получить максимальный прирост безопасности, поддерживаемости, качества поиска и эксплуатационной надёжности без преждевременного усложнения архитектуры.

---

## 1. Executive summary

Docs Hub уже имеет сильный продуктовый фундамент: Go-монолит, SQLite/WAL, FTS5, wiki-links/backlinks, версии документов, workflow, ACL-таблицы, адаптивный UI, PDF.js, Playwright matrix и несколько deployment-профилей.

Главная проблема текущего состояния — не отсутствие ещё десятка enterprise-функций, а **расхождение между целевой архитектурой и реально исполняемым кодом**:

- в `internal/domain`, `internal/application`, `internal/authn`, `internal/authz`, `internal/repository` уже существует новая модульная архитектура;
- `cmd/docshub` при этом по-прежнему запускает большой `internal/httpapp/server.go`, содержащий собственные модели, SQL, авторизацию, workflow, backup/admin и значительную часть бизнес-логики;
- конструкторы `NewDocumentService`, `NewArticleRepository`, `NewOIDCAuthService` в текущем `main` фактически не подключены к composition root;
- ACL описан богаче в схеме БД (`space_members`, `role_bindings`, `document_permissions`), чем в исполняемой policy-логике;
- часть выборок получает кандидатов из БД, а затем фильтрует доступ в Go. Для поиска это особенно критично: `LIMIT` применяется до access filtering, поэтому одновременно ухудшаются и безопасность архитектуры, и полнота выдачи;
- AI retrieval пока является заготовкой: документ дробится по пустым строкам, а каждому chunk присваивается фиксированный `Score = 0.95`;
- CI уже хороший, но `govulncheck` не блокирует сборку, отсутствуют fuzz targets/реальные benchmarks, release supply-chain можно усилить;
- SQLite открыт с WAL, но `SetMaxOpenConns(1)` сериализует весь доступ к БД и не позволяет использовать конкурентное чтение WAL — менять это нужно только после воспроизводимого benchmark/load baseline.

### Pareto-решение

Вместо немедленного внедрения CRDT, PostgreSQL, Redis, OpenSearch и микросервисов сначала выполнить шесть вертикальных улучшений:

| Приоритет | Изменение | Эффект | Стоимость | Почему сейчас |
|---|---|---:|---:|---|
| **P0** | Один исполняемый архитектурный путь | 5/5 | 3/5 | Убирает двойную бизнес-логику и ускоряет все следующие изменения |
| **P0** | ACL как инвариант каждого запроса | 5/5 | 3/5 | Закрывает главный security/correctness риск и улучшает поиск |
| **P0** | Измеримый ACL-aware search/retrieval | 5/5 | 3/5 | Поиск — центральный UX базы знаний и основа будущего RAG |
| **P1** | Единые sessions + production OIDC | 4/5 | 3/5 | Убирает вторую реализацию auth и делает enterprise login реальным |
| **P1** | Backup/restore + observability + DB baseline | 4/5 | 2/5 | Даёт доказуемую эксплуатационную надёжность вместо деклараций |
| **P1** | CI/supply-chain + Go 1.27 | 4/5 | 2/5 | Низкая стоимость, большой выигрыш в раннем обнаружении дефектов |
| **P2** | Governance качества знаний | 3/5 | 2/5 | Делает базу знаний долговечной после стабилизации ядра |

---

## 2. Принципы реализации

### 2.1. Сохранять модульный монолит

Не переходить к микросервисам. Для текущего масштаба один Go binary + SQLite является преимуществом. Нужны чёткие границы модулей и один composition root, а не распределённая система.

### 2.2. Безопасность должна быть свойством запроса, а не постфильтром

Доступность документа должна учитываться **до `LIMIT`, ranking, aggregation и построения graph/backlinks/activity**. Ни один handler не должен самостоятельно интерпретировать роль пользователя.

### 2.3. Сначала измерять, потом усложнять

PostgreSQL, Redis, vector DB, OpenSearch, CRDT и дополнительные workers внедрять только после измеряемого ограничения Team Edition.

### 2.4. Один источник истины на каждую ответственность

- domain model — `internal/domain`;
- use cases — `internal/application`;
- authentication — `internal/authn`;
- authorization — `internal/authz`;
- persistence — `internal/repository` + `internal/db`;
- HTTP — только transport/adapters;
- rendering/static UI — `internal/web`.

### 2.5. Каждое изменение должно иметь proof

Каждый этап обязан завершаться не только кодом, но и:

1. автоматическими тестами;
2. измеримым acceptance criterion;
3. rollback/compatibility plan;
4. обновлением ADR/API/runbook при изменении контракта.

---

# 3. P0-A — свести приложение к одному исполняемому архитектурному пути

## Проблема

`internal/httpapp/server.go` остаётся вторым application layer: собственные `User`, `Article`, `Space`, прямой SQL, sessions, ACL, workflow и admin logic существуют параллельно с уже созданными `domain/application/authn/authz/repository` пакетами.

Это создаёт наиболее дорогой класс дефектов: исправление может быть внесено в «правильный» слой, но реальный HTTP path продолжит выполнять старую реализацию.

## Целевое состояние

```text
cmd/docshub
    ↓
internal/bootstrap  ← composition root
    ├── repository/sqlite
    ├── authn
    ├── authz
    ├── application
    └── httpapp/web handlers
```

`httpapp` знает HTTP, cookies, status codes, request/response DTO и templates, но не содержит SQL и не решает бизнес-политику.

## Атомарный план

### A0. Зафиксировать поведение до рефакторинга

- [ ] Добавить characterization tests для всех текущих маршрутов `Routes()`.
- [ ] Зафиксировать сценарии: login/logout, article read, search, suggest, spaces, graph, create/edit/autosave, workflow, upload/PDF, admin ACL, backup/import.
- [ ] Для write-paths проверять не только HTTP status, но и итоговое состояние БД.
- [ ] Зафиксировать текущие security invariants отдельным table-driven набором.
- [ ] Добавить fixture-generator для 10k документов / 100k links / нескольких spaces и ACL-комбинаций.

**Готово, когда:** рефакторинг транспорта можно выполнять без визуального/поведенческого дрейфа и тесты ловят смену доступа, workflow и autosave semantics.

### A1. Создать composition root

- [ ] Добавить `internal/bootstrap/app.go`.
- [ ] В одном месте создавать DB, repositories, Authorizer, SessionManager и application services.
- [ ] Передавать зависимости в HTTP server явно; исключить service locator/global state.
- [ ] Свести `cmd/docshub/main.go` к config/logging/lifecycle/bootstrap/server shutdown.
- [ ] Добавить compile-time interface assertions для repository adapters.

### A2. Мигрировать вертикальными slices, а не переписывать всё сразу

Порядок:

1. [ ] auth/session;
2. [ ] article read + list;
3. [ ] search/suggest;
4. [ ] editor/autosave;
5. [ ] workflow/revisions;
6. [ ] attachments/PDF;
7. [ ] graph/backlinks/activity;
8. [ ] admin/ACL;
9. [ ] backup/import.

Для каждого slice:

- [ ] handler получает DTO;
- [ ] вызывает application service;
- [ ] application service вызывает repository/policy;
- [ ] прямой SQL из handler удаляется;
- [ ] старый helper удаляется сразу после перевода последнего caller;
- [ ] characterization tests остаются зелёными.

### A3. Удалить дубли

- [ ] Удалить доменные `User/Article/Space/...` из `httpapp`, оставить только view models там, где они действительно отличаются от domain.
- [ ] Удалить неиспользуемые application/authn/authz/repository реализации или, предпочтительно, сделать их реальным production path.
- [ ] Добавить архитектурный test/lint-rule: `internal/httpapp` не импортирует `database/sql` и не обращается к `*db.DB` напрямую.
- [ ] Ограничить `server.go` router/composition glue; handlers разнести по предметным файлам.

### A4. Transaction ownership

- [ ] Определить правило: транзакцию начинает application use case, если операция затрагивает несколько repositories/derived indexes/audit.
- [ ] Ввести небольшой `UnitOfWork`/transaction adapter без generic framework.
- [ ] Autosave, publish, ACL update и attachment-linking делать атомарно вместе с derived data/audit.

## Acceptance criteria

- [ ] Production startup реально использует `application`, `authn`, `authz`, `repository`.
- [ ] Нет прямого SQL в HTTP handlers.
- [ ] Нет двух реализаций sessions/permissions/workflow.
- [ ] Все существующие Go + Playwright tests проходят без изменения пользовательских сценариев.

---

# 4. P0-B — authorization как единый query invariant

## Проблема

Схема БД уже поддерживает richer model (`space_members`, `role_bindings`, `document_permissions` с allow/deny), но `DefaultAuthorizer` в основном опирается на global role, owner и visibility. Параллельно repositories/httpapp имеют собственные `CanRead/CanEdit`.

Выборки search/list/space/graph/backlinks могут сначала читать более широкий набор данных, а затем фильтровать его в Go.

## Целевая модель

Использовать сочетание RBAC + relationship/context-aware authorization:

```text
Subject
  ├── global role
  ├── organization bindings
  ├── space memberships / role bindings
  └── explicit document permissions

Resource
  ├── organization
  ├── space
  ├── document owner
  ├── visibility/classification
  └── workflow status

Decision = deny-by-default + explicit policy precedence
```

### B0. Зафиксировать policy contract ADR

- [ ] Добавить ADR `0005-authorization-policy-and-access-scope.md`.
- [ ] Явно определить precedence. Рекомендуемая база: explicit deny > explicit document allow > scoped role > global role > visibility default.
- [ ] Определить semantics owner/admin и emergency/break-glass поведения.
- [ ] Определить 403 vs 404 policy, чтобы private-resource existence не утекал через ответы.
- [ ] Определить workflow visibility: кто видит `draft`, `in_review`, `approved`, `published`, `archived`.

### B1. Ввести `AccessScope`

- [ ] `authz.ResolveScope(ctx, subject)` строит компактный access context: user, groups, org roles, space roles.
- [ ] Repository list/search methods принимают scope/query constraints, а не вызывают policy по каждой строке.
- [ ] Для resource-specific mutation остаётся `Authorizer.Check(...)` перед изменением.
- [ ] Удалить policy logic из repository (`CanRead`, `CanEdit`) после миграции callers.

### B2. Перенести доступ внутрь SQL

Перевести по очереди:

- [ ] article list;
- [ ] FTS search;
- [ ] suggestions;
- [ ] space counters/lists;
- [ ] backlinks/wiki-links;
- [ ] graph nodes и edges;
- [ ] recent activity;
- [ ] attachment serving;
- [ ] export/backup scopes, если появятся user-scoped exports.

Ключевое правило: **authorization predicate применяется до ORDER BY/LIMIT/ranking**.

### B3. Индексы

Создать новую миграцию (не менять применённые `001..007`):

- [ ] `008_authorization_indexes.sql`;
- [ ] индексы на `space_members(subject_type, subject_id, space_id)`;
- [ ] `role_bindings(subject_type, subject_id, scope_type, scope_id)`;
- [ ] `document_permissions(subject_type, subject_id, document_id, permission, effect)`;
- [ ] индексы `articles(space_id,status,visibility,deleted_at)` по фактическому query plan;
- [ ] перед merge приложить `EXPLAIN QUERY PLAN` для критичных запросов.

Не создавать denormalized permission cache до измеренной необходимости.

### B4. Authorization test matrix

- [ ] Table-driven tests: global roles × space roles × ownership × visibility × workflow status × explicit allow/deny × action.
- [ ] Property-style invariants: добавление deny не может расширить доступ; reader не получает edit/publish; anonymous не получает private metadata.
- [ ] Regression tests на search/graph/backlinks/activity metadata leaks.
- [ ] Test fixtures минимум с двумя spaces, конфликтующими grants и explicit deny.

## Acceptance criteria

- [ ] На основных read paths нет post-filtering чувствительных сущностей после `LIMIT`.
- [ ] Одна policy implementation используется всеми transports/use cases.
- [ ] Поиск никогда не теряет разрешённый результат из-за того, что запрещённые документы заняли pre-filter `LIMIT`.
- [ ] Authorization matrix проходит полностью.

---

# 5. P0-C — поиск и retrieval как измеряемая подсистема

## Проблема

FTS5/BM25 — правильная лёгкая основа для Team Edition, но сейчас search и AI retrieval не имеют формальной relevance evaluation. `AIService` дробит текст по пустым строкам и выдаёт фиксированный score, поэтому он не должен считаться production RAG foundation.

## Цель

Сделать поиск:

- ACL-aware до ranking;
- детерминированным и измеримым;
- хорошим для обычного человека без AI;
- пригодным как retrieval layer для будущего assistant/RAG;
- опционально hybrid, не ломая single-binary профиль.

### C0. Search contract

- [ ] Добавить `internal/search/interfaces.go`.
- [ ] Ввести `SearchRequest`: query, space, status, classification, language, tags, page/cursor, limit.
- [ ] Ввести `SearchHit`: document/revision/chunk IDs, title, heading, snippet, score, highlights, metadata.
- [ ] Application `SearchService` всегда получает `AccessScope`.

### C1. Улучшить SQLite FTS baseline

- [ ] ACL predicate до FTS result limit.
- [ ] Настроить BM25 weights: title > tags/headings > body; веса определить evaluation, а не вкусовщиной.
- [ ] Добавить безопасную query normalization/escaping.
- [ ] Добавить snippets/highlights.
- [ ] Объединить space/status/classification/language filters с ranking одним query plan.
- [ ] Добавить стабильную pagination/cursor semantics.
- [ ] Проверить tokenizer для русского/английского контента и стратегию prefix matching.

### C2. Правильная индексация chunks

- [ ] Chunking строить из Markdown AST по heading boundaries, а не `strings.Split("\n\n")`.
- [ ] Stable chunk key: document stable key + revision + heading path + ordinal.
- [ ] Хранить heading path, section text, document metadata и revision.
- [ ] Reindex должен быть идемпотентным и транзакционным с publication state.
- [ ] Удалённый/недоступный документ не должен оставлять searchable orphan chunks.

### C3. Relevance evaluation harness

Добавить:

```text
testdata/search/
  corpus/
  judgments.json
  queries.json
internal/search/eval/
```

Метрики:

- [ ] MRR@10;
- [ ] nDCG@10;
- [ ] Recall@20;
- [ ] zero ACL leaks;
- [ ] p50/p95 latency на фиксированном corpus.

Набор должен содержать русский и английский, exact terms, acronyms, typo-like prefixes, tags, headings, конфликтующие ACL и несколько spaces.

### C4. Убрать фиктивный AI score

- [ ] `AIService.SearchKnowledgeContext` использует `SearchService`.
- [ ] Каждый chunk получает реальный score/rank и source coordinates.
- [ ] AI context всегда несёт document/revision/heading identifiers для последующей цитируемости.
- [ ] Никакая генерация не получает chunks, которые пользователь не мог бы открыть напрямую.

### C5. Hybrid retrieval — только как optional backend

После сильного lexical baseline:

- [ ] определить `SemanticRetriever` interface;
- [ ] поддержать локальный/внешний embeddings provider за feature flag;
- [ ] fusion выполнять через Reciprocal Rank Fusion (RRF) или другой метод, выбранный по evaluation;
- [ ] не вводить обязательный OpenSearch/vector DB для Team Edition;
- [ ] включать hybrid по умолчанию только если он статистически улучшает test judgments при приемлемом latency/cost.

## Acceptance criteria

- [ ] Все search surfaces используют один `SearchService`.
- [ ] Есть versioned relevance dataset и regression gate.
- [ ] AI retrieval не содержит fixed score/paragraph-number heading.
- [ ] Zero unauthorized hits — отдельная обязательная метрика.

---

# 6. P1-D — единая session subsystem и production-ready OIDC

## Проблема

`internal/authn/session.go` и OIDC scaffold уже существуют, но HTTP server продолжает иметь собственный login/session path. OIDC service не подключён. В scaffold есть `idleTimeout`, который не используется при validation, а provisioning назначает новому OIDC-пользователю роль `editor` без policy mapping.

### D0. Сделать `authn.SessionManager` единственным session path

- [ ] Перенести create/validate/revoke из `httpapp` в `authn`.
- [ ] Обрабатывать ошибки CSPRNG; не игнорировать `rand.Read`.
- [ ] Использовать constant-time comparison/HMAC для token verification.
- [ ] Реализовать absolute lifetime + idle timeout.
- [ ] Обновлять `last_seen_at` с throttling, не писать БД на каждый request.
- [ ] Ротировать session после login, privilege change, password reset и OIDC re-auth.
- [ ] Revoke all sessions при деактивации/критической смене credentials.

### D1. Cookie/cache hardening

- [ ] `Secure`, `HttpOnly`, подходящий `SameSite`.
- [ ] При HTTPS предпочесть host-only cookie (`__Host-...`, `Path=/`, без `Domain`).
- [ ] `Cache-Control: no-store` для auth/private responses, где это требуется.
- [ ] Logout очищает cookie и server-side session.
- [ ] Не логировать session IDs/tokens/CSRF secrets.

### D2. Завершить OIDC

- [ ] Discovery metadata + JWKS validation.
- [ ] Authorization Code + PKCE.
- [ ] `state` + `nonce`, expiry и one-time consumption.
- [ ] Проверка issuer, audience, signature, expiry, nonce; configurable `email_verified` policy.
- [ ] JIT provisioning сделать явной настройкой.
- [ ] **Не выдавать `editor` по умолчанию**: default `reader` или no-access до role mapping.
- [ ] Настроить mapping OIDC groups → organization/space roles.
- [ ] Аудитировать sign-in/provisioning/mapping changes.

### D3. SCIM как следующий enterprise step

- [ ] После OIDC добавить минимальный SCIM 2.0 `/Users` и `/Groups` только при реальном enterprise use case.
- [ ] Scoped bearer tokens, rotation, audit, pagination/filter semantics.
- [ ] Provision/deprovision должен немедленно влиять на sessions/access scope.

## Acceptance criteria

- [ ] Одна session implementation.
- [ ] OIDC интеграционный test с локальным fake provider.
- [ ] Новый OIDC user не получает write-доступ без mapping/policy.
- [ ] Deprovision/disable закрывает активный доступ.

---

# 7. P1-E — доказуемая надёжность: DB, backup/restore, observability

## E0. Migration safety

Текущий `schema_migrations` хранит только имя версии и timestamp.

- [ ] Не изменять `001..007` задним числом.
- [ ] В новой миграционной инфраструктуре хранить checksum и application version.
- [ ] При несовпадении checksum уже применённой миграции — fail closed с диагностикой.
- [ ] Исключить параллельный migration runner.
- [ ] Добавить upgrade tests как минимум с нескольких поддерживаемых предыдущих schema snapshots.

### E1. Backup — проверять восстановление, а не наличие файла

- [ ] Использовать безопасный online SQLite snapshot path (SQLite Backup API или корректный `VACUUM INTO` workflow).
- [ ] Писать сначала во временный файл, затем atomic rename.
- [ ] Manifest: app version, schema version, timestamp, source DB size, SHA-256.
- [ ] После backup запускать verification (`PRAGMA quick_check`/`integrity_check` на копии).
- [ ] Добавить CLI: `docshub backup create`, `backup verify`, `restore --dry-run`.
- [ ] Документировать RPO/RTO и restore runbook.
- [ ] E2E test: seed → backup → destroy temp DB → restore → compare critical records/search/access.

### E2. SQLite concurrency — только после benchmark

Сейчас `SetMaxOpenConns(1)` сериализует доступ.

- [ ] Создать benchmark/load scenario: concurrent readers, autosave writer, search, graph, upload metadata.
- [ ] Снять baseline p50/p95/p99 и `SQLITE_BUSY` count.
- [ ] Проверить раздельный read pool + single writer либо configurable pool.
- [ ] Проверить WAL checkpoint latency/starvation.
- [ ] Изменять pool defaults только если benchmark показывает выигрыш без regression по lock contention/memory.

### E3. Минимальная observability без обязательной инфраструктуры

- [ ] Structured logs с `request_id`, route, status, duration; без secrets/content by default.
- [ ] Метрики: request count/latency/errors, auth failures, DB busy/tx duration, search latency/result count, autosave conflict count, backup outcome/duration, WAL/checkpoint stats.
- [ ] Добавить `/readyz` отдельно от `/healthz`.
- [ ] Optional OpenTelemetry exporter через config; приложение должно работать без collector.
- [ ] Профилирование (`pprof`) только opt-in и на защищённом/local admin endpoint.

### E4. SLO

Начальные SLO задавать после baseline, минимум:

- read/search availability;
- p95 document read;
- p95 search;
- save/autosave error rate;
- backup success;
- restore verification success.

## Acceptance criteria

- [ ] Backup считается успешным только после verification.
- [ ] Restore тестируется автоматически.
- [ ] Есть latency/error baseline до оптимизации connection pool.
- [ ] Production incident можно диагностировать по request/DB/search/backup signals.

---

# 8. P1-F — CI, supply chain и актуальный Go toolchain

На 2026-08-26 Go 1.27 уже выпущен; проект закреплён на Go 1.25. Переход выполнять отдельным небольшим PR после зелёного baseline.

## F0. Сделать существующие проверки строгими

- [ ] Удалить `continue-on-error: true` у `govulncheck` либо разделить policy на явно зафиксированные allowlisted exceptions.
- [ ] `go test -race -shuffle=on ./...`.
- [ ] `go vet ./...` + `staticcheck` или согласованный `golangci-lint` profile.
- [ ] Coverage gate не общий ради процента, а минимум для `authn`, `authz`, `markdownx`, repository access paths.

## F1. Fuzzing

Добавить native Go fuzz targets для наиболее выгодных парсеров/границ доверия:

- [ ] Markdown/wiki-link/tag parser;
- [ ] slug/path normalization;
- [ ] Obsidian ZIP/import path traversal;
- [ ] upload metadata/MIME parsing;
- [ ] search query normalization;
- [ ] session cookie parser.

Seed corpus хранить в репозитории; найденный crash автоматически становится regression test.

## F2. GitHub Actions hardening

- [ ] Job-level minimal `permissions`.
- [ ] Pin third-party actions на commit SHA.
- [ ] Dependabot/Renovate для Go, npm и actions.
- [ ] OpenSSF Scorecard workflow/report.
- [ ] Branch protection: required CI + review + no force pushes на `main`.

## F3. Release integrity

- [ ] SBOM (SPDX/CycloneDX) рядом с release artifacts.
- [ ] Provenance/attestation для release build.
- [ ] Подписывать release artifacts/checksums (например, Sigstore/cosign keyless, если подходит deployment model).
- [ ] Release job должен запускать/требовать те же security gates, что PR.
- [ ] Исправить version injection: текущий `const Version` не должен конфликтовать с `-ldflags -X` release strategy; сделать переменную build metadata единственным source of truth.

## F4. Go 1.27

- [ ] Обновить `go.mod`, CI, Dockerfile и docs согласованно.
- [ ] Выполнить `go fix ./...` и полный test/race/e2e suite.
- [ ] Проверить release cross-compilation на linux/windows/darwin amd64/arm64.
- [ ] Зафиксировать version policy: текущий Go stable + при необходимости предыдущий stable в compatibility matrix.

## Acceptance criteria

- [ ] Известная уязвимость не проходит CI молча.
- [ ] Fuzz corpus покрывает trust-boundary parsers.
- [ ] Release содержит checksum + SBOM + provenance/signature policy.
- [ ] Версия binary совпадает с release tag.

---

# 9. P2 — governance качества знаний

Этот этап даёт продуктовый выигрыш после стабилизации ядра и поиска.

## G0. Использовать уже заложенный lifecycle metadata

В schema уже есть `review_due_at`/`expires_at`.

- [ ] Добавить явного content owner/maintainer.
- [ ] Review cadence и stale state.
- [ ] Dashboard: «требует проверки», «истекает», «без владельца», «битые ссылки», «orphan docs».
- [ ] История verification/review должна быть аудируемой.
- [ ] Search ranking может понижать stale/expired документы, но не скрывать их без явной политики.

## G1. Типология документации

Добавить templates и UX-подсказки по Diátaxis:

- [ ] tutorial;
- [ ] how-to;
- [ ] reference;
- [ ] explanation;
- [ ] сохранить прикладные SOP/post-mortem/meeting templates как отдельные бизнес-типы.

Не навязывать структуру существующим Markdown-файлам; metadata/template — помощь, а не lock-in.

## G2. Quality feedback loop

- [ ] «Полезно / не помогло» с optional reason.
- [ ] Search zero-result и reformulation telemetry с privacy-safe policy.
- [ ] Список популярных документов без owner/review.
- [ ] Broken wiki-link scanner и backlink consistency check.

---

# 10. Что сознательно НЕ делать в первых этапах

Следующие решения выглядят современно, но сейчас имеют худшее отношение эффект/сложность:

- **не переходить на микросервисы**;
- **не делать PostgreSQL обязательным** до измеренного ограничения SQLite Team Edition;
- **не добавлять Redis** только ради «enterprise stack»;
- **не делать OpenSearch/vector DB обязательным** до relevance/load evidence;
- **не внедрять CRDT realtime editing** до завершения единой permission/workflow модели;
- **не переписывать интерфейс в SPA** — текущие server-rendered templates + progressive JS уже дают хороший UX;
- **не проводить ещё один косметический redesign** до архитектурных/security/search этапов;
- **не делать AI центральным способом навигации**, пока обычный search не имеет измеримой relevance baseline.

---

# 11. Рекомендуемая последовательность небольших PR

Каждый PR должен быть mergeable самостоятельно и не оставлять две production-реализации одного use case дольше следующего PR.

### PR-01 — Characterization & performance baseline
- [ ] route/security behavior tests;
- [ ] search corpus + initial metrics;
- [ ] DB/load baseline;
- [ ] no product behavior changes.

### PR-02 — Composition root + sessions
- [ ] `internal/bootstrap`;
- [ ] wire repositories/services;
- [ ] `authn.SessionManager` production path;
- [ ] remove duplicate session logic.

### PR-03 — Document read/list/search through application layer
- [ ] article read/list;
- [ ] search/suggest;
- [ ] no direct SQL for these handlers.

### PR-04 — Central authorization policy
- [ ] ADR 0005;
- [ ] `AccessScope`;
- [ ] policy matrix tests;
- [ ] read/search ACL inside SQL.

### PR-05 — Authorization on every derived surface
- [ ] spaces;
- [ ] graph;
- [ ] backlinks;
- [ ] activity;
- [ ] files;
- [ ] migration `008_authorization_indexes.sql` after query-plan evidence.

### PR-06 — Workflow/editor/application convergence
- [ ] autosave and optimistic lock via service/UoW;
- [ ] workflow/revisions/reviews via one implementation;
- [ ] remove duplicate direct SQL path.

### PR-07 — Search quality & evaluation
- [ ] SearchService;
- [ ] weighted FTS/snippets/filters;
- [ ] evaluation harness + CI regression gate.

### PR-08 — Retrieval/AI cleanup
- [ ] Markdown AST chunks;
- [ ] real scores/source coordinates;
- [ ] optional semantic retriever + RRF behind feature flag only if evaluation wins.

### PR-09 — OIDC hardening
- [ ] PKCE/discovery/JWKS/state/nonce;
- [ ] role/group mapping;
- [ ] least-privilege provisioning;
- [ ] fake-provider integration tests.

### PR-10 — Backup, restore, observability
- [ ] backup manifest/verify/restore test;
- [ ] DB metrics + request/search metrics;
- [ ] `/readyz`, optional OTel.

### PR-11 — CI/supply chain + Go 1.27
- [ ] blocking vulnerability scan;
- [ ] fuzzing;
- [ ] action pinning/minimal permissions;
- [ ] SBOM/provenance/signing;
- [ ] Go upgrade.

### PR-12 — Knowledge governance
- [ ] review/stale ownership UX;
- [ ] quality/broken-link dashboards;
- [ ] Diátaxis templates.

---

# 12. Definition of Done для всего Pareto-плана

## Архитектура

- [ ] `httpapp` не содержит persistence/business policy.
- [ ] Все production use cases проходят через application layer.
- [ ] Нет dead duplicate architecture.

## Безопасность

- [ ] Deny-by-default.
- [ ] Один Authorizer/AccessScope contract.
- [ ] ACL применяется до ranking/limit/aggregation.
- [ ] Нет metadata leaks в search/graph/backlinks/activity/files.

## Поиск

- [ ] Versioned relevance corpus.
- [ ] MRR/nDCG/Recall измеряются CI/benchmark pipeline.
- [ ] Zero unauthorized search hits.
- [ ] AI retrieval использует ту же ACL-aware search substrate.

## Надёжность

- [ ] Восстановление из backup автоматически проверяется.
- [ ] Migration integrity проверяется checksum/version policy.
- [ ] Есть DB/search/request operational signals.

## Supply chain

- [ ] Vulnerability scan blocking.
- [ ] Trust-boundary parsers fuzzed.
- [ ] Release reproducibly связан с tag/version и сопровождается SBOM/provenance policy.

## UX/knowledge

- [ ] Существующий WCAG 2.2/mobile E2E gate не деградировал.
- [ ] Content owner/review/staleness доступны как рабочий lifecycle, а не только поля схемы.

---

# 13. Метрики, по которым оценивать эффект

Не использовать «количество реализованных фич» как основную метрику.

| Область | Метрика |
|---|---|
| Authorization | число известных bypass/leak paths = 0 |
| Architecture | прямой SQL в HTTP handlers = 0; duplicate production implementations = 0 |
| Search | MRR@10, nDCG@10, Recall@20, zero ACL leaks, p95 latency |
| Editing | autosave conflict/data-loss regression = 0 |
| DB | p95 transaction latency, busy errors, WAL checkpoint duration |
| Reliability | verified backup success, restore drill success, RPO/RTO |
| Security CI | unreviewed critical/high findings passing CI = 0 |
| Release | binary version/tag consistency; SBOM/provenance attached |
| Knowledge quality | stale/no-owner/broken-link rates и динамика |

---

# 14. Современные практики, на которых основан план

Приоритеты выше выбраны не по модности технологий, а по пересечению современных практик с фактической архитектурой Docs Hub:

- OWASP Authorization Cheat Sheet — least privilege, deny-by-default, permission checks on every request, предпочтение attribute/relationship-aware моделей при сложных ресурсных связях: https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html
- OWASP Session Management Cheat Sheet — жизненный цикл session IDs, cookie hardening, transport/cache requirements: https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html
- SQLite WAL — concurrent readers/single writer и checkpoint trade-offs: https://www.sqlite.org/wal.html
- SQLite Online Backup API: https://www.sqlite.org/backup.html
- OpenTelemetry — vendor-neutral traces/metrics/logs: https://opentelemetry.io/docs/
- OpenSearch hybrid search и rank fusion concepts: https://docs.opensearch.org/latest/vector-search/ai-search/hybrid-search/index/
- Elastic hybrid search/RRF guidance: https://www.elastic.co/docs/solutions/search/hybrid-search
- SCIM 2.0 protocol, RFC 7644: https://www.rfc-editor.org/rfc/rfc7644
- OpenSSF Scorecard — repository/supply-chain security practices: https://github.com/ossf/scorecard
- WCAG 2.2: https://www.w3.org/TR/WCAG22/
- Diátaxis documentation framework: https://diataxis.fr/start-here/
- Go release history: https://go.dev/doc/devel/release

---

# 15. Итоговая рекомендация

Если реализовать только первые три P0-направления — **архитектурная конвергенция, query-level authorization и измеримый search/retrieval** — Docs Hub получит непропорционально большой выигрыш:

1. исчезает главный источник расхождения между «написанным» и «исполняемым» кодом;
2. security model становится проверяемой и масштабируется на search/graph/files/AI без копирования логики;
3. центральная функция базы знаний — поиск — становится одновременно точнее, безопаснее и готовой к будущему semantic/RAG слою;
4. все следующие enterprise-функции можно добавлять поверх стабильных границ, а не наращивать большой `server.go`.

Именно эти изменения следует считать обязательным следующим релизным циклом. OIDC, observability, supply-chain hardening и content governance идут сразу следом. CRDT/PostgreSQL/Redis/OpenSearch остаются условными ветвями, которые включаются только после измеренного требования.