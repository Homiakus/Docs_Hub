# Product Roadmap — Docs Hub Next

**Актуализировано:** 2026-08-26  
**Baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`

Подробный порядок работ, acceptance criteria, тесты и обоснование приоритетов находятся в **[Pareto Implementation Plan](PARETO_IMPLEMENTATION_PLAN_2026-08-26.md)**.

## Что уже является сильной базой

- [x] Go single-binary application и SQLite WAL persistence.
- [x] FTS5/BM25, tags, wiki-links, backlinks и knowledge graph.
- [x] Revision/workflow schema и optimistic locking/autosave UX.
- [x] Responsive editorial UI, mobile hardening и WCAG-oriented interaction model.
- [x] PDF.js viewer и managed attachments baseline.
- [x] Playwright E2E matrix для Chromium, Firefox и WebKit.
- [x] Docker/native deployment tooling, healthcheck, CI и release workflow.
- [x] Начаты domain/application/authn/authz/repository слои.

## Следующий релизный цикл — Pareto P0

### P0.1 — Один исполняемый архитектурный путь

- [ ] Добавить composition root `internal/bootstrap`.
- [ ] Реально подключить `repository`, `application`, `authn` и `authz` к production HTTP path.
- [ ] Мигрировать handlers вертикальными slices и удалить прямой SQL/business policy из `httpapp`.
- [ ] Удалить дубли доменных моделей, session/workflow/authorization implementations.
- [ ] Защитить рефакторинг characterization и architecture tests.

### P0.2 — Authorization как query invariant

- [ ] Зафиксировать policy precedence ADR: deny-by-default, scoped roles, document permissions, workflow visibility.
- [ ] Ввести `AccessScope` и одну production policy implementation.
- [ ] Применять ACL до ranking, `LIMIT` и aggregation.
- [ ] Перевести search/suggest/spaces/graph/backlinks/activity/files на access-scoped queries.
- [ ] Добавить authorization matrix + leak regression tests.
- [ ] Добавить индексы только после `EXPLAIN QUERY PLAN`/benchmark evidence.

### P0.3 — Измеримый поиск и retrieval

- [ ] Ввести единый `SearchService`.
- [ ] ACL-aware FTS5, weighted ranking, filters, snippets/highlights и стабильную pagination.
- [ ] Добавить versioned relevance dataset и MRR/nDCG/Recall regression metrics.
- [ ] Заменить фиктивный AI chunk score на реальный retrieval rank.
- [ ] Делить Markdown на chunks по AST/headings с source coordinates.
- [ ] Добавлять semantic/hybrid retrieval через RRF только как optional backend и только после доказанного выигрыша.

## Следом — Pareto P1

### P1.1 — Sessions и OIDC

- [ ] Сделать `authn.SessionManager` единственной session implementation.
- [ ] Idle + absolute expiry, rotation, cookie/cache hardening, revoke semantics.
- [ ] Завершить OIDC: discovery/JWKS, Code+PKCE, state/nonce, least-privilege provisioning, group→role mapping.
- [ ] SCIM 2.0 добавить после появления реального enterprise provisioning use case.

### P1.2 — Надёжность и observability

- [ ] Migration checksum/application-version policy.
- [ ] Backup manifest + verification + automated restore test.
- [ ] SQLite concurrency/WAL benchmark перед изменением connection pool.
- [ ] Request/DB/search/backup metrics, `/readyz`, optional OpenTelemetry.
- [ ] SLO определить по измеренному baseline.

### P1.3 — CI, supply chain и toolchain

- [ ] Сделать `govulncheck` blocking.
- [ ] Добавить targeted Go fuzzing и static analysis.
- [ ] Pin GitHub Actions по SHA и ограничить permissions.
- [ ] Dependabot/Renovate + OpenSSF Scorecard.
- [ ] SBOM + provenance/signing policy для release artifacts.
- [ ] Обновить Go 1.25 → Go 1.27 отдельным проверяемым PR.

## P2 — качество и жизненный цикл знаний

- [ ] Content owner, `review_due_at`, stale/expired states.
- [ ] Dashboard: stale/no-owner/broken-links/orphan documents.
- [ ] Diátaxis templates: tutorial / how-to / reference / explanation.
- [ ] Feedback loop и privacy-safe search quality telemetry.

## Отложено до появления измеренного требования

Следующие задачи не являются приоритетом текущего цикла:

- [ ] CRDT real-time collaborative editing.
- [ ] Обязательный PostgreSQL/Redis deployment profile.
- [ ] Обязательный OpenSearch/vector database.
- [ ] Переход на микросервисы.
- [ ] Полный SPA rewrite.
- [ ] Новый косметический redesign без подтверждённой UX-проблемы.

Они возвращаются в активный roadmap только если benchmark, SLA, user research или enterprise requirement показывают, что текущая архитектура достигла реального ограничения.

## Главные KPI roadmap

- authorization bypass / metadata leak paths: **0**;
- direct SQL in HTTP handlers после convergence: **0**;
- duplicate production implementations: **0**;
- unauthorized search results: **0**;
- search quality: MRR@10 / nDCG@10 / Recall@20 + p95 latency;
- verified backup/restore success: **100% на тестовых restore drills**;
- unreviewed vulnerability findings, silently passing CI: **0**;
- release binary version/tag mismatch: **0**.
