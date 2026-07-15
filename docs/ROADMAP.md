# Roadmap Docs_Hub

> **Примечание:** Полный корпоративный детальный план развития проекта размещен в файле [Docs_Hub_Enterprise_Roadmap.md](file:///d:/Programms/202-Programming-Projects/Docs_Hub/Docs_Hub_Enterprise_Roadmap.md).

## Итоговые контрольные вехи (Milestones)

### Milestone A: Secure Internal Pilot (В процессе)
- [x] Очистка Git и гигиена репозитория (`bin/docshub`, `.gitignore`, `CONTRIBUTING.md`, `SECURITY.md`)
- [ ] Исправление P0-уязвимостей авторизации (`editExisting`, `saveArticle`)
- [ ] Фильтрация закрытых сущностей в `/api/graph`, обратных ссылках и категориях
- [ ] Единая CSRF-защита в JS-клиенте (`apiFetch`)
- [ ] Локальный Mermaid в strict режиме и контейнерный healthcheck CLI
- [ ] Middleware заголовков безопасности (CSP, HSTS)

### Milestone B: Corporate MVP
- [ ] Домен Организаций, Пространств (Spaces) и Групп
- [ ] Аутентификация OIDC (Authorization Code Flow + PKCE)
- [ ] Жизненный цикл документов (Draft -> Review -> Published -> Archived)
- [ ] Редакции версий, сравнение (diff) и optimistic locking (`lock_version`)
- [ ] Безопасный бэкап и S3/MinIO адаптер вложений

### Milestone C: Enterprise Beta
- [ ] Поддержка PostgreSQL и Redis в профиле Enterprise
- [ ] PDF.js viewer, распознавание OCR и поиск по страницам
- [ ] Полнотекстовый ACL-aware поиск
- [ ] Новый интерфейс навигации и разделенный Admin UI
- [ ] Система комментариев и уведомлений

### Milestone D: Commercial GA
- [ ] Завершение аудита безопасности, пентенст и прохождение DevSecOps ворот
- [ ] Соответствие WCAG 2.2 AA (Accessibility)
- [ ] Публикация пакета SLA, документации по Disaster Recovery и миграционных утилит
