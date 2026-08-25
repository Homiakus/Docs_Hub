#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

VERSION="1.0.0"
ENV_FILE=".env.local"
COMPOSE_FILE="compose.local.yaml"
PROJECT_NAME="docshub-local"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'
  C_RED=$'\033[31m'; C_GREEN=$'\033[32m'; C_YELLOW=$'\033[33m'
  C_BLUE=$'\033[34m'; C_CYAN=$'\033[36m'
else
  C_RESET=""; C_BOLD=""; C_DIM=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_BLUE=""; C_CYAN=""
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
REPO_DIR="$SCRIPT_DIR"
DOCKER=()

die() { printf '%sERROR:%s %s\n' "$C_RED$C_BOLD" "$C_RESET" "$*" >&2; exit 1; }
info() { printf '%s→%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok() { printf '%s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*"; }

trap 'printf "\n%sERROR%s: command failed at line %s (exit %s).\n" "$C_RED$C_BOLD" "$C_RESET" "$LINENO" "$?" >&2' ERR

pause() {
  [[ "${NONINTERACTIVE:-0}" == "1" ]] && return 0
  printf '\n%sPress Enter to continue...%s' "$C_DIM" "$C_RESET"
  read -r _
}

confirm() {
  local prompt="${1:-Continue?}" answer
  [[ "${ASSUME_YES:-0}" == "1" ]] && return 0
  read -r -p "$prompt [y/N]: " answer
  [[ "$answer" =~ ^[YyДд]$ ]]
}

init_docker() {
  command -v docker >/dev/null 2>&1 || return 1
  docker info >/dev/null 2>&1 || return 1
  docker compose version >/dev/null 2>&1 || return 1
  DOCKER=(docker)
}

dc() {
  [[ ${#DOCKER[@]} -gt 0 ]] || init_docker || die "Docker Engine / Docker Desktop and Compose plugin are required."
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || die "$ENV_FILE is missing. Run local configuration first."
  "${DOCKER[@]}" compose \
    --project-name "$PROJECT_NAME" \
    --project-directory "$REPO_DIR" \
    --env-file "$REPO_DIR/$ENV_FILE" \
    -f "$REPO_DIR/$COMPOSE_FILE" "$@"
}

env_get() {
  local key="$1" file="$REPO_DIR/$ENV_FILE"
  [[ -f "$file" ]] || return 0
  awk -F= -v k="$key" '$1 == k {sub(/^[^=]*=/, ""); gsub(/^"|"$/, ""); print; exit}' "$file"
}

dotenv_escape() {
  local s="$1"
  s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; s="${s//$'\n'/ }"; s="${s//$'\r'/ }"
  printf '"%s"' "$s"
}

random_hex() {
  local bytes="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex "$bytes"
  elif command -v python3 >/dev/null 2>&1; then
    python3 -c "import secrets; print(secrets.token_hex($bytes))"
  else
    die "openssl or python3 is required to generate secure secrets."
  fi
}

validate_port() {
  [[ "$1" =~ ^[0-9]+$ ]] && (( 1 <= 10#$1 && 10#$1 <= 65535 ))
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${port}$"
  elif command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
  elif command -v netstat >/dev/null 2>&1; then
    netstat -an 2>/dev/null | grep -E "[.:]${port}[[:space:]].*LISTEN" >/dev/null
  else
    return 1
  fi
}

local_url() {
  local bind port host
  bind="$(env_get BIND_ADDR || true)"; port="$(env_get HOST_PORT || true)"
  bind="${bind:-127.0.0.1}"; port="${port:-8080}"
  host="$bind"
  [[ "$bind" == "0.0.0.0" ]] && host="127.0.0.1"
  printf 'http://%s:%s' "$host" "$port"
}

lan_ip() {
  if command -v hostname >/dev/null 2>&1; then
    local ip
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    [[ -n "$ip" ]] && { printf '%s' "$ip"; return; }
  fi
  if command -v ipconfig >/dev/null 2>&1; then
    local ip
    ip="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"
    [[ -n "$ip" ]] && { printf '%s' "$ip"; return; }
  fi
  printf ''
}

header() {
  local bind port
  bind="$(env_get BIND_ADDR || true)"; port="$(env_get HOST_PORT || true)"
  clear 2>/dev/null || true
  printf '%s%sDocs Hub — локальное развёртывание%s  %sv%s%s\n' "$C_BOLD" "$C_CYAN" "$C_RESET" "$C_DIM" "$VERSION" "$C_RESET"
  printf '%sКаталог:%s %s\n' "$C_DIM" "$C_RESET" "$REPO_DIR"
  printf '%sАдрес:%s  %s:%s\n' "$C_DIM" "$C_RESET" "${bind:-127.0.0.1}" "${port:-8080}"
  printf '%sДанные:%s  %s/data-local\n' "$C_DIM" "$C_RESET" "$REPO_DIR"
  printf '%s────────────────────────────────────────────────────────%s\n' "$C_DIM" "$C_RESET"
}

preflight() {
  printf '%sLocal preflight%s\n' "$C_BOLD" "$C_RESET"
  printf '  OS:       %s\n' "$(uname -s 2>/dev/null || printf unknown)"
  printf '  Arch:     %s\n' "$(uname -m 2>/dev/null || printf unknown)"
  printf '  Project:  %s\n' "$REPO_DIR"
  printf '  Free disk:%s\n' " $(df -h "$REPO_DIR" 2>/dev/null | awk 'NR==2 {print $4}' || true)"

  [[ -f "$REPO_DIR/Dockerfile" ]] || die "Dockerfile not found. Run this script from the Docs_Hub repository."
  [[ -f "$REPO_DIR/$COMPOSE_FILE" ]] || die "$COMPOSE_FILE not found."

  if init_docker; then
    ok "$(docker --version | head -n1)"
    ok "$(docker compose version)"
  else
    warn "Docker is not ready. Install/start Docker Desktop (Windows/macOS) or Docker Engine + Compose (Linux)."
    return 1
  fi

  local port="$(env_get HOST_PORT || true)"
  port="${port:-8080}"
  if port_in_use "$port"; then
    if [[ -f "$REPO_DIR/$ENV_FILE" ]] && dc ps --status running --services 2>/dev/null | grep -qx docshub; then
      ok "Port $port is used by the current Docs Hub local stack."
    else
      warn "Port $port is already in use. Choose another port in configuration."
    fi
  else
    ok "Port $port is available."
  fi
}

configure_local() {
  command -v openssl >/dev/null 2>&1 || command -v python3 >/dev/null 2>&1 || die "openssl or python3 is required."

  local current_port current_admin current_site current_pass
  current_port="$(env_get HOST_PORT || true)"
  current_admin="$(env_get ADMIN_USER || true)"; current_site="$(env_get SITE_NAME || true)"
  current_pass="$(env_get ADMIN_PASSWORD || true)"

  printf '%sAccess mode%s\n' "$C_BOLD" "$C_RESET"
  printf '  1) This computer only — 127.0.0.1 (recommended)\n'
  printf '  2) Local network — 0.0.0.0 (other devices can connect)\n'
  local mode bind port admin site password secret
  read -r -p "Mode [1]: " mode
  case "${mode:-1}" in
    1) bind="127.0.0.1" ;;
    2) bind="0.0.0.0" ;;
    *) warn "Unknown mode; using localhost only."; bind="127.0.0.1" ;;
  esac

  while true; do
    read -r -p "Host port [${current_port:-8080}]: " port
    port="${port:-${current_port:-8080}}"
    validate_port "$port" && break
    warn "Enter a port from 1 to 65535."
  done
  if (( 10#$port < 1024 )); then
    warn "Ports below 1024 can require elevated privileges on some systems."
  fi

  if port_in_use "$port" && [[ "$port" != "${current_port:-}" ]]; then
    warn "Port $port appears to be in use already."
    confirm "Save this port anyway?" || return 1
  fi

  read -r -p "Site name [${current_site:-Docs Hub Local}]: " site
  site="${site:-${current_site:-Docs Hub Local}}"
  read -r -p "Admin user [${current_admin:-admin}]: " admin
  admin="${admin:-${current_admin:-admin}}"
  [[ -n "$admin" ]] || die "Admin user cannot be empty."

  if [[ -n "$current_pass" ]] && confirm "Keep the current local administrator password?"; then
    password="$current_pass"
  else
    printf 'Admin password (leave blank to generate): '
    read -r -s password
    printf '\n'
    if [[ -z "$password" ]]; then
      password="$(random_hex 18)"
      printf '%sGenerated local admin password:%s %s%s%s\n' "$C_YELLOW$C_BOLD" "$C_RESET" "$C_BOLD" "$password" "$C_RESET"
      warn "Store this password now."
    elif ((${#password} < 8)); then
      die "Admin password must contain at least 8 characters."
    fi
  fi
  secret="$(random_hex 32)"

  umask 077
  local tmp="$REPO_DIR/$ENV_FILE.tmp"
  cat >"$tmp" <<EOF_ENV
# Generated by manage-local.sh. Do not commit this file.
BIND_ADDR=$bind
HOST_PORT=$port

ADMIN_USER=$(dotenv_escape "$admin")
ADMIN_PASSWORD=$(dotenv_escape "$password")
SESSION_SECRET=$secret

ADDR=:8080
DATA_DIR=/data
SITE_NAME=$(dotenv_escape "$site")
COOKIE_SECURE=0
LOG_LEVEL=info
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RPM=120
RATE_LIMIT_BURST=20
TLS_ENABLED=0
TLS_CERT_FILE=
TLS_KEY_FILE=
EOF_ENV
  chmod 600 "$tmp" 2>/dev/null || true
  mv -f "$tmp" "$REPO_DIR/$ENV_FILE"
  mkdir -p "$REPO_DIR/data-local"
  ok "Local configuration written to $ENV_FILE."
  printf '  URL: %s\n' "$(local_url)"
  if [[ "$bind" == "0.0.0.0" ]]; then
    local ip="$(lan_ip)"
    [[ -n "$ip" ]] && printf '  LAN: http://%s:%s\n' "$ip" "$port"
    warn "LAN mode uses plain HTTP. Use it only on a trusted local network."
  fi
}

validate_compose() {
  dc config -q
  ok "Local Docker Compose configuration is valid."
}

wait_for_app() {
  info "Waiting for Docs Hub..."
  local i
  for i in $(seq 1 40); do
    if dc exec -T docshub /app/docshub healthcheck --url=http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      ok "Application healthcheck is healthy."
      return 0
    fi
    sleep 2
  done
  warn "Application did not become healthy."
  dc ps || true
  dc logs --tail=120 docshub || true
  return 1
}

deploy_local() {
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || configure_local
  init_docker || die "Start Docker Desktop / Docker Engine first."
  validate_compose
  info "Building Docs Hub local image..."
  dc build docshub
  info "Starting local stack..."
  dc up -d --remove-orphans
  wait_for_app
  show_status
}

full_local_install() {
  preflight || true
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || configure_local
  deploy_local
}

show_status() {
  printf '%sLocal stack status%s\n' "$C_BOLD" "$C_RESET"
  if init_docker && [[ -f "$REPO_DIR/$ENV_FILE" ]]; then
    dc ps || true
    if dc exec -T docshub /app/docshub healthcheck --url=http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      ok "Healthcheck: healthy"
      printf '  URL: %s\n' "$(local_url)"
      local bind="$(env_get BIND_ADDR || true)" port="$(env_get HOST_PORT || true)"
      if [[ "$bind" == "0.0.0.0" ]]; then
        local ip="$(lan_ip)"
        [[ -n "$ip" ]] && printf '  LAN: http://%s:%s\n' "$ip" "${port:-8080}"
      fi
    else
      warn "Healthcheck: failed"
    fi
  else
    warn "Local stack is not configured or Docker is unavailable."
  fi
}

open_browser() {
  local url="$(local_url)"
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 &
  elif command -v open >/dev/null 2>&1; then
    open "$url" >/dev/null 2>&1 &
  elif command -v powershell.exe >/dev/null 2>&1; then
    powershell.exe -NoProfile -Command "Start-Process '$url'" >/dev/null 2>&1
  else
    warn "Could not open a browser automatically."
  fi
  printf 'URL: %s\n' "$url"
}

show_logs() {
  printf '%sFollowing local logs. Press Ctrl+C to return.%s\n' "$C_DIM" "$C_RESET"
  set +e
  dc logs -f --tail=200 docshub
  set -e
}

restart_local() {
  dc restart docshub
  wait_for_app
}

stop_local() {
  confirm "Stop local Docs Hub? Local data will be preserved." || return 0
  dc down
  ok "Local stack stopped; ./data-local was preserved."
}

reset_local_data() {
  warn "This permanently deletes the LOCAL database and local uploads in ./data-local."
  confirm "Delete all local Docs Hub data?" || return 0
  if init_docker && [[ -f "$REPO_DIR/$ENV_FILE" ]]; then
    dc down >/dev/null 2>&1 || true
  fi
  rm -rf "$REPO_DIR/data-local"
  mkdir -p "$REPO_DIR/data-local"
  ok "Local data reset. Production ./data was not touched."
}

print_help() {
  cat <<'EOF_HELP'
Docs Hub — local deployment manager

Topology:
  Browser -> http://127.0.0.1:HOST_PORT -> Docker -> Docs Hub :8080

Local and production data are isolated:
  local:      ./data-local
  production: ./data

Commands:
  ./manage-local.sh deploy
  ./manage-local.sh config
  ./manage-local.sh status
  ./manage-local.sh logs
  ./manage-local.sh stop

For Windows, the existing PowerShell manager is also available:
  .\\manage.ps1
EOF_HELP
}

menu() {
  while true; do
    header
    cat <<EOF_MENU
 ${C_BOLD}1${C_RESET}) Полное локальное развёртывание
 ${C_BOLD}2${C_RESET}) Настроить локальный адрес, порт и администратора
 ${C_BOLD}3${C_RESET}) Собрать и запустить локально
 ${C_BOLD}4${C_RESET}) Статус и healthcheck
 ${C_BOLD}5${C_RESET}) Открыть сайт в браузере
 ${C_BOLD}6${C_RESET}) Живые логи
 ${C_BOLD}7${C_RESET}) Перезапустить локальный контейнер
 ${C_BOLD}8${C_RESET}) Остановить локальный стек
 ${C_BOLD}9${C_RESET}) Сбросить ТОЛЬКО локальные данные
 ${C_BOLD}10${C_RESET}) Диагностика локальной машины
 ${C_BOLD}11${C_RESET}) Справка
 ${C_BOLD}0${C_RESET}) Выход
EOF_MENU
    printf '\nВыбор: '
    read -r choice
    printf '\n'
    case "$choice" in
      1) full_local_install; pause ;;
      2) configure_local; pause ;;
      3) deploy_local; pause ;;
      4) show_status; pause ;;
      5) open_browser; pause ;;
      6) show_logs ;;
      7) restart_local; pause ;;
      8) stop_local; pause ;;
      9) reset_local_data; pause ;;
      10) preflight; pause ;;
      11) print_help; pause ;;
      0|q|quit|exit) exit 0 ;;
      *) warn "Unknown menu item."; sleep 1 ;;
    esac
  done
}

main() {
  case "${1:-}" in
    --help|-h) print_help ;;
    --version|-v) printf '%s\n' "$VERSION" ;;
    deploy) full_local_install ;;
    config|configure) configure_local ;;
    status) show_status ;;
    logs) show_logs ;;
    stop) stop_local ;;
    restart) restart_local ;;
    reset) reset_local_data ;;
    *) menu ;;
  esac
}

main "$@"
