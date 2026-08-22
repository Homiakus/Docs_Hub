# ADR 0001: SQLite WAL as the Primary Storage Engine

## Status
Accepted

## Context
Previous iterations of Docs Hub relied on a monolithic flat JSON file (`storage.json`). While simple to inspect, this approach suffered from severe concurrency limitations, lack of indexing, high memory overhead for revision tracking, and potential data corruption upon sudden server terminations.

## Decision
We adopted **embedded SQLite operating in Write-Ahead Logging (WAL) mode** via the pure Go driver (`modernc.org/sqlite`).

## Consequences
- **Concurrency**: WAL allows concurrent readers without blocking active writers.
- **ACID Transactions**: Atomic schema changes, version creation, and tag linking.
- **Full-Text Search**: Built-in SQLite FTS5 extension eliminates the need for external search daemons (like Elasticsearch).
- **Zero-Dependency**: No CGo compilation requirements or separate database process management.
