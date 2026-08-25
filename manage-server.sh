#!/usr/bin/env bash
set -Eeuo pipefail
IFS=$'\n\t'

VERSION="1.0.0"
REPO_URL="https://github.com/Homiakus/Docs_Hub.git"
DEFAULT_INSTALL_DIR="/opt/docs-hub"
ENV_FILE=".env.production"
COMPOSE_FILE="compose.production.yaml"
PROJECT_NAME="docshub"
DEFAULT_BACKUP_DIR="/var/backups/docs-hub"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  C_RESET=$'\033[0m'
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_BLUE=$'\033[34m'
  C_CYAN=$'\033[36m'
else
  C_RESET="" C_BOLD="" C_DIM="" C_RED="" C_GREEN="" C_YELLOW="" C_BLUE="" C_CYAN=""
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
if [[ -f "$SCRIPT_DIR/go.mod" && -f "$SCRIPT_DIR/Dockerfile" ]]; then
  REPO_DIR="$SCRIPT_DIR"
elif [[ -d "$DEFAULT_INSTALL_DIR/.git" ]]; then
  REPO_DIR="$DEFAULT_INSTALL_DIR"
else
  REPO_DIR="$DEFAULT_INSTALL_DIR"
fi

SUDO=()
if (( EUID != 0 )); then
  if command -v sudo >/dev/null 2>&1; then
    SUDO=(sudo)
  else
    printf '%s\n' "This action requires root privileges and sudo is not installed." >&2
    exit 1
  fi
fi

DOCKER=()
RUN_USER="${SUDO_USER:-${USER:-root}}"
BACKUP_DIR="${DOCSHUB_BACKUP_DIR:-$DEFAULT_BACKUP_DIR}"

die() { printf '%sERROR:%s %s\n' "$C_RED$C_BOLD" "$C_RESET" "$*" >&2; exit 1; }
info() { printf '%s→%s %s\n' "$C_BLUE" "$C_RESET" "$*"; }
ok() { printf '%s✓%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '%s!%s %s\n' "$C_YELLOW" "$C_RESET" "$*"; }

on_error() {
  local line="$1" code="$2"
  printf '\n%sERROR%s: command failed at line %s (exit %s).\n' "$C_RED$C_BOLD" "$C_RESET" "$line" "$code" >&2
}
trap 'on_error "$LINENO" "$?"' ERR

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

as_root() {
  "${SUDO[@]}" "$@"
}

run_as_owner() {
  if (( EUID == 0 )) && [[ "$RUN_USER" != "root" ]] && command -v sudo >/dev/null 2>&1; then
    sudo -u "$RUN_USER" -- "$@"
  else
    "$@"
  fi
}

require_linux() {
  [[ "$(uname -s)" == "Linux" ]] || die "The production manager supports Linux servers only."
}

load_os_release() {
  [[ -r /etc/os-release ]] || die "/etc/os-release not found."
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID="${ID:-unknown}"
  OS_CODENAME="${VERSION_CODENAME:-}"
}

init_docker_cmd() {
  if ! command -v docker >/dev/null 2>&1; then
    DOCKER=()
    return 1
  fi
  if docker info >/dev/null 2>&1; then
    DOCKER=(docker)
  elif "${SUDO[@]}" docker info >/dev/null 2>&1; then
    DOCKER=("${SUDO[@]}" docker)
  else
    DOCKER=()
    return 1
  fi
  "${DOCKER[@]}" compose version >/dev/null 2>&1 || return 1
}

dc() {
  [[ ${#DOCKER[@]} -gt 0 ]] || init_docker_cmd || die "Docker Compose is unavailable."
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || die "$REPO_DIR/$ENV_FILE is missing. Configure production first."
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

domain_value() { env_get DOMAIN; }

header() {
  local domain="-"
  domain="$(domain_value || true)"
  clear 2>/dev/null || true
  printf '%s%sDocs Hub — управление сервером%s  %sv%s%s\n' "$C_BOLD" "$C_CYAN" "$C_RESET" "$C_DIM" "$VERSION" "$C_RESET"
  printf '%sРепозиторий:%s %s\n' "$C_DIM" "$C_RESET" "$REPO_DIR"
  printf '%sДомен:%s     %s\n' "$C_DIM" "$C_RESET" "${domain:-не настроен}"
  printf '%s────────────────────────────────────────────────────────%s\n' "$C_DIM" "$C_RESET"
}

validate_domain() {
  local domain="$1"
  [[ "$domain" != *"://"* && "$domain" != */* && "$domain" != *:* ]] || return 1
  [[ "$domain" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$ ]]
}

validate_email() {
  [[ "$1" =~ ^[^[:space:]@]+@[^[:space:]@]+\.[^[:space:]@]+$ ]]
}

dotenv_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/ }"
  s="${s//$'\r'/ }"
  printf '"%s"' "$s"
}

preflight() {
  require_linux
  load_os_release
  printf '%sSystem preflight%s\n' "$C_BOLD" "$C_RESET"
  printf '  OS:          %s %s\n' "${PRETTY_NAME:-$OS_ID}" "${VERSION_ID:-}"
  printf '  Arch:        %s\n' "$(uname -m)"
  printf '  Kernel:      %s\n' "$(uname -r)"
  printf '  Free disk:   %s\n' "$(df -h "$REPO_DIR" 2>/dev/null | awk 'NR==2 {print $4}' || df -h / | awk 'NR==2 {print $4}')"
  printf '  Install dir: %s\n' "$REPO_DIR"

  local missing=()
  for cmd in git curl openssl; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
  done
  if ((${#missing[@]})); then
    warn "Missing tools: ${missing[*]}"
  else
    ok "Base tools are present."
  fi

  if init_docker_cmd; then
    ok "$("${DOCKER[@]}" --version | head -n1)"
    ok "$("${DOCKER[@]}" compose version)"
  else
    warn "Docker Engine + Compose plugin are not ready."
  fi

  for port in 80 443; do
    if command -v ss >/dev/null 2>&1 && ss -ltn "sport = :$port" 2>/dev/null | grep -q LISTEN; then
      warn "TCP port $port is already in use. Existing web servers may conflict with Caddy."
    fi
  done
}

install_base_packages() {
  require_linux
  load_os_release
  case "$OS_ID" in
    ubuntu|debian)
      info "Installing base packages..."
      as_root apt-get update
      as_root apt-get install -y ca-certificates curl git openssl dnsutils
      ;;
    *)
      warn "Automatic package installation is implemented for Debian/Ubuntu."
      for cmd in git curl openssl getent; do
        command -v "$cmd" >/dev/null 2>&1 || die "Install '$cmd' manually, then retry."
      done
      ;;
  esac
}

install_docker() {
  if init_docker_cmd; then
    ok "Docker Engine and Compose plugin are already available."
    return 0
  fi

  require_linux
  load_os_release
  case "$OS_ID" in
    ubuntu|debian) ;;
    *) die "Automatic Docker installation currently supports Debian/Ubuntu only." ;;
  esac

  info "Installing Docker Engine from Docker's official APT repository."
  as_root apt-get update
  as_root apt-get install -y ca-certificates curl
  as_root install -m 0755 -d /etc/apt/keyrings

  local key_url repo_url
  key_url="https://download.docker.com/linux/$OS_ID/gpg"
  repo_url="https://download.docker.com/linux/$OS_ID"

  as_root curl -fsSL "$key_url" -o /etc/apt/keyrings/docker.asc
  as_root chmod a+r /etc/apt/keyrings/docker.asc

  [[ -n "$OS_CODENAME" ]] || die "Could not determine VERSION_CODENAME."
  local arch
  arch="$(dpkg --print-architecture)"

  as_root tee /etc/apt/sources.list.d/docker.sources >/dev/null <<EOF
Types: deb
URIs: $repo_url
Suites: $OS_CODENAME
Components: stable
Architectures: $arch
Signed-By: /etc/apt/keyrings/docker.asc
EOF

  as_root apt-get update

  local conflicts=()
  local pkg
  for pkg in docker.io docker-compose docker-doc docker-buildx podman-docker containerd runc; do
    dpkg-query -W -f='${Status}' "$pkg" 2>/dev/null | grep -q "install ok installed" && conflicts+=("$pkg") || true
  done
  if ((${#conflicts[@]})); then
    warn "Conflicting distro packages detected: ${conflicts[*]}"
    confirm "Remove them before installing official Docker packages?" || die "Docker installation cancelled."
    as_root apt-get remove -y "${conflicts[@]}"
  fi

  as_root apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  as_root systemctl enable --now docker
  init_docker_cmd || die "Docker installation completed but daemon/Compose is not usable."
  ok "Docker is ready."
}

ensure_repo() {
  if [[ -d "$REPO_DIR/.git" ]]; then
    ok "Repository already exists at $REPO_DIR."
    return 0
  fi

  info "Cloning $REPO_URL into $REPO_DIR..."
  as_root mkdir -p "$(dirname "$REPO_DIR")"
  if [[ "$RUN_USER" != "root" ]]; then
    as_root chown "$RUN_USER":"$(id -gn "$RUN_USER")" "$(dirname "$REPO_DIR")"
  fi
  run_as_owner git clone "$REPO_URL" "$REPO_DIR"
  [[ -f "$REPO_DIR/$COMPOSE_FILE" ]] || die "Production compose file is missing after clone."
  ok "Repository cloned."
}

configure_env() {
  ensure_repo
  command -v openssl >/dev/null 2>&1 || install_base_packages

  local current_domain current_email current_admin current_site current_pass
  current_domain="$(env_get DOMAIN || true)"
  current_email="$(env_get ACME_EMAIL || true)"
  current_admin="$(env_get ADMIN_USER || true)"
  current_site="$(env_get SITE_NAME || true)"
  current_pass="$(env_get ADMIN_PASSWORD || true)"

  local domain email admin site password session_secret
  while true; do
    read -r -p "Domain (e.g. docs.example.com) [${current_domain:-}]: " domain
    domain="${domain:-$current_domain}"
    validate_domain "$domain" && break
    warn "Enter a hostname only, without https://, path, or port."
  done

  while true; do
    read -r -p "ACME email [${current_email:-}]: " email
    email="${email:-$current_email}"
    validate_email "$email" && break
    warn "Enter a valid email address."
  done

  read -r -p "Site name [${current_site:-Docs Hub Next}]: " site
  site="${site:-${current_site:-Docs Hub Next}}"
  read -r -p "Admin user [${current_admin:-admin}]: " admin
  admin="${admin:-${current_admin:-admin}}"
  admin="${admin//$'\n'/}"
  [[ -n "$admin" ]] || die "Admin user cannot be empty."

  if [[ -n "$current_pass" ]] && confirm "Keep the current administrator password?"; then
    password="$current_pass"
  else
    printf 'Admin password (leave blank to generate a strong password): '
    read -r -s password
    printf '\n'
    if [[ -z "$password" ]]; then
      password="$(openssl rand -hex 18)"
      printf '%sGenerated admin password:%s %s%s%s\n' "$C_YELLOW$C_BOLD" "$C_RESET" "$C_BOLD" "$password" "$C_RESET"
      warn "Store this password in a password manager now; it will not be printed again."
    elif ((${#password} < 12)); then
      warn "Password is shorter than 12 characters."
      confirm "Use it anyway?" || return 1
    fi
  fi

  session_secret="$(openssl rand -hex 32)"

  umask 077
  local tmp="$REPO_DIR/$ENV_FILE.tmp"
  cat >"$tmp" <<EOF
# Generated by manage-server.sh. Do not commit this file.
DOMAIN=$domain
ACME_EMAIL=$email

ADMIN_USER=$(dotenv_escape "$admin")
ADMIN_PASSWORD=$(dotenv_escape "$password")
SESSION_SECRET=$session_secret

ADDR=:8080
DATA_DIR=/data
SITE_NAME=$(dotenv_escape "$site")

COOKIE_SECURE=1
LOG_LEVEL=info
RATE_LIMIT_ENABLED=true
RATE_LIMIT_RPM=60
RATE_LIMIT_BURST=10

# TLS terminates at Caddy. Keep application TLS disabled.
TLS_ENABLED=0
TLS_CERT_FILE=
TLS_KEY_FILE=
EOF
  chmod 600 "$tmp"
  mv -f "$tmp" "$REPO_DIR/$ENV_FILE"
  ok "Production configuration written to $REPO_DIR/$ENV_FILE (mode 600)."
}

dns_check() {
  local domain="${1:-$(domain_value)}"
  [[ -n "$domain" ]] || die "Domain is not configured."

  info "Checking DNS for $domain..."
  local a_records public_ip aaaa_records
  a_records="$(getent ahostsv4 "$domain" 2>/dev/null | awk '{print $1}' | sort -u | paste -sd ' ' - || true)"
  aaaa_records="$(getent ahostsv6 "$domain" 2>/dev/null | awk '{print $1}' | sort -u | paste -sd ' ' - || true)"
  public_ip="$(curl -4fsS --max-time 7 https://api.ipify.org 2>/dev/null || true)"

  printf '  A records:    %s\n' "${a_records:-none}"
  [[ -n "$aaaa_records" ]] && printf '  AAAA records: %s\n' "$aaaa_records"
  printf '  Server IPv4:  %s\n' "${public_ip:-unknown}"

  if [[ -z "$a_records" ]]; then
    warn "No IPv4 A record resolved. Public ACME issuance will normally fail."
    return 1
  fi
  if [[ -n "$public_ip" && " $a_records " != *" $public_ip "* ]]; then
    warn "The domain A record does not include this server's public IPv4."
    return 1
  fi
  ok "DNS A record looks compatible with this server."
}

configure_firewall() {
  if ! command -v ufw >/dev/null 2>&1; then
    load_os_release
    if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" ]]; then
      confirm "UFW is not installed. Install it?" || return 0
      as_root apt-get update
      as_root apt-get install -y ufw
    else
      warn "UFW not found. Open TCP 80/443 in your firewall or cloud security group."
      return 0
    fi
  fi

  local status
  status="$(as_root ufw status | head -n1 || true)"
  as_root ufw allow 80/tcp
  as_root ufw allow 443/tcp

  if [[ "$status" != *"active"* ]]; then
    warn "UFW is currently inactive."
    if confirm "Enable UFW now? SSH (22/tcp) will be allowed first."; then
      as_root ufw allow 22/tcp
      as_root ufw --force enable
    fi
  fi
  as_root ufw status verbose
}

validate_compose() {
  [[ -f "$REPO_DIR/$COMPOSE_FILE" ]] || die "$COMPOSE_FILE not found."
  dc config -q
  ok "Docker Compose configuration is valid."
}

wait_for_app() {
  info "Waiting for Docs Hub healthcheck..."
  local i
  for i in $(seq 1 40); do
    if dc exec -T docshub /app/docshub healthcheck --url=http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      ok "Application healthcheck is OK."
      return 0
    fi
    sleep 2
  done
  warn "Application did not become healthy in time."
  dc ps || true
  dc logs --tail=120 docshub || true
  return 1
}

wait_for_https() {
  local domain
  domain="$(domain_value)"
  [[ -n "$domain" ]] || return 1

  info "Waiting for HTTPS certificate and public endpoint..."
  local i
  for i in $(seq 1 30); do
    if curl -fsS --max-time 8 "https://$domain/healthz" >/dev/null 2>&1; then
      ok "HTTPS is live: https://$domain"
      return 0
    fi
    sleep 3
  done
  warn "HTTPS is not reachable yet. Check DNS, NAT/security-group rules, and ports 80/443."
  dc logs --tail=120 caddy || true
  return 1
}

deploy_stack() {
  ensure_repo
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || configure_env
  install_docker
  validate_compose

  if ! dns_check "$(domain_value)"; then
    confirm "DNS check failed. Start the stack anyway?" || return 1
  fi

  info "Pulling reverse proxy image..."
  dc pull caddy
  info "Building Docs Hub..."
  dc build --pull docshub
  info "Starting production stack..."
  dc up -d --remove-orphans

  wait_for_app
  wait_for_https || true
  show_status
}

ssl_info() {
  local domain
  domain="$(domain_value)"
  [[ -n "$domain" ]] || { warn "Domain is not configured."; return 1; }
  command -v openssl >/dev/null 2>&1 || { warn "openssl is not installed."; return 1; }

  printf '%sTLS certificate%s\n' "$C_BOLD" "$C_RESET"
  local cert
  cert="$(echo | openssl s_client -connect "$domain:443" -servername "$domain" -showcerts 2>/dev/null | openssl x509 -noout -subject -issuer -dates -serial 2>/dev/null || true)"
  if [[ -n "$cert" ]]; then
    printf '%s\n' "$cert"
  else
    warn "Could not retrieve the public certificate from $domain:443."
    return 1
  fi
}

show_status() {
  printf '\n%sStack status%s\n' "$C_BOLD" "$C_RESET"
  if init_docker_cmd && [[ -f "$REPO_DIR/$ENV_FILE" ]]; then
    dc ps || true
    if dc exec -T docshub /app/docshub healthcheck --url=http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      ok "Internal application healthcheck: healthy"
    else
      warn "Internal application healthcheck: failed"
    fi
  else
    warn "Docker stack is not configured/running."
  fi
  printf '\n'
  ssl_info || true
}

backup_data() {
  [[ -d "$REPO_DIR/data" ]] || { warn "No data directory exists yet."; return 1; }
  init_docker_cmd || die "Docker is required for a consistent cold backup."
  as_root mkdir -p "$BACKUP_DIR"

  local stamp archive was_running=0
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  archive="$BACKUP_DIR/docshub-data-$stamp.tar.gz"

  if dc ps --status running --services 2>/dev/null | grep -qx docshub; then
    was_running=1
    info "Stopping application briefly for a consistent SQLite/WAL backup..."
    dc stop -t 30 docshub
  fi

  if ! as_root tar -czf "$archive" -C "$REPO_DIR" data; then
    warn "Backup archive creation failed."
    if [[ "$was_running" == "1" ]]; then
      dc start docshub >/dev/null 2>&1 || true
    fi
    return 1
  fi
  as_root chmod 600 "$archive"

  if [[ "$was_running" == "1" ]]; then
    dc start docshub
    wait_for_app || true
  fi

  ok "Backup created: $archive"
  printf '  Size: %s\n' "$(du -h "$archive" | awk '{print $1}')"
}

restore_data() {
  as_root mkdir -p "$BACKUP_DIR"
  mapfile -t backups < <(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'docshub-data-*.tar.gz' -printf '%T@ %p\n' 2>/dev/null | sort -rn | cut -d' ' -f2-)
  ((${#backups[@]})) || { warn "No backups found in $BACKUP_DIR."; return 1; }

  printf '%sAvailable backups%s\n' "$C_BOLD" "$C_RESET"
  local i
  for i in "${!backups[@]}"; do
    printf '  %2d) %s\n' "$((i+1))" "${backups[$i]}"
  done
  local choice
  read -r -p "Select backup number: " choice
  [[ "$choice" =~ ^[0-9]+$ ]] || return 1
  (( choice >= 1 && choice <= ${#backups[@]} )) || return 1
  local archive="${backups[$((choice-1))]}"

  warn "Restore replaces the current data directory."
  confirm "Restore $archive?" || return 0

  init_docker_cmd || die "Docker is required."
  dc stop -t 30 docshub || true

  local safety="$BACKUP_DIR/pre-restore-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
  if [[ -d "$REPO_DIR/data" ]]; then
    as_root tar -czf "$safety" -C "$REPO_DIR" data
    ok "Safety backup created: $safety"
    as_root rm -rf "$REPO_DIR/data"
  fi
  as_root tar -xzf "$archive" -C "$REPO_DIR"
  as_root chown -R "$RUN_USER":"$(id -gn "$RUN_USER")" "$REPO_DIR/data" 2>/dev/null || true
  dc start docshub
  wait_for_app
  ok "Restore completed."
}

update_repo_and_deploy() {
  ensure_repo
  [[ -f "$REPO_DIR/$ENV_FILE" ]] || die "Configure production first."
  init_docker_cmd || install_docker

  if [[ -n "$(run_as_owner git -C "$REPO_DIR" status --porcelain --untracked-files=no)" ]]; then
    die "Tracked local changes exist. Commit/stash them before automated update."
  fi

  if [[ -d "$REPO_DIR/data" ]]; then
    backup_data
  fi

  info "Fetching origin/main..."
  run_as_owner git -C "$REPO_DIR" fetch --prune origin main
  local before after
  before="$(run_as_owner git -C "$REPO_DIR" rev-parse HEAD)"
  run_as_owner git -C "$REPO_DIR" merge --ff-only origin/main
  after="$(run_as_owner git -C "$REPO_DIR" rev-parse HEAD)"
  if [[ "$before" == "$after" ]]; then
    ok "Repository is already up to date."
  else
    ok "Updated: ${before:0:12} → ${after:0:12}"
  fi

  validate_compose
  dc pull caddy
  dc build --pull docshub
  dc up -d --remove-orphans
  wait_for_app
  wait_for_https || true
}

show_logs() {
  init_docker_cmd || die "Docker is unavailable."
  printf '%sFollowing logs. Press Ctrl+C to return.%s\n' "$C_DIM" "$C_RESET"
  set +e
  dc logs -f --tail=200
  set -e
}

restart_stack() {
  dc restart
  wait_for_app
  wait_for_https || true
}

stop_stack() {
  confirm "Stop Docs Hub and Caddy? Persistent data and certificates will be kept." || return 0
  dc down
  ok "Stack stopped. Named Caddy volumes and ./data were preserved."
}

full_install() {
  preflight
  install_base_packages
  install_docker
  ensure_repo
  if [[ ! -f "$REPO_DIR/$ENV_FILE" ]] || confirm "Reconfigure domain/admin settings before deployment?"; then
    configure_env
  fi
  if command -v ufw >/dev/null 2>&1 && as_root ufw status | head -n1 | grep -q active; then
    info "UFW is active; opening HTTP/HTTPS."
    as_root ufw allow 80/tcp
    as_root ufw allow 443/tcp
  else
    warn "Make sure ports 80/tcp and 443/tcp are open in the host/cloud firewall."
  fi
  deploy_stack
}

print_help() {
  cat <<'EOF'
Docs Hub — управление сервером

Production topology:
  Internet -> Caddy :80/:443 -> Docs Hub :8080 (private Docker network)

Caddy automatically provisions and renews ACME TLS certificates. The app itself
keeps TLS disabled and COOKIE_SECURE=1 is enabled.

Optional environment variables:
  DOCSHUB_BACKUP_DIR=/path  Backup directory (default /var/backups/docs-hub)
  ASSUME_YES=1             Auto-confirm non-destructive prompts
  NO_COLOR=1               Disable ANSI colors
EOF
}

menu() {
  while true; do
    header
    cat <<EOF
 ${C_BOLD}1${C_RESET}) Полное production-развёртывание (рекомендуется)
 ${C_BOLD}2${C_RESET}) Настроить домен, ACME email и администратора
 ${C_BOLD}3${C_RESET}) Собрать и запустить production
 ${C_BOLD}4${C_RESET}) Обновить из origin/main + backup + redeploy
 ${C_BOLD}5${C_RESET}) Статус, healthcheck и SSL-сертификат
 ${C_BOLD}6${C_RESET}) Живые логи
 ${C_BOLD}7${C_RESET}) Перезапустить стек
 ${C_BOLD}8${C_RESET}) Создать резервную копию данных
 ${C_BOLD}9${C_RESET}) Восстановить резервную копию
 ${C_BOLD}10${C_RESET}) Настроить UFW для HTTP/HTTPS
 ${C_BOLD}11${C_RESET}) Диагностика сервера (preflight)
 ${C_BOLD}12${C_RESET}) Остановить стек
 ${C_BOLD}13${C_RESET}) Справка
 ${C_BOLD}0${C_RESET}) Выход
EOF
    printf '\nВыбор: '
    read -r choice
    printf '\n'
    case "$choice" in
      1) full_install; pause ;;
      2) install_base_packages; ensure_repo; configure_env; pause ;;
      3) deploy_stack; pause ;;
      4) update_repo_and_deploy; pause ;;
      5) show_status; pause ;;
      6) show_logs ;;
      7) restart_stack; pause ;;
      8) backup_data; pause ;;
      9) restore_data; pause ;;
      10) configure_firewall; pause ;;
      11) preflight; pause ;;
      12) stop_stack; pause ;;
      13) print_help; pause ;;
      0|q|quit|exit) exit 0 ;;
      *) warn "Unknown menu item."; sleep 1 ;;
    esac
  done
}

main() {
  case "${1:-}" in
    --help|-h) print_help ;;
    --version|-v) printf '%s\n' "$VERSION" ;;
    deploy) full_install ;;
    status) show_status ;;
    update) update_repo_and_deploy ;;
    backup) backup_data ;;
    *) menu ;;
  esac
}

main "$@"
