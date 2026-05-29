# Docs Hub Next

Новая версия Docs Hub: лёгкая корпоративная wiki в стиле Obsidian, но с серверной моделью доступа, аудитом и нормальной базой данных.

## Что уже заложено

- Go 1.23, один бинарный файл.
- SQLite WAL вместо самописной JSON-БД.
- Markdown pipeline: `goldmark` + GFM + подсветка кода + `bluemonday` sanitizer.
- Wiki-links `[[slug]]` и `[[slug|label]]`.
- Теги `#tag`.
- Backlinks через таблицу `links`, без парсинга на лету.
- Graph API `/api/graph`.
- Article versions через `article_versions`.
- FTS5 search через `article_fts`.
- Users, groups, ACL tables.
- Server-side sessions.
- Argon2id password hashing.
- Embedded templates/static через `embed`.
- Docker Compose.

## Production features (v0.2.0)

- **Config validation** — `ADMIN_PASSWORD` и `SESSION_SECRET` обязательны, с проверкой минимальной длины.
- **Rate limiting** — token-bucket на 60 req/min по IP, настраивается через `RATE_LIMIT_RPM` / `RATE_LIMIT_BURST`.
- **TLS support** — включается через `TLS_ENABLED=1` + `TLS_CERT_FILE` / `TLS_KEY_FILE`.
- **Health check** — `/healthz` проверяет подключение к БД, возвращает 503 при деградации.
- **Structured logging** — настраиваемый уровень через `LOG_LEVEL` (debug/info/warn/error).
- **Graceful shutdown** — 30s таймаут, IDLE/READ/WRITE таймауты на сервере.
- **CI/CD** — GitHub Actions: lint + test -race + build + Docker build.
- **Docker healthcheck** — встроен в Dockerfile и compose.yaml.

## Быстрый старт

```bash
cp .env.example .env
# заполните ADMIN_PASSWORD и SESSION_SECRET

# Локально (читает .env через export)
export $(grep -v '^#' .env | xargs)
go run ./cmd/docshub

# Или Docker
docker compose up -d --build
```

Открыть:

```
http://localhost:8080
```

## Конфигурация

Все параметры через env vars (см. `.env.example`):

| Переменная | По умолчанию | Описание |
|---|---|---|
| `ADMIN_PASSWORD` | **обязательно** | Пароль админа (мин. 8 символов) |
| `SESSION_SECRET` | **обязательно** | Секрет для сессий (мин. 16 символов) |
| `ADDR` | `:8080` | Адрес сервера |
| `DATA_DIR` | `./data` | Директория данных |
| `SITE_NAME` | `Docs Hub Next` | Название сайта |
| `ADMIN_USER` | `admin` | Логин админа |
| `COOKIE_SECURE` | `0` | Secure-флаг для cookie |
| `LOG_LEVEL` | `info` | Уровень логирования |
| `RATE_LIMIT_ENABLED` | `true` | Включить rate limiter |
| `RATE_LIMIT_RPM` | `60` | Запросов в минуту на IP |
| `RATE_LIMIT_BURST` | `10` | Размер burst |
| `TLS_ENABLED` | `0` | Включить HTTPS |
| `TLS_CERT_FILE` | — | Путь к сертификату |
| `TLS_KEY_FILE` | — | Путь к ключу |

## Разработка

```bash
make help     # список команд
make test     # тесты
make test-cov # coverage
make lint     # go vet
make build    # сборка бинарника
make all      # lint + test + build
```

## Миграция старого `storage.json` в SQLite

```bash
go run ./cmd/migrate-json --from ./data/storage.json --to ./data/docshub.db
```
