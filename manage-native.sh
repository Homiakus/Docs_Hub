#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

VERSION="1.0.0"
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
ENV_FILE="$ROOT/.env.native"
DATA_DIR="$ROOT/data-native"
BIN_DIR="$ROOT/bin"
BIN="$BIN_DIR/docshub-native"
RUN_DIR="$ROOT/.run"
PID_FILE="$RUN_DIR/docshub-native.pid"
LOG_DIR="$ROOT/.logs"
LOG_FILE="$LOG_DIR/docshub-native.log"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  R=$'\033[0m'; B=$'\033[1m'; D=$'\033[2m'; RED=$'\033[31m'; G=$'\033[32m'; Y=$'\033[33m'; C=$'\033[36m'; BL=$'\033[34m'
else
  R="" B="" D="" RED="" G="" Y="" C="" BL=""
fi

die(){ printf '%sERROR:%s %s\n' "$RED$B" "$R" "$*" >&2; exit 1; }
info(){ printf '%s→%s %s\n' "$BL" "$R" "$*"; }
ok(){ printf '%s✓%s %s\n' "$G" "$R" "$*"; }
warn(){ printf '%s!%s %s\n' "$Y" "$R" "$*"; }
trap 'printf "\n%sERROR%s: line %s, exit %s\n" "$RED$B" "$R" "$LINENO" "$?" >&2' ERR

pause(){ [[ "${NONINTERACTIVE:-0}" == "1" ]] && return 0; printf '\n%sPress Enter to continue...%s' "$D" "$R"; read -r _; }
confirm(){ local a; [[ "${ASSUME_YES:-0}" == "1" ]] && return 0; read -r -p "${1:-Continue?} [y/N]: " a; [[ "$a" =~ ^[YyДд]$ ]]; }

ensure_dirs(){ mkdir -p "$DATA_DIR" "$BIN_DIR" "$RUN_DIR" "$LOG_DIR"; }

random_hex(){
  local bytes="${1:-32}"
  if command -v openssl >/dev/null 2>&1; then openssl rand -hex "$bytes"
  elif command -v python3 >/dev/null 2>&1; then python3 -c "import secrets; print(secrets.token_hex($bytes))"
  else die "openssl or python3 is required to generate secure secrets."; fi
}

shell_quote(){ local s="$1"; s=${s//\'/\'"\'"\'}; printf "'%s'" "$s"; }

env_get(){
  local key="$1"; [[ -f "$ENV_FILE" ]] || return 0
  awk -F= -v k="$key" '$1==k {sub(/^[^=]*=/, ""); gsub(/^\047|\047$/, ""); print; exit}' "$ENV_FILE"
}

validate_port(){ [[ "$1" =~ ^[0-9]+$ ]] && (( 1 <= 10#$1 && 10#$1 <= 65535 )); }

required_go_version(){ awk '$1=="go" {print $2; exit}' "$ROOT/go.mod"; }
go_version(){ go env GOVERSION 2>/dev/null | sed 's/^go//'; }
version_ge(){ local have="$1" need="$2"; [[ "$(printf '%s\n%s\n' "$need" "$have" | sort -V | head -n1)" == "$need" ]]; }

check_go(){
  command -v go >/dev/null 2>&1 || die "Go is not installed. Install Go $(required_go_version)+ and retry."
  local have need
  have="$(go_version)"; need="$(required_go_version)"
  version_ge "$have" "$need" || die "Go $need+ is required; detected Go $have."
  ok "Go $have"
}

native_addr(){ local a; a="$(env_get ADDR || true)"; printf '%s' "${a:-127.0.0.1:8080}"; }
local_url(){ local a host port; a="$(native_addr)"; host="${a%:*}"; port="${a##*:}"; [[ "$host" == "0.0.0.0" || "$host" == "::" ]] && host="127.0.0.1"; printf 'http://%s:%s' "$host" "$port"; }

lan_ip(){
  if command -v hostname >/dev/null 2>&1; then local ip; ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"; [[ -n "$ip" ]] && { printf '%s' "$ip"; return; }; fi
  if command -v ipconfig >/dev/null 2>&1; then local ip; ip="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || true)"; [[ -n "$ip" ]] && { printf '%s' "$ip"; return; }; fi
}

pid_value(){ [[ -f "$PID_FILE" ]] && tr -dc '0-9' < "$PID_FILE"; }
is_running(){ local p; p="$(pid_value || true)"; [[ -n "$p" ]] && kill -0 "$p" 2>/dev/null; }

load_env(){
  [[ -f "$ENV_FILE" ]] || die ".env.native is missing. Configure native mode first."
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
}

preflight(){
  printf '%sNative preflight (no Docker)%s\n' "$B" "$R"
  [[ -f "$ROOT/go.mod" && -d "$ROOT/cmd/docshub" ]] || die "Run this script from the Docs_Hub repository."
  check_go
  command -v git >/dev/null 2>&1 && ok "$(git --version)" || warn "git is not installed (build still may work)."
  printf '  OS:       %s\n' "$(uname -s 2>/dev/null || echo unknown)"
  printf '  Arch:     %s\n' "$(uname -m 2>/dev/null || echo unknown)"
  printf '  Project:  %s\n' "$ROOT"
  printf '  Data:     %s\n' "$DATA_DIR"
  printf '  Binary:   %s\n' "$BIN"
  if [[ -f "$ENV_FILE" ]]; then printf '  URL:      %s\n' "$(local_url)"; fi
}

configure(){
  ensure_dirs
  local old_addr old_user old_site old_pass host port user site pass secret mode
  old_addr="$(env_get ADDR || true)"; old_user="$(env_get ADMIN_USER || true)"; old_site="$(env_get SITE_NAME || true)"; old_pass="$(env_get ADMIN_PASSWORD || true)"
  local old_port="8080"; [[ -n "$old_addr" ]] && old_port="${old_addr##*:}"

  printf '%sAccess mode%s\n' "$B" "$R"
  printf '  1) This computer only — 127.0.0.1 (recommended)\n'
  printf '  2) Trusted LAN — 0.0.0.0\n'
  read -r -p 'Mode [1]: ' mode
  case "${mode:-1}" in 1) host="127.0.0.1";; 2) host="0.0.0.0";; *) host="127.0.0.1";; esac

  while true; do
    read -r -p "Port [$old_port]: " port; port="${port:-$old_port}"
    validate_port "$port" && break
    warn "Enter a port from 1 to 65535."
  done

  read -r -p "Site name [${old_site:-Docs Hub Native}]: " site; site="${site:-${old_site:-Docs Hub Native}}"
  read -r -p "Admin user [${old_user:-admin}]: " user; user="${user:-${old_user:-admin}}"; [[ -n "$user" ]] || die "Admin user cannot be empty."

  if [[ -n "$old_pass" ]] && confirm "Keep current native admin password?"; then pass="$old_pass"
  else
    printf 'Admin password (leave blank to generate): '; read -r -s pass; printf '\n'
    if [[ -z "$pass" ]]; then pass="$(random_hex 18)"; printf '%sGenerated admin password:%s %s%s%s\n' "$Y$B" "$R" "$B" "$pass" "$R"; warn "Store it now."
    elif ((${#pass} < 8)); then die "Admin password must contain at least 8 characters."; fi
  fi
  secret="$(random_hex 32)"

  umask 077
  cat > "$ENV_FILE.tmp" <<EOF_ENV
# Generated by manage-native.sh. Shell-compatible and git-ignored.
ADDR=$(shell_quote "$host:$port")
DATA_DIR=$(shell_quote "$DATA_DIR")
SITE_NAME=$(shell_quote "$site")
ADMIN_USER=$(shell_quote "$user")
ADMIN_PASSWORD=$(shell_quote "$pass")
SESSION_SECRET=$(shell_quote "$secret")
COOKIE_SECURE='0'
LOG_LEVEL='info'
RATE_LIMIT_ENABLED='true'
RATE_LIMIT_RPM='120'
RATE_LIMIT_BURST='20'
TLS_ENABLED='0'
TLS_CERT_FILE=''
TLS_KEY_FILE=''
EOF_ENV
  chmod 600 "$ENV_FILE.tmp" 2>/dev/null || true
  mv -f "$ENV_FILE.tmp" "$ENV_FILE"
  ok "Native configuration written: $ENV_FILE"
  printf '  URL: %s\n' "$(local_url)"
  if [[ "$host" == "0.0.0.0" ]]; then local ip; ip="$(lan_ip || true)"; [[ -n "$ip" ]] && printf '  LAN: http://%s:%s\n' "$ip" "$port"; warn "LAN mode is plain HTTP; use only on a trusted network."; fi
}

build(){
  check_go; ensure_dirs
  info "Downloading/verifying Go modules..."; (cd "$ROOT" && go mod download)
  info "Building native Docs Hub binary..."; (cd "$ROOT" && go build -trimpath -ldflags='-s -w' -o "$BIN" ./cmd/docshub)
  ok "Built: $BIN"; "$BIN" version || true
}

test_app(){ check_go; info "Running Go tests..."; (cd "$ROOT" && go test ./...); ok "Tests passed."; }

wait_health(){
  info "Waiting for healthcheck..."
  local i
  for i in $(seq 1 30); do curl -fsS --max-time 3 "$(local_url)/healthz" >/dev/null 2>&1 && { ok "Healthcheck: healthy"; return 0; }; sleep 1; done
  warn "Healthcheck failed. Last log lines:"; tail -n 80 "$LOG_FILE" 2>/dev/null || true; return 1
}

start(){
  [[ -f "$ENV_FILE" ]] || configure
  [[ -x "$BIN" ]] || build
  ensure_dirs
  if is_running; then ok "Docs Hub native is already running (PID $(pid_value))."; show_status; return 0; fi
  rm -f "$PID_FILE"
  load_env
  info "Starting native Docs Hub..."
  (cd "$ROOT" && nohup "$BIN" >> "$LOG_FILE" 2>&1 & echo $! > "$PID_FILE")
  sleep 0.2
  is_running || { warn "Process exited immediately."; tail -n 100 "$LOG_FILE" || true; return 1; }
  wait_health
  show_status
}

stop(){
  if ! is_running; then warn "Native process is not running."; rm -f "$PID_FILE"; return 0; fi
  local p; p="$(pid_value)"; info "Stopping PID $p..."; kill -TERM "$p" 2>/dev/null || true
  local i; for i in $(seq 1 30); do kill -0 "$p" 2>/dev/null || { rm -f "$PID_FILE"; ok "Stopped gracefully."; return 0; }; sleep 1; done
  warn "Graceful shutdown timed out."; if confirm "Force kill PID $p?"; then kill -KILL "$p" 2>/dev/null || true; rm -f "$PID_FILE"; ok "Process killed."; fi
}

restart(){ stop; start; }
deploy(){ preflight; [[ -f "$ENV_FILE" ]] || configure; build; start; }

show_status(){
  printf '%sNative status%s\n' "$B" "$R"
  if is_running; then
    printf '  Process:   running (PID %s)\n' "$(pid_value)"
    printf '  URL:       %s\n' "$(local_url)"
    curl -fsS --max-time 3 "$(local_url)/healthz" >/dev/null 2>&1 && ok "Healthcheck: healthy" || warn "Healthcheck: failed"
  else
    printf '  Process:   stopped\n'
  fi
  [[ -x "$BIN" ]] && printf '  Binary:    %s\n' "$BIN"
  [[ -f "$LOG_FILE" ]] && printf '  Log:       %s\n' "$LOG_FILE"
}

logs(){ ensure_dirs; touch "$LOG_FILE"; tail -n 200 -f "$LOG_FILE"; }

open_browser(){ local u; u="$(local_url)"; if command -v xdg-open >/dev/null 2>&1; then xdg-open "$u" >/dev/null 2>&1 & elif command -v open >/dev/null 2>&1; then open "$u" >/dev/null 2>&1 & elif command -v powershell.exe >/dev/null 2>&1; then powershell.exe -NoProfile -Command "Start-Process '$u'" >/dev/null 2>&1; else warn "Open manually: $u"; fi; printf 'URL: %s\n' "$u"; }

reset_data(){ warn "This deletes ONLY native data: $DATA_DIR"; confirm "Delete native database/uploads?" || return 0; is_running && stop; rm -rf "$DATA_DIR"; mkdir -p "$DATA_DIR"; ok "Native data reset. Docker/production data were not touched."; }

print_help(){ cat <<EOF_HELP
Docs Hub native deployment — no Docker

Requirements:
  Go $(required_go_version)+

Isolation:
  config: .env.native
  data:   data-native/
  binary: bin/docshub-native
  log:    .logs/docshub-native.log

Commands:
  ./manage-native.sh deploy
  ./manage-native.sh config
  ./manage-native.sh build
  ./manage-native.sh start
  ./manage-native.sh status
  ./manage-native.sh logs
  ./manage-native.sh restart
  ./manage-native.sh stop
  ./manage-native.sh test
EOF_HELP
}

header(){ clear 2>/dev/null || true; printf '%s%sDocs Hub — Native / без Docker%s  %sv%s%s\n' "$B" "$C" "$R" "$D" "$VERSION" "$R"; printf '%sПроект:%s %s\n' "$D" "$R" "$ROOT"; printf '%sДанные:%s %s\n' "$D" "$R" "$DATA_DIR"; [[ -f "$ENV_FILE" ]] && printf '%sURL:%s    %s\n' "$D" "$R" "$(local_url)"; printf '%s────────────────────────────────────────────────────────%s\n' "$D" "$R"; }

menu(){
  while true; do header; cat <<EOF_MENU
 ${B}1${R}) Полное развёртывание без Docker
 ${B}2${R}) Настроить адрес, порт и администратора
 ${B}3${R}) Собрать Go-бинарник
 ${B}4${R}) Запустить
 ${B}5${R}) Статус и healthcheck
 ${B}6${R}) Открыть сайт в браузере
 ${B}7${R}) Живые логи
 ${B}8${R}) Перезапустить
 ${B}9${R}) Остановить
 ${B}10${R}) Запустить Go-тесты
 ${B}11${R}) Диагностика
 ${B}12${R}) Сбросить ТОЛЬКО native-данные
 ${B}13${R}) Справка
 ${B}0${R}) Выход
EOF_MENU
    printf '\nВыбор: '; read -r ch; printf '\n'
    case "$ch" in
      1) deploy; pause;; 2) configure; pause;; 3) build; pause;; 4) start; pause;; 5) show_status; pause;; 6) open_browser; pause;; 7) logs;; 8) restart; pause;; 9) stop; pause;; 10) test_app; pause;; 11) preflight; pause;; 12) reset_data; pause;; 13) print_help; pause;; 0|q|quit|exit) exit 0;; *) warn "Unknown menu item."; sleep 1;; esac
  done
}

main(){ case "${1:-}" in --help|-h) print_help;; --version|-v) echo "$VERSION";; deploy) deploy;; config|configure) configure;; build) build;; start) start;; status) show_status;; logs) logs;; restart) restart;; stop) stop;; test) test_app;; doctor|preflight) preflight;; reset) reset_data;; *) menu;; esac; }
main "$@"
