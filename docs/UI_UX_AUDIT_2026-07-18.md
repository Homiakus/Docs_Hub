# Docs_Hub UI/UX audit and redesign

Date: 2026-07-18  
Baseline commit: `8cc963577f60afaefa8e3b1a9b2b4d0a883c8692`

## Scope

The audit covered the complete authenticated and unauthenticated interface, document lifecycle, search, permissions, autosave, attachments, PDF, knowledge graph, administration, responsive behavior, accessibility and CI quality gates.

Reviewed surfaces:

- application shell and authentication;
- dashboard, spaces and search;
- reader, editor, templates and version history;
- autosave and optimistic locking;
- workflow and document visibility;
- media/PDF upload and serving;
- knowledge graph;
- administration, backup and Obsidian import;
- Go integration tests and Playwright configuration.

## Baseline findings

### P0 — broken visual foundation

`static/style.css` imported modules from paths that did not exist. The browser requested `/static/tokens.css`, `/static/typography.css` and `/static/components/*`, while the actual files were stored under `/static/css/`. As a result, semantic tokens and component styles were absent at runtime.

Static analysis of the baseline templates found 95 class names in use, 83 without effective runtime rules. Even after correcting import paths, 70 classes still had no definition. The templates also contained 77 inline style declarations, so pages had no coherent responsive design system.

### P0 — autosave data integrity

The client ignored the ID returned after creating a draft. Every later autosave from `/new` was therefore sent with `id=0`, producing duplicate documents. The hidden form ID and lock version were not updated. The server ignored database errors, hard-coded organization/space `1`, did not check document-level edit rights and incremented the lock version only in the JSON response, not in the database.

### P0 — lifecycle and permissions

Workflow services existed but had no routes or controls. New manual saves used the migration default `published`, while autosave used `draft`. Readers could receive non-published authenticated documents through list/search-related surfaces because visibility checks did not include workflow status.

### P1 — incomplete primary scenarios

- Search rendered space/status controls but ignored both parameters.
- The editor rendered a space selector but the save handler ignored it.
- The PDF page was an empty iframe with a hard-coded ten-page counter; PDF upload was rejected.
- The graph was rendered twice by two independent scripts and used a circular layout with straight, unlabeled, non-directional edges.
- Template definitions were not connected to the editor.
- Admin markup used removed legacy classes, while its data-grid script targeted selectors that did not exist.

### P1 — accessibility and responsive behavior

The command palette and table builder lacked a modal dialog contract, focus containment, focus restoration, listbox/combobox semantics and arrow-key behavior. Mobile navigation remained focusable while visually off-canvas. There was no reliable responsive or accessibility gate in CI.

### P1 — quality gates

The Playwright suite had no authenticated setup or deterministic data lifecycle, allowed up to ten axe violations, contained a search test without an assertion and referenced visual snapshots that were not present. CI ran Go and Docker checks only.

## Redesign outcome

| Area | Implemented change |
|---|---|
| Design system | Corrected module paths, rebuilt semantic tokens, removed template inline styles and added responsive layouts for all pages. |
| Shell | New sidebar/topbar, mobile drawer with `inert`, skip link, theme persistence and compact global search. |
| Dashboard | Role-aware metrics, draft continuation, review queue, recent documents, templates and graph entry point. |
| Search | Working query/space/status filters with preserved control state and ACL/workflow-aware results. |
| Reader | New reading layout, progress indicator, responsive TOC, metadata, document-specific edit action, workflow controls and version history. |
| Editor | Split Markdown/preview canvas, templates, properties, word count, media/PDF dropzone and accessible formatting controls. |
| Autosave | Server ID adoption, URL replacement, hidden state synchronization, local recovery, sequential saves, explicit conflict UI and atomic lock checks. |
| Workflow | Validated `draft → in_review → approved → published → archived` transitions with role checks, optimistic lock and audit event. |
| Search index | Rebuilt the original contentless FTS5 table as an updateable index and synchronized derived tags, links, files and FTS in one transaction. |
| PDF | Accepted/stored PDF files, associated them with article ACL, counted pages, generated viewer links and embedded the real file. |
| Graph | Single renderer, layered/chunked layout, orthogonal directional edges, labels, pan/zoom/fit, status filter, search, legend and accessible list. |
| Admin | Rebuilt users, categories, content, ACL, backups and import as responsive sections with filtering and destructive-action confirmation. |
| Accessibility | Modal semantics, focus restoration/trapping, combobox/listbox keyboard model, live regions, reduced-motion and print modes. |
| CI | Added authenticated functional Playwright scenarios at 1440, 768 and 390 px plus axe blocking checks and failure artifacts. |

## Verification strategy

The redesign is protected by:

- Go unit/integration and security tests;
- autosave ID/lock/conflict tests;
- search space/status filter tests;
- atomic workflow transition tests;
- PDF upload/viewer tests;
- reader draft-leak regression tests;
- JavaScript syntax checks;
- Playwright functional, responsive and accessibility checks in GitHub Actions.

## Remaining enterprise work

The embedded PDF viewer is functional and access-controlled, but OCR, per-page extraction and PDF full-text search remain separate roadmap work. OIDC, PostgreSQL/Redis and distributed storage are also outside this UI redesign and remain tracked in the enterprise roadmap.
