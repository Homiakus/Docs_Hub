# Docs Hub Next — REST API Specification

This document describes the HTTP endpoints and JSON APIs exposed by Docs Hub Next.

---

## 1. Authentication & Security Headers

All state-changing endpoints (`POST`, `PUT`, `DELETE`) require a valid session cookie and a CSRF token passed either as:
- Form parameter: `csrf_token`
- Request header: `X-CSRF-Token: <token>`

---

## 2. Health & Telemetry Endpoints

### `GET /healthz`
Checks system liveness and database connectivity.

**Response (`200 OK`):**
```json
{
  "status": "ok",
  "app": "docshub-next",
  "time": "2026-08-22T12:00:00Z",
  "db": true
}
```

**Degraded Response (`503 Service Unavailable`):**
```json
{
  "status": "degraded",
  "app": "docshub-next",
  "time": "2026-08-22T12:00:00Z",
  "db": false
}
```

---

## 3. Search & Discovery Endpoints

### `GET /api/v1/search/suggest?q={query}`
Returns instant search suggestions across titles, tags, and document slugs.

**Query Parameters:**
- `q` (string, required): Search query or `#tag` name.

**Response (`200 OK`):**
```json
{
  "query": "architecture",
  "results": [
    {
      "id": 12,
      "slug": "system-architecture",
      "title": "System Architecture Overview",
      "status": "published",
      "category": "Engineering",
      "url": "/a/system-architecture"
    }
  ]
}
```

---

## 4. Knowledge Graph Topology

### `GET /api/graph`
Returns the entire document node and edge network for interactive graph visualization.

**Response (`200 OK`):**
```json
{
  "nodes": [
    {
      "id": "system-architecture",
      "label": "System Architecture Overview",
      "group": "Engineering"
    },
    {
      "id": "database-schema",
      "label": "Database Schema",
      "group": "Engineering"
    }
  ],
  "links": [
    {
      "source": "system-architecture",
      "target": "database-schema",
      "label": "References"
    }
  ]
}
```

---

## 5. Editor & Live Compilation

### `POST /api/preview`
Compiles raw Markdown into sanitized HTML on the server.

**Headers:**
- `Content-Type: text/plain; charset=utf-8`
- `X-CSRF-Token: <token>`

**Request Body:**
```markdown
# Realtime Heading
Wiki link to [[system-architecture]].
```

**Response (`200 OK`):**
```html
<h1 id="realtime-heading">Realtime Heading</h1>
<p>Wiki link to <a href="/a/system-architecture">system-architecture</a>.</p>
```

---

### `PUT /api/v1/documents/draft`
Saves an asynchronous working draft of an article without publishing a new revision.

**Headers:**
- `Content-Type: application/json`
- `X-CSRF-Token: <token>`

**Request Body:**
```json
{
  "id": 12,
  "slug": "system-architecture",
  "title": "System Architecture Overview (Draft)",
  "content": "# Updated draft content...",
  "lock_version": 3
}
```

**Response (`200 OK`):**
```json
{
  "status": "saved",
  "saved_at": "2026-08-22T12:05:00Z",
  "lock_version": 4
}
```

---

## 6. Media Asset Ingestion

### `POST /api/uploads`
Uploads binary assets (images, audio, video) and generates Markdown embed snippets.

**Headers:**
- `Content-Type: multipart/form-data`
- `X-CSRF-Token: <token>`

**Form Fields:**
- `file`: Binary file upload (`image/*`, `audio/*`, `video/*`).

**Response (`200 OK`):**
```json
{
  "key": "a1b2c3d4e5f6.webp",
  "url": "/uploads/a1b2c3d4e5f6.webp",
  "filename": "diagram.webp",
  "content_type": "image/webp",
  "markdown": "![diagram.webp](/uploads/a1b2c3d4e5f6.webp)"
}
```

---

## 7. Workflow Governance

### `POST /documents/{id}/workflow`
Transitions a document between editorial lifecycle states (`draft` &rarr; `in_review` &rarr; `published` &rarr; `archived`).

**Form Parameters:**
- `action`: `submit_review`, `approve_publish`, `request_changes`, `archive`.
- `csrf_token`: `<token>`.

**Response:**
Redirects (`303 See Other`) to `/a/{slug}`.
