# Product Roadmap — Docs Hub Next

## 🎯 Milestone 1 — Core Foundation & Persistence (Completed ✅)
- [x] SQLite WAL database architecture replacing legacy JSON storage.
- [x] Full-text search engine (FTS5) with BM25 ranking and tag matching.
- [x] Bidirectional wiki-links (`[[slug]]` / `[[slug|label]]`) and automatic backlink tracking.
- [x] Immutable document revision history with visual diff timestamps.
- [x] Interactive 2D Knowledge Graph Explorer using dynamic SVG.
- [x] Argon2id password hashing and secure session management.

## 🎨 Milestone 2 — Neo-Swiss Design & Fluid Responsiveness (Completed ✅)
- [x] Fluid typography and spacing system (`clamp()`, container queries).
- [x] Zero-overflow multi-device responsive layouts ($320\text{px} \to 3840\text{px}$).
- [x] Centralized motion design system with standardized easings.
- [x] Perceptual Dark/Light themes with 8 curated accent presets and custom color engine.
- [x] Contextual operational status bar with live UTC clock and network monitor.
- [x] Touch and mobile navigation drawer with accessible touch targets.

## 🛡️ Milestone 3 — Enterprise Governance & Workflows (Completed ✅)
- [x] Multi-space organizational isolation (`Engineering`, `Product`, `Operations`).
- [x] Document lifecycle workflow state machine (`Draft` &rarr; `In Review` &rarr; `Published` &rarr; `Archived`).
- [x] Comprehensive security baseline tests and Playwright E2E automation.
- [x] Rate limiting, CSRF protection, and Bluemonday XSS sanitization.
- [x] PDF viewing and embedded media attachments.

## 🚀 Milestone 4 — Enterprise Scale & Collaboration (Upcoming 🔮)
- [ ] Enterprise Single Sign-On (OIDC / SAML 2.0 / Google / GitHub).
- [ ] Real-time collaborative editing via CRDTs (Yjs / Automerge).
- [ ] Inline document comments, mentions (`@user`), and review suggestions.
- [ ] S3 / MinIO object storage driver for massive media libraries.
- [ ] Automated PDF / DOCX content extraction and indexing.
