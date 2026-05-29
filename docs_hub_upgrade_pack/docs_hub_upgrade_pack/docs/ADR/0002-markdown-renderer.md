# ADR 0002: Replace custom Markdown renderer

## Status

Proposed

## Context

Custom Markdown parsing and sanitization is security-sensitive and hard to extend.

## Decision

Use:

- `github.com/yuin/goldmark` for Markdown rendering;
- `github.com/microcosm-cc/bluemonday` for HTML sanitizing;
- custom extractors for wiki-links, tags, headings and tasks.

## Consequences

- safer HTML output;
- cleaner path to Mermaid, math, callouts;
- easier tests;
- fewer custom regex edge cases.

