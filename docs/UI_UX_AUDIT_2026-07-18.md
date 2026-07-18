# Docs_Hub UI/UX audit, redesign and mobile hardening

Date: 2026-07-18  
Baseline commit: `8cc963577f60afaefa8e3b1a9b2b4d0a883c8692`
Mobile follow-up baseline: `98b061855ceb309281c437f3b7b6cc477372fe06`

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

## Mobile follow-up findings

The redesigned shell was functionally responsive, but a second pass exposed a breakpoint cliff between 761 and 900 px. At iPad Mini width the desktop sidebar, two-pane editor and dense graph toolbar still competed for space. Several compact controls remained 25–40 px high, fixed viewport heights did not account for mobile browser chrome or safe areas, and the drawer did not fully isolate focus from the page.

The editor also needed a mobile-specific interaction model: showing Markdown and preview together made both panes too narrow, settings consumed the full scroll path, and save actions disappeared when the virtual keyboard was open. The graph captured touch gestures too aggressively and could block vertical page scrolling. The PDF route still delegated interaction to an iframe, so navigation, search, zoom and failure handling varied by browser. Finally, all three nominal Playwright device projects used Chromium, leaving WebKit and Firefox behavior unverified.

## Redesign outcome

| Area | Implemented change |
|---|---|
| Design system | Corrected module paths, rebuilt semantic tokens, removed template inline styles and added responsive layouts for all pages. |
| Shell | New sidebar/topbar, mobile drawer with `inert`, skip link, theme persistence and compact global search. |
| Dashboard | Role-aware metrics, draft continuation, review queue, recent documents, templates and graph entry point. |
| Search | Working query/space/status filters with preserved control state and ACL/workflow-aware results. |
| Reader | New reading layout, progress indicator, responsive TOC, metadata, document-specific edit action, workflow controls and version history. |
| Editor | Split desktop canvas plus a single-pane mobile mode, tabs for Markdown/preview, collapsible properties, touch-sized tools and a safe-area-aware sticky save bar. |
| Autosave | Server ID adoption, URL replacement, hidden state synchronization, local recovery, sequential saves, explicit conflict UI and atomic lock checks. |
| Workflow | Validated `draft → in_review → approved → published → archived` transitions with role checks, optimistic lock and audit event. |
| Search index | Rebuilt the original contentless FTS5 table as an updateable index and synchronized derived tags, links, files and FTS in one transaction. |
| PDF | Accepted/stored PDF files, associated them with article ACL and added a pinned PDF.js canvas viewer with page navigation, zoom, selectable text, local search and an explicit fallback. |
| Graph | Single renderer, layered/chunked layout, orthogonal directional edges, labels, mouse/touch/keyboard pan and zoom, pinch gesture, status filter, search, legend and accessible list. |
| Admin | Rebuilt users, categories, content, ACL, backups and import as responsive sections with filtering and destructive-action confirmation. |
| Accessibility | Modal and drawer focus isolation/restoration, combobox/listbox keyboard model, live regions, 44 px coarse-pointer targets, improved light-theme contrast, reduced-motion and print modes. |
| Responsive foundation | Unified the compact breakpoint at 900 px and added `100dvh`, safe-area insets, virtual-keyboard compensation, scroll containment and overflow-safe tables/carousels. |
| CI | Added 45 authenticated functional checks across Chromium, Firefox and WebKit on desktop, iPad Mini and two mobile profiles, including landscape, WCAG 2.2 AA, touch-target and horizontal-overflow assertions. |

## Verification strategy

The redesign is protected by:

- Go unit/integration and security tests;
- autosave ID/lock/conflict tests;
- search space/status filter tests;
- atomic workflow transition tests;
- PDF upload/viewer tests;
- reader draft-leak regression tests;
- JavaScript syntax checks;
- Playwright functional, responsive and accessibility checks in GitHub Actions;
- real canvas rendering of a structurally valid PDF fixture;
- explicit assertions for drawer focus restoration, 44 px touch targets and absence of page-level horizontal overflow.

## Remaining enterprise work

The browser PDF viewer is functional and access-controlled, but OCR, server-side per-page extraction and indexing PDF contents in global search remain separate roadmap work. OIDC, PostgreSQL/Redis and distributed storage are also outside this UI redesign and remain tracked in the enterprise roadmap.
