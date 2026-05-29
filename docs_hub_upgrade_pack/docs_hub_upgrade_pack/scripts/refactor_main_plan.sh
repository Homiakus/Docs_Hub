#!/usr/bin/env bash
set -euo pipefail

cat <<'PLAN'
Manual refactor plan for main.go:

1. Copy types User/Group/Article/etc. into internal/model.
2. Replace RenderMarkdown body with internal/markdown renderer call.
3. Move storage-related methods into internal/store/json_store.go.
4. Move routes() and handlers into internal/web.
5. Keep old main.go as thin composition root.

Do not run this as an automatic patch against production code yet.
PLAN
