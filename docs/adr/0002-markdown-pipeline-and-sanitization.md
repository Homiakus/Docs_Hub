# ADR 0002: Goldmark and Bluemonday Markdown Security Pipeline

## Status
Accepted

## Context
Markdown rendering in a multi-user corporate wiki is vulnerable to cross-site scripting (XSS) attacks, malicious protocol injection (`javascript:`, `data:`), and unauthorized embedding.

## Decision
We implemented a two-pass rendering and sanitization architecture:
1. **Parser**: `goldmark` with GitHub Flavored Markdown (GFM) extensions parses CommonMark into an AST and extracts links/tags.
2. **Sanitizer**: `microcosm-cc/bluemonday` (`UGCPolicy()`) strips all unsafe HTML tags, attributes, and malicious event handlers from the compiled output.

## Consequences
- Guarantees strict defense-in-depth against stored XSS.
- Preserves safe user-generated elements such as table wrappers, code blocks, and Mermaid diagram definitions.
