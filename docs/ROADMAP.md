# Product Roadmap — Docs Hub Next

**Актуализировано:** 2026-08-26  
**Docs_Hub baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`  
**SecureAcces baseline:** `main@827abb1add11a9fcbd0a9944e65efbd20c675739`

Главные документы:

- **[Pareto Implementation Plan](PARETO_IMPLEMENTATION_PLAN_2026-08-26.md)** — архитектурная конвергенция, search/retrieval, reliability и engineering quality.
- **[SecureAccess + Domains + Projects + Editor Master Plan](SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md)** — целевая продуктовая модель, security boundary, информационная архитектура и новый редактор.

## Что уже является сильной базой

- [x] Go single-binary application и SQLite WAL persistence.
- [x] FTS5/BM25, tags, wiki-links, backlinks и knowledge graph.
- [x] Revision/workflow schema и optimistic locking/autosave baseline.
- [x] Responsive editorial UI, mobile hardening и WCAG-oriented interaction model.
- [x] PDF.js viewer и managed attachments baseline.
- [x] Playwright E2E matrix для Chromium, Firefox и WebKit.
- [x] Docker/native deployment tooling, healthcheck, CI и release workflow.
- [x] Начаты domain/application/repository слои.
- [x] SecureAcces уже предоставляет Account/User/Workspace/Membership, sessions, request Principal, `Authorize`, HTTP middleware и durable Axiom/Pebble adapter.

---

# P0 — новый фундамент продукта

## P0.1 — SecureAccess как единственный security authority

- [ ] Зафиксировать ADR: Docs_Hub не реализует собственную authentication/authorization policy.
- [ ] Выпустить SecureAccess vNext с hierarchical Workspace inheritance (`INHERIT` / `ISOLATED`).
- [ ] Добавить в SecureAccess query-safe `AccessScope` для фильтрации до SQL `LIMIT`/ranking.
- [ ] Добавить content workflow permissions: comment/review/publish/archive.
- [ ] Добавить human workspace management authorizer, bulk memberships и `ExplainAccess`.
- [ ] Добавить stable external workspace binding / idempotent provisioning.
- [ ] Подключить SecureAccess через новый Docs_Hub `internal/bootstrap` и thin `internal/security` bridge.
- [ ] Перевести sessions/login/resource authorization на SecureAccess.
- [ ] После parity gate удалить production use старых `internal/authn`, `internal/authz` и legacy ACL.

## P0.2 — Domains → Projects → Documents

- [ ] Ввести `Domain` как тематическую область и основную границу доступа.
- [ ] Ввести `Project` внутри Domain как рабочий контекст документов.
- [ ] Domain = SecureAccess Workspace; Project = child Workspace.
- [ ] Project по умолчанию наследует Domain access; `Restricted Project` разрывает inheritance.
- [ ] Документ принадлежит ровно одному Project.
- [ ] Restricted document использовать только как advanced exception через dedicated child security workspace.
- [ ] Мигрировать существующие `spaces` в Projects compatibility-first: сначала `domain_id`, затем product rename, physical rename только после code convergence.
- [ ] Сделать canonical navigation `/domains/{domain}/projects/{project}/...` при сохранении redirect/stable resolver для старых URLs.

## P0.3 — Authorization как query invariant

- [ ] ResourceResolver получает security IDs только из authoritative Docs metadata.
- [ ] Search/list APIs получают SecureAccess `AccessScope`.
- [ ] Применять scope до ranking, `LIMIT` и aggregation.
- [ ] Перевести document read, files, search, suggestions, domains/projects, backlinks, graph, activity, dashboard и AI retrieval.
- [ ] Добавить authorization matrix + metadata leak regression tests.
- [ ] Запретить post-filter-after-LIMIT architecture test.

## P0.4 — Domain/Project UX

- [ ] Sidebar: Home / My Work / Domains / Pinned / Recent.
- [ ] Domain home: Projects, recent docs, reviews, members для managers.
- [ ] Project home: document tree, changes/reviews, activity, files, access state.
- [ ] Создание документа из Project не спрашивает Domain/Project повторно.
- [ ] Domain access dialog: searchable multi-select users + role preset + optional expiration.
- [ ] Project access: default `Inherited from Domain`, one explicit action to make restricted.
- [ ] Показывать effective access как `direct` / `inherited from ...` через SecureAccess `ExplainAccess`.
- [ ] Добавить user-centric access view: все Domains/Projects конкретного пользователя.

## P0.5 — Измеримый поиск и retrieval

- [ ] Ввести единый `SearchService`.
- [ ] ACL-aware FTS5, weighted ranking, Domain/Project filters, snippets/highlights и стабильную pagination.
- [ ] Добавить versioned relevance dataset и MRR/nDCG/Recall regression metrics.
- [ ] Заменить фиктивный AI chunk score на реальный retrieval rank.
- [ ] Делить Markdown на chunks по AST/headings с source coordinates.
- [ ] Добавлять semantic/hybrid retrieval через RRF только как optional backend и только после доказанного выигрыша.

---

# P1 — редактор и безопасное редактирование

## P1.1 — реальный Markdown Live Editor

- [ ] Заменить production `<textarea>` editing surface на CodeMirror 6.
- [ ] Сохранить Markdown как canonical storage format.
- [ ] Default mode: Obsidian-style Live Preview.
- [ ] Source mode и Split mode — для power users.
- [ ] Slash command palette `/`.
- [ ] Selection bubble toolbar.
- [ ] ACL-aware `[[wiki-link]]` autocomplete.
- [ ] Drag/drop/paste attachments с progress/retry.
- [ ] Contextual outline и diagnostics.
- [ ] Properties вынести в side sheet вместо постоянно раскрытой панели.
- [ ] Build-time frontend toolchain допускается, runtime остаётся single Go binary with embedded assets.

## P1.2 — autosave, recovery и conflicts

- [ ] Единый state machine `clean → dirty → saving → saved/offline/conflict`.
- [ ] Local recovery journal до подтверждённого server save.
- [ ] Sequential saves и request cancellation/coalescing.
- [ ] Conflict UI: Compare / Keep as change / Reload latest.
- [ ] Crash/refresh/offline regression tests.
- [ ] Никакого silent overwrite.

## P1.3 — Change Requests и Review

- [ ] Новый Draft можно редактировать напрямую.
- [ ] Edit опубликованного документа создаёт Change Request.
- [ ] Published revision не изменяется до merge.
- [ ] Views: Editor / Changes / Preview / Overview.
- [ ] Rendered block diff как default, source diff как advanced.
- [ ] Reviewers, comments, Approve, Request changes, Merge/Publish.
- [ ] Централизованный `/reviews` и `My Work`.

## P1.4 — Mobile + accessibility

- [ ] Single-pane mobile editor.
- [ ] Properties/slash palette как bottom sheet.
- [ ] Keyboard-safe sticky actions + safe-area support.
- [ ] Touch targets ≥44 CSS px.
- [ ] WCAG 2.2 AA blocking gate.
- [ ] Full keyboard editor/access management navigation.
- [ ] Zero horizontal page overflow across tested device matrix.

---

# P2 — эксплуатационная надёжность и governance

## P2.1 — согласованный backup/restore

- [ ] Backup manifest включает Docs SQLite + SecureAccess Axiom/Pebble + uploads + checksums + versions.
- [ ] Automated restore drill проверяет documents, Domain/Project bindings, memberships и revocation.
- [ ] Orphan security workspace reconciler.
- [ ] Fail-closed behaviour при недоступности security store.

## P2.2 — Observability

- [ ] Request/DB/search/security/backup metrics.
- [ ] `/readyz` отдельно от liveness.
- [ ] Optional OpenTelemetry export.
- [ ] Access denials без утечки sensitive metadata.
- [ ] SLO определить после измеренного baseline.

## P2.3 — CI, supply chain и toolchain

- [ ] Сделать `govulncheck` blocking.
- [ ] Добавить targeted Go fuzzing и static analysis.
- [ ] Pin GitHub Actions по SHA и ограничить permissions.
- [ ] Dependabot/Renovate + OpenSSF Scorecard.
- [ ] SBOM + provenance/signing policy для release artifacts.
- [ ] Обновить Docs_Hub Go 1.25 → поддерживаемую SecureAccess линию Go 1.27 отдельным проверяемым PR.

## P2.4 — качество знаний

- [ ] Content owner, `review_due_at`, stale/expired states.
- [ ] Dashboard: stale/no-owner/broken-links/orphan documents.
- [ ] Global / Domain / Project templates.
- [ ] Diátaxis presets: tutorial / how-to / reference / explanation.
- [ ] Feedback loop и privacy-safe search quality telemetry.

## P2.5 — Teams/Groups

- [ ] После стабильного user-level Domain access расширить SecureAccess Teams/Groups.
- [ ] Domain/Project access можно выдавать команде одним membership.
- [ ] UI остаётся тем же: user/team picker + role preset.

---

# Отложено до появления измеренного требования

- [ ] CRDT real-time collaborative editing.
- [ ] Обязательный PostgreSQL/Redis deployment profile.
- [ ] Обязательный OpenSearch/vector database.
- [ ] Переход на микросервисы.
- [ ] Полный SPA rewrite.
- [ ] Per-block ACL.
- [ ] Глубокая вложенность Domains/Projects.
- [ ] Новый косметический redesign без подтверждённой UX-проблемы.

---

# Главные KPI roadmap

- production auth/authorization authorities кроме SecureAccess: **0**;
- authorization bypass / metadata leak paths: **0**;
- documents without Domain/Project after migration: **0**;
- orphan active security bindings after reconciliation: **0**;
- unauthorized search/suggest/graph/backlink results: **0**;
- post-filter-after-LIMIT sensitive queries: **0**;
- silent editor overwrite/data-loss regression cases: **0**;
- WCAG blocking violations: **0**;
- verified content + security backup restore: **100% test drills**;
- search quality: MRR@10 / nDCG@10 / Recall@20 + p95 latency;
- direct SQL in HTTP handlers after convergence: **0**;
- silently passing vulnerability findings in CI: **0**.
