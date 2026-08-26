# Product Roadmap — Docs Hub Next

**Актуализировано:** 2026-08-26  
**Docs_Hub baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`  
**SecureAcces baseline:** `main@827abb1add11a9fcbd0a9944e65efbd20c675739`  
**AutoTraceLab baseline:** `main@69b4ebebe3b96295c8e39679130fa946669c5f7b`

Главные документы:

- **[Pareto Implementation Plan](PARETO_IMPLEMENTATION_PLAN_2026-08-26.md)** — архитектурная конвергенция, search/retrieval, reliability и engineering quality.
- **[SecureAccess + Domains + Projects + Editor Master Plan](SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md)** — целевая продуктовая модель, security boundary, информационная архитектура и новый редактор.
- **[Anchored Comments + Markdown Presentations + AutoTrace Plan](COLLAB_PRESENTATIONS_AUTOTRACE_PLAN_2026-08-26.md)** — произвольные комментарии к тексту, presentation-mode одного Markdown source и сложные технические схемы через AutoTrace.

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
- [x] AutoTraceLab уже имеет importable Go Core с versioned `ContractVersion`, Block/Port/Edge model, routing options, validation, metrics и incremental Scene architecture.

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

# P1 — редактор, совместная работа и представления документа

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

## P1.3 — Anchored Comments в произвольном месте текста

- [ ] Selection toolbar action `Comment` и shortcut `Ctrl/Cmd+Alt+M`.
- [ ] Разрешить comments в Editor и Reader без перехода в edit mode.
- [ ] Thread/replies/mentions/resolve/reopen/deep links.
- [ ] Хранить comments отдельно от Markdown source.
- [ ] Anchor = revision + TextPosition + TextQuote (`exact/prefix/suffix`) + AST/block identity.
- [ ] Re-anchor cascade: revision diff mapping → quote/context → AST/block matching → explicit orphan state.
- [ ] Запретить silent re-anchor при неоднозначности.
- [ ] Поддержать comments внутри Change Request и перенос anchors при merge.
- [ ] Добавить optional `comment / suggestion / blocking` thread kinds.
- [ ] Comment metadata и notifications наследуют document ACL; `PermComment` проверяется SecureAccess.
- [ ] Добавить blocking re-anchor/security regression corpus.

## P1.4 — Change Requests и Review

- [ ] Новый Draft можно редактировать напрямую.
- [ ] Edit опубликованного документа создаёт Change Request.
- [ ] Published revision не изменяется до merge.
- [ ] Views: Editor / Changes / Preview / Comments / Overview.
- [ ] Rendered block diff как default, source diff как advanced.
- [ ] Reviewers, anchored comments, Approve, Request changes, Merge/Publish.
- [ ] Опциональный gate: unresolved blocking comments не позволяют Approve.
- [ ] Централизованный `/reviews` и `My Work`.

## P1.5 — Markdown Presentation View

- [ ] Один `.md` source используется одновременно для Reader/Editor/Presentation/Print.
- [ ] Versioned safe presentation front matter: theme, ratio, transition, paginate, headingDivider.
- [ ] Поддержать explicit slide separator `---` вне front matter/fenced blocks.
- [ ] Поддержать heading-based automatic slide splitting.
- [ ] Speaker notes через безопасный `:::notes` directive.
- [ ] Whitelist fragments/slide directives без arbitrary JS/CSS.
- [ ] Pipeline: Goldmark/Docs AST → Slide AST → sanitized HTML → embedded Reveal.js runtime.
- [ ] Modes: Present here / Fullscreen / Presenter / Print-PDF.
- [ ] Editor tab `Slides` + thumbnails + slide diagnostics.
- [ ] Slash commands `/new slide`, `/speaker notes`, `/fragment`, `/diagram slide`.
- [ ] Presentation URLs и presenter/print endpoints имеют тот же ACL, что document.
- [ ] Audience DOM по умолчанию не содержит collaborative comments/speaker notes.

## P1.6 — AutoTrace для сложных технических схем

- [ ] Mermaid оставить для простых flow/sequence diagrams; AutoTrace — для больших block/port technical diagrams.
- [ ] Добавить fenced Markdown block ` ```autotrace ` с versioned Docs AutoTrace DSL.
- [ ] Автор не обязан задавать raw `x/y/width/height`; Docs adapter выполняет initial layout.
- [ ] Добавить `DiagramEngine` boundary и отдельный `internal/diagram/autotrace` adapter.
- [ ] Использовать importable Go Core `github.com/Homiakus/autotraceLab/go_engine/core`, не AutoTrace React iframe.
- [ ] Перед production pin выпустить/использовать semver Go module и зафиксированный `ContractVersion`.
- [ ] Добавить compatibility tests Docs_Hub ↔ AutoTrace.
- [ ] Желательный AutoTrace vNext contract: validated `LayoutAndRoute(SceneSpec)` либо официальный documented layout-before-route contract.
- [ ] Strict DSL parser с line/column diagnostics и hard complexity limits.
- [ ] Server produces validated scene; browser renders responsive sanitized SVG.
- [ ] Viewer: fit, zoom/pan, fullscreen, node/edge labels, static print mode.
- [ ] Content-addressed render cache учитывает source + engine version + adapter/theme geometry version.
- [ ] Invalid/unroutable diagram показывает diagnostic/fail-safe state, а не правдоподобную неправильную схему.
- [ ] AutoTrace block работает одинаково в Reader, Editor Preview, Presentation и Print/PDF.
- [ ] Slide mode fit-ит logical diagram в safe area без topology reroute на каждый resize.
- [ ] Diagram source/labels не отправляются во внешние API и не попадают в public telemetry.

## P1.7 — Mobile + accessibility

- [ ] Single-pane mobile editor.
- [ ] Properties/slash/comments palette как bottom sheet.
- [ ] Keyboard-safe sticky actions + safe-area support.
- [ ] Touch targets ≥44 CSS px.
- [ ] Presentation mobile landscape и touch navigation.
- [ ] AutoTrace viewer gestures не блокируют page/slide navigation без явного focus mode.
- [ ] WCAG 2.2 AA blocking gate.
- [ ] Full keyboard editor/access/comments/presentation navigation.
- [ ] Zero horizontal page overflow across tested device matrix.

---

# P2 — эксплуатационная надёжность и governance

## P2.1 — согласованный backup/restore

- [ ] Backup manifest включает Docs SQLite + SecureAccess Axiom/Pebble + uploads + checksums + versions.
- [ ] Automated restore drill проверяет documents, Domain/Project bindings, memberships, comments/anchors и revocation.
- [ ] Rebuildable presentation/AutoTrace caches не являются единственной копией пользовательского source.
- [ ] Orphan security workspace reconciler.
- [ ] Fail-closed behaviour при недоступности security store.

## P2.2 — Observability

- [ ] Request/DB/search/security/backup metrics.
- [ ] Comments re-anchor success/orphan metrics без comment text.
- [ ] Presentation render/validation error metrics без document content.
- [ ] AutoTrace parse/layout/route/render latency, cache hit ratio и bounded-failure metrics.
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
- [ ] Pin Reveal/static presentation runtime и AutoTrace module versions.

## P2.4 — качество знаний

- [ ] Content owner, `review_due_at`, stale/expired states.
- [ ] Dashboard: stale/no-owner/broken-links/orphan documents.
- [ ] Global / Domain / Project templates.
- [ ] Diátaxis presets: tutorial / how-to / reference / explanation.
- [ ] Presentation-ready templates для architecture/review/training docs.
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
- [ ] Pixel-coordinate comments как основной anchor mechanism.
- [ ] Отдельная editable presentation copy / PPTX как source of truth.
- [ ] AutoTrace animation/routing choreography как prerequisite первой версии.
- [ ] Глубокая вложенность Domains/Projects.
- [ ] Новый косметический redesign без подтверждённой UX-проблемы.

---

# Главные KPI roadmap

- production auth/authorization authorities кроме SecureAccess: **0**;
- authorization bypass / metadata leak paths: **0**;
- documents without Domain/Project after migration: **0**;
- orphan active security bindings after reconciliation: **0**;
- unauthorized search/suggest/graph/backlink/comment results: **0**;
- post-filter-after-LIMIT sensitive queries: **0**;
- comment threads silently attached to ambiguous/wrong text in regression corpus: **0**;
- presentation copies diverging from canonical Markdown: **0**;
- presentation routes bypassing document ACL: **0**;
- invalid AutoTrace routes rendered as valid technical diagrams: **0**;
- unbounded AutoTrace render requests: **0**;
- silent editor overwrite/data-loss regression cases: **0**;
- WCAG blocking violations: **0**;
- verified content + security + comments backup restore: **100% test drills**;
- search quality: MRR@10 / nDCG@10 / Recall@20 + p95 latency;
- direct SQL in HTTP handlers after convergence: **0**;
- silently passing vulnerability findings in CI: **0**.
