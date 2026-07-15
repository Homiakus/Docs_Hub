# Docs_Hub

**Docs_Hub** — корпоративная база знаний и платформенное wiki-решение на Go для управления знаниями, вложениями, диаграммами и документами.

Версия: **v0.3.0-alpha.1**

## Документация проекта

- [Docs_Hub_Enterprise_Roadmap.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/Docs_Hub_Enterprise_Roadmap.md) — Дорожная карта превращения Docs_Hub в коммерческую Enterprise платформу
- [docs/ARCHITECTURE.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/docs/ARCHITECTURE.md) — Описание модульной архитектуры и принципов домена
- [docs/ROADMAP.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/docs/ROADMAP.md) — Краткое резюме релизных вех
- [CONTRIBUTING.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/CONTRIBUTING.md) — Правила участия в разработке
- [SECURITY.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/SECURITY.md) — Политика раскрытия уязвимостей
- [CHANGELOG.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/CHANGELOG.md) — Журнал релиза и изменений

## Быстрый старт

```bash
cp .env.example .env
# Заполните ADMIN_PASSWORD и SESSION_SECRET

# Запуск локально
export $(grep -v '^#' .env | xargs)
go run ./cmd/docshub

# Запуск в Docker
docker compose up -d --build
```

Откройте в браузере: `http://localhost:8080`

## Проверка состояния здоровья (Healthcheck)

Исполняемый файл предоставляет встроенную команду для контейнерного и внешнего контроля здоровья:

```bash
docshub healthcheck --url=http://127.0.0.1:8080/healthz
```

## Тестирование и Сборка

```bash
go test -v -race ./...
go vet ./...
go build -o bin/docshub ./cmd/docshub
```
