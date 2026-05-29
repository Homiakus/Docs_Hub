# ADR 0001: SQLite-first storage

## Status

Proposed

## Context

Docs Hub currently stores all business data in one JSON file. This is simple, but weak for search, concurrency, audit, version history, and ACL queries.

## Decision

Use SQLite as the next storage layer before PostgreSQL.

## Why SQLite first

- preserves simple deployment;
- still one database file;
- supports transactions;
- supports indexes;
- supports FTS5;
- makes migrations explicit;
- easier backup than custom JSON snapshots.

## Consequences

- JSON becomes import/export format;
- store package needs repository interfaces;
- schema migrations become first-class;
- tests can use in-memory SQLite.

