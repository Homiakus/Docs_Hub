# Project Guidelines & Specifications

## Overview
Project specification document for Spec-Driven Development Loops.

## Key Architectural Rules
- Maintain modular code structure and clean separation of concerns.
- All changes must pass hard verification gates (tests, linting, formatting).
- Do not break existing functionality or introduce unhandled errors.

## Automated Testing & Documentation Mandate
- Create and update automated test suites covering all core functionality.
- Keep PRD.md, CLAUDE.md, and ARCHITECTURE.md updated on every iteration pass.

<!-- claude-code-studio:begin -->
## Claude Code Studio — project contract

### Engineering workflow
- Treat this repository as a Go project unless the codebase proves otherwise.
- Read the nearest package, tests, interfaces, and call sites before editing.
- Prefer semantic tooling (gopls/Serena) over broad file dumps when available.
- Use Context7 or another trusted documentation source before relying on changing external APIs.
- For large cross-package work, make a compact repository map first (Repomix is preferred when installed).
- Keep changes focused. Do not reformat unrelated files or rewrite working code without a concrete reason.

### Go quality gate
- Format changed Go files with gofmt.
- Run the smallest relevant tests while iterating, then run `go test ./...` before declaring a meaningful change complete.
- Run `go vet ./...` for refactors, concurrency changes, unsafe code, build logic, or exported API changes.
- Run `go mod tidy` only when imports/dependencies changed; review go.mod/go.sum afterward.
- Do not add or upgrade dependencies silently. Explain why the dependency is needed.

### Git and release safety
- Inspect `git status` and `git diff` before any commit-like action.
- Never force-push, hard-reset, destructive-clean, rewrite history, publish a release, deploy, or apply infrastructure without explicit user intent.
- Never commit credentials, API keys, tokens, private keys, .env files, or generated secret material.

### Security and secrets
- Do not read secrets simply because a tool can. Respect permission deny rules and project boundaries.
- Prefer environment-variable references such as `${TOKEN_NAME}` in MCP configuration; keep the value outside the repository.
- Treat third-party plugins, MCP servers, hooks, and install scripts as executable code. Review source/trust before enabling medium/high-risk integrations.

### Completion report
- Summarize changed files and behavior.
- List verification commands actually run and their results.
- Call out unverified assumptions, skipped checks, and any required manual step.
<!-- claude-code-studio:end -->
