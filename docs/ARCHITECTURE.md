# Architecture

Docs Hub keeps the deployment simple: one Go binary, one JSON database file, one uploads directory, one backups directory.

## Components

- `main.go`: HTTP server, auth, ACL, editor UI, Markdown rendering, attachments, backups, migrations.
- `storage.json`: users, groups, articles, sessions, attachment metadata, audit and migration log.
- `uploads/`: generated filenames only; original filenames are metadata.
- `backups/`: admin-created zip snapshots.

## Security model

- Passwords: PBKDF2-HMAC-SHA256 with per-user salt.
- Sessions: server-side session records, cookie only contains session id + random token.
- CSRF: synchronizer token for admin POST actions.
- Uploads: extension allowlist, generated stored name, size limit, no execution from upload directory.
- Search: every result is filtered through `CanRead` before it can be displayed.

## Scaling path

- Replace JSON store with PostgreSQL.
- Move uploads to S3/MinIO.
- Replace built-in search with Meilisearch/OpenSearch.
- Bundle Toast UI Editor locally instead of loading it from the CDN.
