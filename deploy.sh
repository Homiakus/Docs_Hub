#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

run_local() { exec "$ROOT/manage-local.sh"; }
run_production() { exec "$ROOT/manage-server.sh"; }

case "${1:-}" in
  local) shift; exec "$ROOT/manage-local.sh" "$@" ;;
  production|prod|server) shift; exec "$ROOT/manage-server.sh" "$@" ;;
  --help|-h)
    cat <<'HELP'
Docs Hub deployment launcher

Usage:
  ./deploy.sh local [command]       Local Docker deployment
  ./deploy.sh production [command] Production server + automatic HTTPS
  ./deploy.sh                       Interactive mode selector

Examples:
  ./deploy.sh local deploy
  ./deploy.sh local status
  ./deploy.sh production deploy
HELP
    ;;
  "")
    printf 'Docs Hub deployment mode\n\n'
    printf '  1) Local machine (HTTP, localhost/LAN)\n'
    printf '  2) Production server (Caddy + automatic HTTPS)\n'
    printf '  0) Exit\n\n'
    read -r -p 'Select mode: ' choice
    case "$choice" in
      1) run_local ;;
      2) run_production ;;
      0|q|quit|exit) exit 0 ;;
      *) printf 'Unknown mode.\n' >&2; exit 2 ;;
    esac
    ;;
  *) printf 'Unknown mode: %s\n' "$1" >&2; exit 2 ;;
esac
