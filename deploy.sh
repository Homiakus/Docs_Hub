#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"

run_local() { exec "$ROOT/manage-local.sh"; }
run_native() { exec "$ROOT/manage-native.sh"; }
run_production() { exec "$ROOT/manage-server.sh"; }

case "${1:-}" in
  local|docker-local) shift; exec "$ROOT/manage-local.sh" "$@" ;;
  native|no-docker|nodocker) shift; exec "$ROOT/manage-native.sh" "$@" ;;
  production|prod|server) shift; exec "$ROOT/manage-server.sh" "$@" ;;
  --help|-h)
    cat <<'HELP'
Docs Hub deployment launcher

Usage:
  ./deploy.sh local [command]       Local Docker deployment
  ./deploy.sh native [command]      Native Go deployment, no Docker
  ./deploy.sh production [command] Production server + automatic HTTPS
  ./deploy.sh                       Interactive mode selector

Examples:
  ./deploy.sh local deploy
  ./deploy.sh native deploy
  ./deploy.sh native status
  ./deploy.sh production deploy
HELP
    ;;
  "")
    printf 'Docs Hub deployment mode\n\n'
    printf '  1) Local machine — Docker (HTTP, localhost/LAN)\n'
    printf '  2) Native — WITHOUT Docker (Go binary, localhost/LAN)\n'
    printf '  3) Production server — Docker + Caddy + automatic HTTPS\n'
    printf '  0) Exit\n\n'
    read -r -p 'Select mode: ' choice
    case "$choice" in
      1) run_local ;;
      2) run_native ;;
      3) run_production ;;
      0|q|quit|exit) exit 0 ;;
      *) printf 'Unknown mode.\n' >&2; exit 2 ;;
    esac
    ;;
  *) printf 'Unknown mode: %s\n' "$1" >&2; exit 2 ;;
esac
