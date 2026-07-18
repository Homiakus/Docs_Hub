# Docs_Hub

Docs_Hub — корпоративная база знаний на Go: пространства, полнотекстовый поиск, управляемый workflow документов, версии, вложения, PDF и граф связей.

Текущая версия: **v0.4.0-alpha.2**

## Что входит

- адаптивный интерфейс для desktop, tablet и mobile;
- Markdown-редактор с live preview, шаблонами, таблицами и Mermaid;
- надёжное автосохранение с optimistic locking и локальным восстановлением;
- жизненный цикл `draft → in_review → approved → published → archived`;
- ACL-aware поиск, пространства, категории и граф знаний;
- загрузка изображений, аудио, видео и PDF со встроенным viewer;
- админ-панель пользователей, ролей, контента, ACL, импорта и бэкапов;
- CSRF, CSP, защищённые сессии и аудит действий.

## Быстрый старт

```bash
cp .env.example .env
# Задайте ADMIN_PASSWORD и SESSION_SECRET в .env

set -a
. ./.env
set +a
go run ./cmd/docshub
```

Откройте `http://localhost:8080`. Для запуска в контейнере:

```bash
docker compose up -d --build
```

Демонстрационные пространства и документы можно добавить командой `go run ./cmd/docshub seed-demo`, предварительно задав `DEMO_PASSWORD` длиной не менее восьми символов.

Проверка здоровья:

```bash
go run ./cmd/docshub healthcheck --url=http://127.0.0.1:8080/healthz
```

## Проверки

```bash
go vet ./...
go test -race ./...
go build ./cmd/docshub

cd tests/e2e
npm ci
npx playwright install chromium firefox webkit
export E2E_ADMIN_PASSWORD="$(openssl rand -hex 16)"
export E2E_SESSION_SECRET="$(openssl rand -hex 32)"
npm test
```

CI дополнительно собирает Docker-образ и запускает Playwright в Chromium, Firefox и WebKit на desktop, tablet и mobile с accessibility-проверками.

## Документация

- [Архитектура](docs/ARCHITECTURE.md)
- [UI/UX audit и результаты redesign](docs/UI_UX_AUDIT_2026-07-18.md)
- [Roadmap](docs/ROADMAP.md)
- [Enterprise roadmap](Docs_Hub_Enterprise_Roadmap.md)
- [Вклад в проект](CONTRIBUTING.md)
- [Политика безопасности](SECURITY.md)
- [История изменений](CHANGELOG.md)
