# System Architecture — Docs Hub Next

## 1. Overview & Architectural Vision

Docs Hub Next is designed as a **zero-dependency, single-binary, self-hosted enterprise knowledge base**. Unlike legacy document management systems that require complex multi-container orchestration or heavyweight JVM runtimes, Docs Hub combines the speed and simplicity of embedded SQLite with modern knowledge graph ergonomics inspired by Obsidian.

### Core Architectural Axioms
1. **Zero External Runtime Dependencies**: Standard deployment consists of a single statically compiled Go binary with embedded HTML/CSS/JS web assets.
2. **Local-First Speed with Multi-User Governance**: Lightning-fast page loads through embedded SQLite WAL and in-memory caches, paired with strict server-side authentication, CSRF tokens, and RBAC authorization.
3. **Markdown as the Universal Interchange Format**: Content is persisted as standard CommonMark / GitHub Flavored Markdown (GFM) with extended wiki-links `[[slug]]`, tags `#tag`, and Mermaid diagram fences.
4. **Resilient Data Integrity**: Full ACID guarantees via transactions, automated schema migrations, and point-in-time backup capabilities.

---

## 2. Package & Layer Structure

```text
Docs_Hub/
├── cmd/
│   ├── docshub/             # Application entrypoint & HTTP server bootstrap
│   ├── docshub-seed/        # Demo data generator & developer seeder
│   └── migrate-json/        # Migration utility for legacy JSON storage files
├── internal/
│   ├── application/         # Core application services (document, template, workflow, AI)
│   ├── auth/                # Argon2id password hashing and secret generation
│   ├── authn/               # Session management and OIDC identity authentication
│   ├── authz/               # Space and role-based access authorization rules
│   ├── config/              # Environment variable loading & validation logic
│   ├── db/                  # SQLite connection pool, WAL settings & embedded SQL migrations
│   ├── domain/              # Core domain entities, models, and value objects
│   ├── files/               # Media asset storage and file validation services
│   ├── httpapp/             # Chi HTTP router, handlers, middleware & CSRF protection
│   ├── markdownx/           # Markdown AST compiler, link extraction, sanitization
│   ├── repository/          # Repository contracts and SQLite implementation
│   ├── store/               # Legacy store compatibility and migration interfaces
│   └── web/                 # Embedded HTML templates, CSS tokens, and JavaScript assets
└── docs/                    # Architectural decisions, API specifications & guides
```

---

## 3. Storage Engine & Database Schema

Docs Hub utilizes embedded SQLite operating in **Write-Ahead Logging (WAL)** mode. This delivers concurrent reads without blocking writes and sub-millisecond query latencies.

### Key Schema Tables

```mermaid
erDiagram
    ORGANIZATIONS ||--o{ SPACES : contains
    SPACES ||--o{ ARTICLES : scopes
    USERS ||--o{ ARTICLES : authors
    ARTICLES ||--o{ ARTICLE_VERSIONS : tracks
    ARTICLES ||--o{ ARTICLE_TAGS : labeled_with
    TAGS ||--o{ ARTICLE_TAGS : associates
    ARTICLES ||--o{ LINKS : sources
    ARTICLES ||--o{ ARTICLE_FILES : attaches
    FILES ||--o{ ARTICLE_FILES : references
    USERS ||--o{ SESSIONS : authenticates
    USERS ||--o{ AUDIT_EVENTS : triggers
```

### Table Specifications
- **`articles`**: Master document record containing `id`, `slug`, `title`, `status` (`draft`, `in_review`, `published`, `archived`), `visibility` (`authenticated`, `private`, `public`), `lock_version` for optimistic locking, and category foreign keys.
- **`article_versions`**: Immutable snapshot of every revision containing full `content`, pre-rendered `rendered_html`, `author_id`, and timestamp.
- **`article_fts`**: SQLite FTS5 virtual table providing full-text search across document titles, content, and tags with BM25 ranking.
- **`links`**: Bidirectional link index tracking `from_article_id`, `target_slug`, and `label`. Enables $O(1)$ backlink queries and graph traversal.
- **`spaces` & `organizations`**: Logical isolation boundaries for multi-team access control.
- **`audit_events`**: Append-only security and operational audit trail tracking actor, action, IP address, and change metadata.

---

## 4. Markdown & Security Pipeline

Content processing follows a strict multi-stage compilation and sanitization pipeline to guarantee security and performance:

```mermaid
flowchart LR
    Raw["Raw Markdown Input"] --> AST["Goldmark AST Parser (GFM + Tables + Task Lists)"]
    AST --> Extract["Wiki-Link & Tag Extractor"]
    AST --> HTML["HTML Renderer"]
    HTML --> Sanitize["Bluemonday Strict Policy Sanitizer"]
    Sanitize --> SafeHTML["Safe Rendered HTML + Mermaid Fences"]
```

1. **AST Transformation**: `goldmark` parses CommonMark with GFM extensions (tables, strikethrough, autolinks).
2. **Wiki-Link Resolution**: Custom AST traverser resolves `[[slug]]` and `[[slug|Display Text]]` into relative `/a/{slug}` routes.
3. **XSS Sanitization**: `bluemonday.UGCPolicy()` strips all malicious `<script>`, `<iframe>`, `onerror`, and unauthorized protocols. Safe SVGs, audio/video players, and code blocks are preserved.
4. **Client Rendering**: Mermaid code blocks (`<pre class="mermaid">`) are dynamically rendered on the client canvas.

---

## 5. Security & Threat Modeling

- **Password Storage**: Argon2id with recommended OWASP parameters (memory: 64MB, iterations: 3, parallelism: 2, 16-byte random salt).
- **Session Tokens**: Cryptographically random 32-byte tokens stored in SQLite with TTL expiry and strict `SameSite=Lax`, `HttpOnly` cookie flags.
- **CSRF Defense**: Double-submit cryptographic token pattern verified on all non-idempotent HTTP methods (POST, PUT, DELETE).
- **Brute-Force Rate Limiting**: In-memory token-bucket limiter restricting login attempts and global request rates per remote IP.
- **Optimistic Concurrency**: `lock_version` integer verification on article edits preventing silent overwrites during concurrent edits.

---

## 6. Frontend Architecture

The user interface is built on a **Neo-Swiss Editorial Grid System** utilizing vanilla modern CSS and progressive JavaScript:
- **Design Tokens**: Centralized CSS custom properties for surfaces (`--surface-*`), typography scales (`--font-*`), and motion curves (`--ease-*`, `--motion-*`).
- **Responsive Geometry**: Fluid container queries and `clamp()` calculations adapting flawlessly from $320\text{px}$ mobile screens to $3840\text{px}$ 4K displays.
- **Theme & Accent Engine**: Perceptual Dark/Light themes with instant client-side switching and customizable accent palettes persisted via `localStorage`.
