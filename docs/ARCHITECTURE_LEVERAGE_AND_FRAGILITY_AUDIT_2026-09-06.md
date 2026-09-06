# Docs_Hub — Architecture Leverage & Fragility Audit

**Status:** active remediation addendum  
**Date:** 2026-09-06  
**Baseline:** `main@22efb8ac5cdc458a6ad137fc7b5f85bc1bd8c8dd`  
**Scope:** architectural leverage, fragility, authorization boundaries, repository contracts, transaction ownership, sessions, migrations, SQLite limits, CI architecture gates.

> This document is a mandatory addendum to `docs/DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md`. It does not create a second product roadmap. Existing `AUD-001..AUD-008` remain valid; this audit adds newly confirmed leverage points and fragility items that must be folded into the same remediation order and evidence discipline.

---

## 1. Executive conclusion

Docs_Hub does **not** need a wholesale architecture rewrite. The chosen direction — `HTTP -> Application -> Ports -> Repository/Adapters` — is correct. The highest-return work is to eliminate the remaining alternate execution paths that bypass that direction.

The dominant architectural risk is **path multiplicity**: the same resource can currently be reached or authorized through several mechanisms (handler checks, legacy authorizer, workspace adapter, repository SQL predicates, direct SQL). As features grow, correctness becomes dependent on developer discipline instead of structural guarantees.

The target state is defined by three non-negotiable invariants:

1. Every user operation carries a trusted `Principal + OrganizationScope`.
2. Every resource operation passes through one authorization path before data is exposed or mutated.
3. HTTP handlers do not know how persistence is implemented and never execute SQL directly.

If these three invariants become mechanically enforced, the cost and risk of future Domains/Projects, collaboration, workflow, audit, search and enterprise features drops sharply.

---

## 2. Pareto map — where effort has the highest return

| Priority | Leverage point | ROI | Main effect |
|---|---|---:|---|
| P0 | Single security / tenant scope boundary | 10/10 | Removes entire classes of cross-org, ACL and IDOR failures |
| P0 | Resource-level authorization for every mutation | 10/10 | Closes concrete unsafe paths now present in the codebase |
| P1 | Scope-safe repository contracts | 9/10 | Makes accidental data leaks structurally harder |
| P1 | Vertical extraction from `httpapp.Server` | 9/10 | Reduces nonlinear change cost without a big-bang rewrite |
| P1 | Application-owned transaction boundaries | 8/10 | Makes save/version/index/audit operations atomic |
| P1 | Persistent authoritative access state | 8/10 | Removes restart/multi-instance divergence |
| P2 | Architecture/security CI gates | 7/10 | Prevents the old architecture from growing back |
| P2 | Migration checksums and schema invariants | 6/10 | Prevents silent schema drift between installations |
| P2 | Explicit SQLite operating envelope | 6/10 | Avoids premature migration while making scale limits measurable |

---

## 3. Newly confirmed architectural fragility

### AUD-009 — unscoped repository getters — P0/P1

**Files:**
- `internal/repository/interfaces.go`
- `internal/repository/sqlite/article_repo.go`

The repository contract exposes unrestricted getters such as:

```go
GetBySlug(ctx context.Context, slug string)
GetByID(ctx context.Context, id int64)
```

while list/search paths partially apply user-aware predicates.

This produces an unsafe asymmetry: one method embeds authorization filtering, another returns the resource without any principal, organization or scope. A caller can therefore accidentally bypass access rules simply by selecting a different repository method.

**Failure mode:** authorization becomes caller discipline rather than repository/API structure.

**Target:** normal user-facing repository operations require an explicit read/write scope.

Preferred direction:

```go
type ReadScope struct {
    PrincipalID    int64
    OrganizationID int64
    WorkspaceIDs   []string
}

GetVisibleByID(ctx context.Context, scope ReadScope, id int64) (*domain.Article, error)
GetEditableByID(ctx context.Context, scope WriteScope, id int64) (*domain.Article, error)
```

If unrestricted loading is genuinely required for migration/system jobs, it must live behind a deliberately named internal/system interface such as `GetByIDSystem`, not the default repository contract.

**Acceptance criteria:**
- [ ] no user-facing application service loads a document through an unscoped repository getter;
- [ ] system-only getters are isolated and explicitly named;
- [ ] cross-user, cross-project and cross-organization negative tests prove denial;
- [ ] architecture test prevents new unscoped getters from entering the user-facing repository port.

---

### AUD-010 — activity query accepts a user but does not apply user scope — P0/P1

**File:** `internal/repository/sqlite/article_repo.go`  
**Function:** `ListRecentActivity`

The function accepts `u *domain.User`, but the SQL reads recent article audit events globally and does not apply the article authorization predicate or workspace scope before `ORDER BY ... LIMIT 12`.

This is exactly the class of metadata leak already prohibited by the master-plan rule **Authorization before ranking/aggregation/LIMIT**.

**Failure mode:** titles/slugs/metadata of inaccessible documents may appear in activity feeds even if document body access is denied.

**Acceptance criteria:**
- [ ] activity query receives an explicit access scope, not a decorative user parameter;
- [ ] inaccessible resources are filtered before ordering/limit;
- [ ] regression test creates visible and invisible activity and proves only visible activity is returned;
- [ ] same rule is applied to graph, backlinks, search suggestions, AI retrieval and counters.

---

### AUD-011 — `httpapp.Server` remains a gravity well — P1

**File:** `internal/httpapp/server.go`

`Server` currently depends directly on database, application services, repositories, sessions, authorizer, Telegram integration and transport/view models. The package imports both high-level application concepts and low-level persistence components.

The problem is not file size by itself. The problem is dependency direction:

```text
HTTP -> DB
HTTP -> Repository
HTTP -> Application
HTTP -> Authorization
```

instead of:

```text
HTTP -> Application
Application -> Ports
Adapters -> Ports
```

**Remediation strategy:** strangler refactoring, not rewrite.

Extract complete vertical use cases in this order:

1. `CommentService`
2. `DocumentService` / save + autosave
3. `RevisionService`
4. `SessionService`
5. `SearchService`
6. remaining workflow/admin operations

Each extraction is complete only when the HTTP handler becomes transport-only: parse input, obtain principal, call service, translate result.

**Acceptance criteria:**
- [ ] no new business logic is added directly to `server.go`;
- [ ] extracted handlers do not import `database/sql` or concrete SQLite repositories;
- [ ] direct `s.db.*` usage inside `internal/httpapp` monotonically decreases to zero;
- [ ] architecture test blocks regression.

---

### AUD-012 — direct SQL remains inside transport handlers — P1

**File:** `internal/httpapp/comment_handlers.go`  
**Function:** `dbArticleByID`

The comment HTTP layer still loads an article using `s.db.QueryRowContext` directly. This bypasses the intended application/repository boundary and makes it easy for authorization and transaction rules to diverge from the rest of the system.

**Target invariant:**

```text
internal/httpapp/** MUST NOT use
- database/sql
- QueryContext / QueryRowContext
- ExecContext
- BeginTx
```

A temporary allowlist is acceptable only while the migration is actively shrinking and must be tracked as debt with an expiry condition.

**Acceptance criteria:**
- [ ] `dbArticleByID` is removed from HTTP layer;
- [ ] comment handlers call `CommentService` only;
- [ ] CI contains a structural test/check that fails on new direct SQL in transport packages.

---

### AUD-013 — transaction ownership is too low-level — P1

Document editing increasingly represents a multi-step invariant, not a single repository call:

```text
validate
-> authorize
-> mutate document
-> create immutable revision
-> update links/index
-> write audit event
-> commit
```

If each step controls its own persistence independently, failures can leave partially updated state.

**Target:** application service owns the transaction/use-case boundary through a Unit of Work / transaction port.

Example direction:

```go
type UnitOfWork interface {
    WithinTx(ctx context.Context, fn func(Ports) error) error
}
```

The exact API may differ, but transaction scope must follow the business invariant, not the table/repository boundary.

Apply first to:
- autosave/document save;
- revision creation;
- workflow transitions;
- comment resolve/delete when audit/notification state is involved;
- ACL/workspace binding mutations;
- session/security state changes requiring audit coupling.

**Acceptance criteria:** fault-injection tests prove all-or-nothing behavior at each multi-write use case.

---

### AUD-014 — migration identity has no checksum — P2

**File:** `internal/db/db.go`

`schema_migrations` records migration file name/version and `applied_at`, but not the content digest. Applied migrations are skipped by name.

If an old migration file is edited, an existing installation and a fresh installation can silently converge on different schemas while both report the same migration version.

This risk is especially important because the project already follows a forward-only migration policy.

**Target:** migration identity = filename/version + immutable content checksum.

Suggested schema evolution:

```text
schema_migrations
- version
- sha256
- applied_at
```

At startup:

```text
known version + different checksum -> hard failure with actionable diagnostic
```

**Acceptance criteria:**
- [ ] checksums are recorded for new migrations;
- [ ] historical migrations receive a deterministic baseline strategy without rewriting applied SQL;
- [ ] checksum mismatch fails closed;
- [ ] CI verifies migration immutability and fresh-schema equivalence.

---

### AUD-015 — SQLite concurrency limit is implicit, not an engineering contract — P2

**File:** `internal/db/db.go`

SQLite is opened with WAL, foreign keys and busy timeout, which is appropriate for the single-binary deployment model. However, `SetMaxOpenConns(1)` effectively serializes database access through one connection.

This is not automatically a defect. It becomes architectural fragility when the operating envelope is undefined and growth decisions are made without measurement.

**Do not migrate to PostgreSQL merely because this limit exists.** First define and benchmark the intended envelope.

Required engineering contract:

```text
Deployment mode: single node / embedded SQLite
Target concurrent users: <measured>
Target document count: <measured>
Target read p95: <measured>
Target save/autosave p95: <measured>
Allowed busy/lock error rate: 0 or explicit threshold
```

Create a reproducible contention benchmark covering concurrent reads, autosave, search and audit writes. PostgreSQL becomes a justified decision only when measured requirements exceed this envelope.

---

## 4. Existing P0 confirmed again by this audit

The following existing master-plan items remain the first implementation priority and are reinforced by this review:

- `AUD-002`: workspace permission checks do not prove organization membership.
- `AUD-003`: project access mode is partly process-local and therefore not an authoritative boundary.
- `AUD-004`: comment resolve is an IDOR-class mutation because it does not establish `comment -> document -> workspace -> permission` before update.
- `AUD-006`: session idle timeout is declared but not actually enforced; CSPRNG error is ignored.
- `AUD-007`: `main` is currently unprotected, so architecture/security guarantees depend on discipline rather than repository governance.

These must not be downgraded merely because new work is added to the plan.

---

## 5. Unified target security path

The current architecture contains several partially overlapping authorization mechanisms. The desired end state is one auditable path:

```text
HTTP / Bot / future API
        |
        v
Trusted Principal Resolver
        |
        v
Application Use Case
        |
        +--> Policy / Scope Resolver (single authority)
        |
        v
Scoped Repository Ports
        |
        v
SQLite / future adapter
```

Rules:

1. A handler may not invent organization/workspace scope.
2. A repository may not widen scope.
3. A cache may not be authoritative and stale state may never widen access.
4. Filtering occurs before ranking, aggregation, graph expansion or `LIMIT`.
5. Every write checks resource-level permission, not merely authentication.
6. System/background access uses a separately typed/named capability and is auditable.

---

## 6. FMEA-style risk register

| ID | Failure mode | Effect | S | O | D | RPN | Required control |
|---|---|---|---:|---:|---:|---:|---|
| R-SEC-01 | Unscoped repository getter used in user path | Cross-tenant/private data disclosure | 10 | 6 | 7 | 420 | Scoped repository contracts + negative tests |
| R-SEC-02 | Mutation checks authn but not resource authz | IDOR / unauthorized state change | 10 | 5 | 7 | 350 | Resource chain authorization inside application service |
| R-SEC-03 | Multiple policy sources disagree | Access expansion or inconsistent deny/allow | 10 | 6 | 8 | 480 | Single security authority + explain-access tests |
| R-SEC-04 | Activity/search filtered after LIMIT | Metadata leak and incorrect result set | 8 | 6 | 7 | 336 | Query-safe scope before ranking/limit |
| R-STATE-01 | Access mode kept in process memory | Restart/multi-instance permission divergence | 9 | 5 | 6 | 270 | Persistent authority; cache only fail-closed |
| R-DATA-01 | Multi-write use case lacks app transaction | Partial document/revision/index/audit state | 8 | 5 | 6 | 240 | Unit-of-Work + fault injection |
| R-SCHEMA-01 | Applied migration edited without detection | Installation-specific schema drift | 8 | 4 | 9 | 288 | Migration checksum + CI immutability gate |
| R-ARCH-01 | HTTP continues direct SQL | Security/business rule duplication | 8 | 7 | 5 | 280 | Architecture gate + vertical service extraction |
| R-SCALE-01 | SQLite envelope unknown | Lock contention discovered late | 6 | 5 | 6 | 180 | Reproducible contention benchmark + SLO |

Scale: Severity (S), Occurrence (O), Detectability difficulty (D) from 1 to 10; `RPN = S * O * D`. Scores are prioritization aids, not substitutes for P0 security classification.

---

## 7. Atomic remediation queue

### P0 — close security ambiguity

- [ ] `LEV-P0-01` Introduce one trusted principal/organization scope type across user-facing use cases.
- [ ] `LEV-P0-02` Make organization membership part of effective authorization, not global role inference.
- [ ] `LEV-P0-03` Replace process-local project access authority with persistent/effective security policy.
- [ ] `LEV-P0-04` Fix comment resolve through `CommentService.Resolve(principal, commentID)` with document/workspace permission check.
- [ ] `LEV-P0-05` Audit every `POST/PUT/PATCH/DELETE` for resource-level authorization.
- [ ] `LEV-P0-06` Fix `ListRecentActivity` and every metadata/retrieval query to apply scope before `ORDER/LIMIT`.
- [ ] `LEV-P0-07` Enforce real session idle timeout and propagate `crypto/rand` failures.

### P1 — make the safe architecture the easiest architecture

- [ ] `LEV-P1-01` Replace default unscoped article getters in user-facing repository ports with scoped variants.
- [ ] `LEV-P1-02` Extract `CommentService`; remove direct comment SQL/article lookup from HTTP.
- [ ] `LEV-P1-03` Extract document save/autosave use case with application-owned transaction boundary.
- [ ] `LEV-P1-04` Continue vertical extraction until `httpapp` is transport-only.
- [ ] `LEV-P1-05` Add fault-injection tests for multi-write atomicity.

### P2 — lock the architecture in CI

- [ ] `LEV-P2-01` Add architecture check: no direct SQL from `internal/httpapp`.
- [ ] `LEV-P2-02` Add architecture check: user-facing repository ports cannot introduce unscoped resource getters.
- [ ] `LEV-P2-03` Add cross-user/cross-project/cross-org authorization matrix tests.
- [ ] `LEV-P2-04` Make vulnerability scanning blocking after current baseline is clean.
- [ ] `LEV-P2-05` Add migration checksums and immutable-migration CI verification.
- [ ] `LEV-P2-06` Define SQLite operating envelope and add contention benchmark/SLO evidence.

---

## 8. Definition of Done for each remediation item

A checkbox is not evidence. Each item becomes DONE only with:

```text
Status: DONE
Commit: <sha>
Evidence:
- targeted unit/integration/security test
- go test -race ./...
- architecture/static gate if applicable
- CI run id/url
Residual risks: none | explicit list
```

For authorization fixes, a **negative regression test is mandatory before or with the implementation**.

For structural refactors, success is measured not only by passing tests but by removal of the unsafe path. Example: adding `CommentService` while keeping direct SQL as an equally valid handler path does not close the item.

---

## 9. Architecture fitness functions to add

The following properties should be machine-checkable:

```text
FIT-001: internal/httpapp does not import database/sql.
FIT-002: internal/httpapp does not call Query/Exec/BeginTx directly.
FIT-003: every protected user resource query requires Principal/Scope.
FIT-004: every non-idempotent resource endpoint has negative authz coverage.
FIT-005: activity/search/graph/backlinks apply access scope before ranking/LIMIT.
FIT-006: migration content is immutable after application.
FIT-007: access-mode restart does not change effective permissions.
FIT-008: fault injection cannot leave save/revision/index/audit partially committed.
```

Prefer small Go architecture tests or static checks owned by the repository over prose-only conventions.

---

## 10. Explicit non-goals

Do **not** solve this audit by:

- rewriting Docs_Hub from scratch;
- introducing microservices solely to reduce file size;
- replacing SQLite before a benchmark proves the need;
- adding another independent authorization abstraction on top of existing ones;
- masking failing security/E2E tests;
- moving SQL into helper functions while keeping HTTP as the business-logic owner.

The goal is fewer architectural paths, not more layers.

---

## 11. Recommended implementation order

```text
1. Restore/confirm green trustworthy CI baseline.
2. Principal + organization membership boundary.
3. Resource-level mutation authorization, starting with comments.
4. Query-safe scope, including activity/search/graph/backlinks.
5. Scoped repository contracts.
6. CommentService -> DocumentSaveService vertical extraction.
7. Application transaction boundary + fault injection.
8. Architecture fitness functions in CI.
9. Migration checksum hardening.
10. SQLite operating envelope benchmark.
```

This ordering maximizes risk reduction per unit effort and prevents later features from being built on an ambiguous security/data-access foundation.
