#!/usr/bin/env bash
# Docs_Hub Interactive Setup Script (Linux / macOS)
# Encoding: UTF-8

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/.env"

# Colors
C_RESET='\033[0m'
C_CYAN='\033[0;36m'
C_YELLOW='\033[1;33m'
C_GREEN='\033[0;32m'
C_RED='\033[0;31m'
C_WHITE='\033[1;37m'
C_GRAY='\033[0;90m'

show_header() {
    clear
    echo -e "${C_CYAN}================================================================${C_RESET}"
    echo -e "${C_YELLOW}                DOCS_HUB INTERACTIVE SETUP & RUN                ${C_RESET}"
    echo -e "${C_CYAN}================================================================${C_RESET}"
    echo ""
}

generate_secret() {
    local len="${1:-32}"
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex "$len"
    else
        head -c "$len" /dev/urandom | xxd -p | tr -d '\n'
    fi
}

load_env() {
    if [[ -f "$ENV_FILE" ]]; then
        # Export all variables from .env
        set -a
        # shellcheck disable=SC1090
        source "$ENV_FILE"
        set +a
    fi
}

save_env() {
    cat <<EOF > "$ENV_FILE"
# Docs Hub Next — Configuration
# Auto-generated / configured by setup script

# Admin Credentials
ADMIN_USER=${ADMIN_USER:-admin}
ADMIN_PASSWORD=${ADMIN_PASSWORD}
SESSION_SECRET=${SESSION_SECRET}

# Server & Storage
ADDR=${ADDR:-:8080}
HOST_PORT=${HOST_PORT:-8080}
BIND_ADDR=${BIND_ADDR:-0.0.0.0}
DATA_DIR=${DATA_DIR:-./data}
UPLOAD_DIR=${UPLOAD_DIR:-./data/uploads}
DB_PATH=${DB_PATH:-./data/docshub.db}
SITE_NAME=${SITE_NAME:-Docs Hub Next}

# Telegram Bot Notifications
TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
TELEGRAM_NOTIFICATIONS_ENABLED=${TELEGRAM_NOTIFICATIONS_ENABLED:-false}

# Security
COOKIE_SECURE=${COOKIE_SECURE:-0}
LOG_LEVEL=${LOG_LEVEL:-info}
RATE_LIMIT_ENABLED=${RATE_LIMIT_ENABLED:-true}
RATE_LIMIT_RPM=${RATE_LIMIT_RPM:-60}
RATE_LIMIT_BURST=${RATE_LIMIT_BURST:-10}
EOF
    echo -e "${C_GREEN} -> Файл .env успешно сохранён!${C_RESET}"
}

configure_all() {
    show_header
    echo -e "${C_YELLOW}[1] Пошаговая настройка конфигурации .env${C_RESET}"
    echo "--------------------------------------------------------"
    load_env

    # 1. Admin username
    read -r -p "Имя пользователя администратора [${ADMIN_USER:-admin}]: " input_user
    ADMIN_USER="${input_user:-${ADMIN_USER:-admin}}"

    # 2. Admin password
    read -r -p "Пароль администратора (мин. 8 симв.) [${ADMIN_PASSWORD:-автогенерация}]: " input_pass
    if [[ -n "$input_pass" ]]; then
        ADMIN_PASSWORD="$input_pass"
    elif [[ -z "$ADMIN_PASSWORD" ]]; then
        ADMIN_PASSWORD=$(generate_secret 12)
        echo -e "${C_GREEN} -> Сгенерирован надёжный пароль: $ADMIN_PASSWORD${C_RESET}"
    fi

    # 3. Session Secret
    if [[ -z "$SESSION_SECRET" ]]; then
        SESSION_SECRET=$(generate_secret 32)
        echo -e "${C_GREEN} -> Сгенерирован 32-байтный SESSION_SECRET.${C_RESET}"
    fi

    # 4. Port
    read -r -p "Порт для Docs_Hub [${HOST_PORT:-8080}]: " input_port
    HOST_PORT="${input_port:-${HOST_PORT:-8080}}"
    ADDR=":$HOST_PORT"

    # 5. Telegram Bot
    echo ""
    echo -e "${C_CYAN}--- Настройка Telegram бота для уведомлений (опционально) ---${C_RESET}"
    read -r -p "Telegram Bot Token (из @BotFather) [${TELEGRAM_BOT_TOKEN}]: " input_token
    TELEGRAM_BOT_TOKEN="${input_token:-$TELEGRAM_BOT_TOKEN}"

    read -r -p "Telegram Chat ID / User ID (из @userinfobot) [${TELEGRAM_CHAT_ID}]: " input_chat
    TELEGRAM_CHAT_ID="${input_chat:-$TELEGRAM_CHAT_ID}"

    if [[ -n "$TELEGRAM_BOT_TOKEN" && -n "$TELEGRAM_CHAT_ID" ]]; then
        TELEGRAM_NOTIFICATIONS_ENABLED="true"
        echo -e "${C_GREEN} -> Telegram уведомления включены.${C_RESET}"
    else
        TELEGRAM_NOTIFICATIONS_ENABLED="false"
    fi

    save_env
    echo ""
    read -r -p "Нажмите Enter для возврата в меню..."
}

test_telegram() {
    show_header
    echo -e "${C_YELLOW}[2] Проверка отправки сообщения через Telegram Bot${C_RESET}"
    echo "--------------------------------------------------------"
    load_env

    if [[ -z "$TELEGRAM_BOT_TOKEN" || -z "$TELEGRAM_CHAT_ID" ]]; then
        echo -e "${C_RED}ОШИБКА: TELEGRAM_BOT_TOKEN или TELEGRAM_CHAT_ID не заполнены в .env!${C_RESET}"
        echo -e "${C_YELLOW}Сначала выполните пункт 1 для настройки.${C_RESET}"
        echo ""
        read -r -p "Нажмите Enter..."
        return
    fi

    echo -e "${C_CYAN}Отправка тестового сообщения ботом в чат $TELEGRAM_CHAT_ID...${C_RESET}"
    local msg="🚀 <b>Docs_Hub</b>: Тестовое уведомление успешно доставлено!\nСистема готова к работе."
    local resp
    resp=$(curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
        -d "chat_id=${TELEGRAM_CHAT_ID}" \
        -d "text=${msg}" \
        -d "parse_mode=HTML")

    if echo "$resp" | grep -q '"ok":true'; then
        echo -e "${C_GREEN} -> УСПЕХ! Сообщение отправлено в Telegram.${C_RESET}"
    else
        echo -e "${C_RED} -> Ошибка Telegram API: $resp${C_RESET}"
    fi

    echo ""
    read -r -p "Нажмите Enter..."
}

run_local() {
    show_header
    echo -e "${C_YELLOW}[3] Запуск Docs_Hub локально${C_RESET}"
    echo "--------------------------------------------------------"
    load_env

    if [[ -z "$ADMIN_PASSWORD" || -z "$SESSION_SECRET" ]]; then
        echo -e "${C_YELLOW}Внимание: .env не настроен. Запуск генерации...${C_RESET}"
        configure_all
        load_env
    fi

    echo -e "${C_GREEN}Запуск Docs_Hub на http://localhost:${HOST_PORT:-8080}...${C_RESET}"
    echo -e "${C_CYAN}Логин админа: ${ADMIN_USER:-admin}${C_RESET}"
    echo -e "${C_GRAY}Для остановки нажмите Ctrl+C${C_RESET}"
    echo ""

    go run ./cmd/docshub
}

run_docker() {
    show_header
    echo -e "${C_YELLOW}[4] Запуск Docs_Hub в Docker Compose${C_RESET}"
    echo "--------------------------------------------------------"

    if ! command -v docker >/dev/null 2>&1; then
        echo -e "${C_RED}ОШИБКА: Docker не найден в системе!${C_RESET}"
        echo ""
        read -r -p "Нажмите Enter..."
        return
    fi

    echo -e "${C_CYAN}Запуск: docker compose up -d --build...${C_RESET}"
    docker compose up -d --build
    echo ""
    echo -e "${C_GREEN}Контейнеры запущены. Проверка:${C_RESET}"
    docker compose ps
    echo ""
    read -r -p "Нажмите Enter..."
}

check_health() {
    show_header
    echo -e "${C_YELLOW}[5] Проверка состояния сервера (/healthz и /readyz)${C_RESET}"
    echo "--------------------------------------------------------"
    load_env
    local port="${HOST_PORT:-8080}"
    local base_url="http://localhost:$port"

    echo -e "${C_CYAN}Опрос $base_url/healthz...${C_RESET}"
    curl -s "$base_url/healthz" || echo -e "${C_RED}недоступен${C_RESET}"
    echo ""
    echo -e "${C_CYAN}Опрос $base_url/readyz...${C_RESET}"
    curl -s "$base_url/readyz" || echo -e "${C_RED}недоступен${C_RESET}"
    echo ""
    read -r -p "Нажмите Enter..."
}

# Main Loop
while true; do
    show_header
    echo -e "${C_WHITE}1. Пошаговая настройка .env (Admin, Пароль, Telegram Бот, Порт)${C_RESET}"
    echo -e "${C_WHITE}2. Проверить отправку тестового сообщения в Telegram${C_RESET}"
    echo -e "${C_WHITE}3. Запустить Docs_Hub локально (go run ./cmd/docshub)${C_RESET}"
    echo -e "${C_WHITE}4. Запустить Docs_Hub в Docker Compose${C_RESET}"
    echo -e "${C_WHITE}5. Проверить статус работающего сервера (/healthz, /readyz)${C_RESET}"
    echo -e "${C_WHITE}6. Сгенерировать новые случайные ключи безопасности${C_RESET}"
    echo -e "${C_GRAY}0. Выход${C_RESET}"
    echo ""
    read -r -p "Выберите пункт меню [0-6]: " choice

    case "$choice" in
        1) configure_all ;;
        2) test_telegram ;;
        3) run_local ;;
        4) run_docker ;;
        5) check_health ;;
        6)
            load_env
            ADMIN_PASSWORD=$(generate_secret 14)
            SESSION_SECRET=$(generate_secret 32)
            save_env
            echo -e "${C_GREEN}\nСгенерированы новые ключи:\nПароль: $ADMIN_PASSWORD\nSession Secret: $SESSION_SECRET${C_RESET}"
            echo ""
            read -r -p "Нажмите Enter..."
            ;;
        0) exit 0 ;;
        *) echo "Неверный выбор" ;;
    esac
done
