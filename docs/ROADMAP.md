# Roadmap Docs_Hub

> **Примечание:** Полный корпоративный план развития размещён в [Docs_Hub_Enterprise_Roadmap.md](../Docs_Hub_Enterprise_Roadmap.md).

## Итоговые контрольные вехи (Milestones)

### Milestone A: Secure Internal Pilot (В процессе)
- [x] Очистка Git и гигиена репозитория (`bin/docshub`, `.gitignore`, `CONTRIBUTING.md`, `SECURITY.md`)
- [x] Исправление P0-уязвимостей авторизации (`editExisting`, `saveArticle`, autosave, workflow)
- [x] Фильтрация закрытых сущностей в `/api/graph`, поиске, обратных ссылках, activity, файлах и категориях
- [x] Единая CSRF-защита в JS-клиенте (`apiFetch`)
- [ ] Локальный Mermaid в strict режиме и контейнерный healthcheck CLI
- [x] Middleware заголовков безопасности (CSP, HSTS)

### Milestone B: Corporate MVP
- [x] Базовый домен Организаций и Пространств (Spaces)
- [ ] Аутентификация OIDC (Authorization Code Flow + PKCE)
- [x] Жизненный цикл документов (Draft -> Review -> Approved -> Published -> Archived)
- [x] Редакции версий и optimistic locking (`lock_version`)
- [ ] Безопасный бэкап и S3/MinIO адаптер вложений

### Milestone C: Enterprise Beta
- [ ] Поддержка PostgreSQL и Redis в профиле Enterprise
- [ ] PDF.js viewer, распознавание OCR и поиск по страницам
- [ ] Полнотекстовый ACL-aware поиск
- [x] Новый интерфейс навигации и разделённый Admin UI
- [ ] Система комментариев и уведомлений

### Milestone D: Commercial GA
- [ ] Завершение аудита безопасности, пентенст и прохождение DevSecOps ворот
- [ ] Соответствие WCAG 2.2 AA (Accessibility)
- [ ] Публикация пакета SLA, документации по Disaster Recovery и миграционных утилит
