# Docs_Hub: подробный технический план улучшения интерфейса и работы с документами

## 1. Назначение и область плана

План относится к актуальному состоянию ветки `main`, коммиту `2105357dae4b8d9f9e155d4968a3a58c19e77140` от 15 июля 2026 года.

Цель — превратить существующий интерфейс технической Markdown-wiki в современную корпоративную базу знаний, удобную для сотрудников без навыков Markdown и пригодную для коммерческого применения.

План охватывает:

- дизайн-систему;
- оболочку приложения и навигацию;
- главную страницу и поиск;
- чтение документов;
- редактор;
- таблицы;
- Mermaid и граф знаний;
- изображения, аудио и видео;
- загрузку и чтение PDF;
- версии и workflow;
- административный интерфейс;
- мобильный UX;
- доступность;
- производительность frontend;
- тестирование и rollout.

План не предполагает немедленного перехода на SPA. Рекомендуется сохранить server-side rendering на Go templates, а сложные области — редактор, table builder, PDF viewer и graph explorer — реализовать как изолированные интерактивные компоненты.

## 2. Ключевая проблема текущей реализации

Последний коммит добавил доменные модели и сервисные заготовки:

- `internal/application/document_service.go`;
- `internal/application/workflow_service.go`;
- `internal/application/template_service.go`;
- `internal/application/attachment_service.go`;
- `internal/authn/oidc.go`;
- revisions, workflow, spaces и PDF migrations.

Однако интерфейс продолжает использовать прежние маршруты и структуры:

```text
GET  /
GET  /a/{slug}
GET  /new
GET  /edit/{slug}
POST /save
POST /api/preview
POST /api/uploads
GET  /api/graph
GET  /admin
```

Новые сервисы не подключены к полноценным handlers и страницам. Поэтому первый архитектурный приоритет — не создавать дополнительные изолированные scaffolds, а соединить существующие domain/application packages с реальными пользовательскими сценариями.

## 3. Целевые продуктовые сценарии

После выполнения плана пользователь должен уметь:

1. Найти документ по названию, содержимому, тегу или тексту PDF.
2. Понять пространство, владельца, статус и актуальность документа.
3. Создать документ из шаблона без знания Markdown.
4. Вставить таблицу через визуальный builder.
5. Вставить Mermaid-диаграмму и увидеть ошибку с номером строки.
6. Загрузить изображение, задать подпись, размер и alt-текст.
7. Загрузить PDF, просмотреть его, выполнить поиск и скопировать ссылку на страницу.
8. Сохранить черновик автоматически.
9. Отправить редакцию на проверку и опубликовать после одобрения.
10. Сравнить и восстановить версии.
11. Увидеть только разрешённые действия.
12. Выполнить основные операции с клавиатуры и мобильного устройства.

## 4. Целевые UX-метрики

| Сценарий | Целевой показатель |
|---|---:|
| Найти известный документ | не более 15 секунд |
| Создать первый документ из шаблона | не более 2 минут |
| Вставить таблицу 5×5 | не более 60 секунд |
| Загрузить и вставить изображение | не более 30 секунд |
| Загрузить и открыть PDF | не более 30 секунд без учёта обработки |
| Перейти из поиска к найденной странице PDF | один клик |
| Отправить документ на review | не более 20 секунд |
| Task completion rate usability-тестов | не менее 90% |
| SUS | не менее 80 |
| WCAG | 2.2 AA |
| LCP для reader page | не более 2.5 секунды |
| CLS | не более 0.1 |

## 5. Целевая frontend-архитектура

### 5.1. Структура static assets

Разделить текущие монолитные `style.css` и `app.js`:

```text
internal/web/static/
  css/
    tokens.css
    reset.css
    typography.css
    layout.css
    utilities.css
    components/
      button.css
      input.css
      badge.css
      menu.css
      dialog.css
      toast.css
      tabs.css
      table.css
      attachment.css
      skeleton.css
    pages/
      home.css
      document.css
      editor.css
      search.css
      graph.css
      admin.css
      auth.css
    themes/
      light.css
      dark.css
  js/
    core/
      api-client.js
      csrf.js
      events.js
      dialog.js
      toast.js
      focus.js
    navigation/
      sidebar.js
      command-palette.js
      search-suggestions.js
    reader/
      toc.js
      tables.js
      code-copy.js
      attachments.js
    editor/
      editor.js
      autosave.js
      conflict.js
      toolbar.js
      wiki-links.js
      table-builder.js
      media-dialog.js
      mermaid-block.js
    graph/
      graph-explorer.js
    pdf/
      pdf-viewer.js
    admin/
      data-grid.js
```

На начальном этапе файлы можно собирать без сложного framework. Добавить esbuild/Vite только если это оправдано сложностью editor и viewer. Build должен создавать hashed assets и manifest, который читается Go template helpers.

### 5.2. Структура templates

```text
internal/web/templates/
  layouts/
    app.html
    auth.html
    admin.html
  components/
    app_header.html
    sidebar.html
    breadcrumb.html
    document_card.html
    status_badge.html
    metadata_row.html
    empty_state.html
    pagination.html
    dialog.html
    attachment_card.html
    permission_badge.html
  pages/
    home.html
    search.html
    spaces/index.html
    spaces/show.html
    documents/show.html
    documents/edit.html
    documents/history.html
    documents/compare.html
    documents/access.html
    graph.html
    pdf/viewer.html
    admin/users.html
    admin/groups.html
    admin/spaces.html
    admin/documents.html
    admin/files.html
    admin/audit.html
    admin/backups.html
    admin/settings.html
```

Изменить `internal/web/web.go`, чтобы embed включал вложенные каталоги или перечислял patterns явно.

### 5.3. Page/ViewModel слой

Не передавать в templates произвольный общий `Page` со всеми возможными полями. Создать специализированные view models:

```go
type AppShellVM struct {
    CurrentUser CurrentUserVM
    Navigation  NavigationVM
    Breadcrumbs []BreadcrumbVM
    CSRFToken   string
    Locale      string
    Theme       string
}

type DocumentPageVM struct {
    Shell       AppShellVM
    Document    DocumentVM
    Permissions DocumentPermissionsVM
    TOC         []TOCItemVM
    Revisions   []RevisionSummaryVM
    Attachments []AttachmentVM
}
```

Handlers должны готовить данные для конкретной страницы через application services.

## 6. Этап 0. Зафиксировать UX baseline

**Срок:** 3–5 рабочих дней.  
**Результат:** измеримая исходная точка.

### Задачи

1. Запустить приложение с демонстрационными данными:
   - 5 пространств;
   - 100 документов;
   - длинная статья;
   - статья с 20 версиями;
   - большие таблицы;
   - Mermaid;
   - изображения, видео и PDF;
   - пользователи reader/editor/admin.
2. Добавить dev seed command `docshub seed-demo`.
3. Снять screenshots на 320, 768, 1024 и 1440 px.
4. Добавить Playwright и visual snapshots.
5. Добавить axe-core проверки.
6. Зафиксировать текущие Core Web Vitals.
7. Провести 5 коротких usability-тестов:
   - найти документ;
   - создать статью;
   - вставить таблицу;
   - загрузить файл;
   - изменить доступ.

### Файлы

- создать `cmd/docshub-seed/main.go` или subcommand;
- создать `tests/e2e`;
- создать `playwright.config.ts`;
- обновить CI.

### Критерий приёмки

- baseline воспроизводим;
- screenshots хранятся как test artifacts;
- все обнаруженные UX-дефекты заведены в backlog с severity.

## 7. Этап 1. Дизайн-система и визуальные токены

**Срок:** 1–2 недели.  
**Зависимости:** этап 0.

### 7.1. Семантические цвета

Заменить прямые цвета на токены:

```css
:root {
  --surface-canvas: ...;
  --surface-primary: ...;
  --surface-secondary: ...;
  --surface-raised: ...;
  --surface-hover: ...;
  --text-primary: ...;
  --text-secondary: ...;
  --text-tertiary: ...;
  --text-inverse: ...;
  --border-subtle: ...;
  --border-strong: ...;
  --action-primary: ...;
  --action-primary-hover: ...;
  --status-success: ...;
  --status-warning: ...;
  --status-danger: ...;
  --focus-ring: ...;
}
```

Удалить прямые цвета `#d9d4e7`, `#7f788f`, `#6f697d`, `#767083` из sidebar.

### 7.2. Типографика

Ввести type scale и классы:

- `.text-display`;
- `.text-page-title`;
- `.text-document-title`;
- `.text-section-title`;
- `.text-body`;
- `.text-ui`;
- `.text-meta`.

Не полагаться на наличие Inter/JetBrains Mono в системе. Выбрать системный stack либо self-hosted fonts с лицензиями и preload.

### 7.3. Spacing и density

Создать шкалу 4/8/12/16/24/32/48. Определить normal и compact density для admin/data tables.

### 7.4. Компоненты

Стандартизировать:

- button variants;
- icon buttons;
- badges;
- inputs;
- selects;
- cards;
- menus;
- tooltips;
- dialogs;
- tabs;
- banners;
- toasts;
- skeletons;
- empty states.

### 7.5. Иконки

Выбрать единый SVG icon set и embed локально. Не использовать emoji как системные иконки theme, warning или file type.

### Тесты

- contrast AA light/dark;
- visual snapshots компонентов;
- focus-visible;
- forced-colors mode;
- 200% zoom;
- prefers-reduced-motion.

### Критерий приёмки

- в CSS отсутствуют случайные hardcoded UI colors;
- light и dark themes проходят contrast tests;
- компоненты имеют hover/focus/disabled/loading/error states.

## 8. Этап 2. Новая оболочка и информационная архитектура

**Срок:** 2–3 недели.  
**Зависимости:** этап 1.

### 8.1. Подключить spaces

Backend уже содержит `Organization` и `Space`, но templates их не используют.

Добавить в repository/application:

```go
ListSpacesForUser(ctx, userID, organizationID)
GetSpaceTree(ctx, userID, organizationID)
GetSpaceHome(ctx, spaceID, userID)
```

Добавить routes:

```text
GET /spaces
GET /spaces/{spaceSlug}
GET /spaces/{spaceSlug}/documents
```

### 8.2. Sidebar

Структура:

```text
Логотип / название
Поиск или Ctrl+K

Главная
Избранное
Недавние
Мои черновики
На проверке

Пространства
  Quality
  Engineering
  HR

Настройки/Администрирование
Профиль
```

Добавить:

- active state;
- collapse sidebar;
- collapse spaces;
- сохранение состояния;
- счётчики только для действительно важных элементов;
- permission-aware navigation.

### 8.3. Breadcrumbs

Добавить `BreadcrumbVM` и component. Breadcrumb формируется из organization/space/document hierarchy.

### 8.4. Profile menu

Заменить статичный user chip меню:

- профиль;
- тема;
- язык;
- активные сессии;
- выход.

### 8.5. Mobile navigation

- focus trap;
- `inert` для фона;
- возврат фокуса;
- Escape close;
- scroll lock;
- overlay вместо обязательного fullscreen;
- bottom action bar для editor.

### Файлы

- переписать `templates/base.html`;
- создать components sidebar/breadcrumb/profile menu;
- создать `navigation_service.go`;
- разделить navigation JS;
- заменить sidebar CSS.

### Критерий приёмки

- пользователь всегда понимает текущий space и документ;
- недоступные actions не отображаются;
- sidebar работает мышью, клавиатурой и touch;
- на 320 px search и actions не перекрываются.

## 9. Этап 3. Главная страница и поиск

**Срок:** 2–3 недели.  
**Зависимости:** этап 2.

### 9.1. Dashboard queries

Добавить application methods:

```go
ListRecentDocuments(ctx, userID, limit)
ListFavoriteDocuments(ctx, userID, limit)
ListUserDrafts(ctx, userID, limit)
ListReviewQueue(ctx, userID, limit)
ListRecentlyPublished(ctx, userID, limit)
ListStaleOwnedDocuments(ctx, userID, limit)
```

При необходимости добавить таблицы:

```sql
user_favorites(user_id, document_id, created_at)
document_views(user_id, document_id, viewed_at)
```

### 9.2. Home UI

Заменить простой список в `home.html` секциями:

- продолжить работу;
- мои черновики;
- ожидают review;
- пространства;
- недавно опубликовано;
- устаревшие документы владельца.

Создать reusable `document_card.html`.

### 9.3. Search page

Вынести результаты из home в `/search`:

```text
GET /search?q=&space=&type=&status=&owner=&updated=
GET /api/v1/search/suggest?q=
```

Результат содержит:

- title;
- highlighted snippet;
- space path;
- status;
- owner;
- updated date;
- type;
- PDF page при совпадении во вложении.

### 9.4. Command palette

`Ctrl/Cmd+K`:

- поиск;
- переход в space;
- создать документ;
- открыть recent;
- выполнить разрешённые команды.

### Критерий приёмки

- slug не является главным metadata в списке;
- поиск полностью управляется клавиатурой;
- zero-results предлагает изменить filters;
- результаты никогда не раскрывают недоступные документы.

## 10. Этап 4. Новый reader документов

**Срок:** 2–3 недели.  
**Зависимости:** этапы 2–3.

### 10.1. Document header

Изменить `article.html`:

- breadcrumbs;
- space;
- document status;
- classification;
- owner;
- author последней редакции;
- локализованная дата;
- next review date;
- favorite/subscription actions;
- permission-aware edit/review/publish menu.

Slug перенести в Properties.

### 10.2. Правый inspector

Заменить одновременно показанные history/backlinks вкладками:

```text
Содержание | Связи | История | Свойства
```

Состояние выбранной вкладки допускается хранить в URL hash или session storage.

### 10.3. TOC

- sticky;
- active heading через IntersectionObserver;
- collapse nested levels;
- copy heading link;
- reading progress;
- fallback для мобильного как collapsible details.

### 10.4. Content blocks

Добавить стили и renderer support:

- note;
- info;
- warning;
- danger;
- decision;
- procedure step;
- checklist;
- tabs;
- collapsible section;
- reference card;
- attachment card.

### 10.5. Footer

- related documents;
- attachments;
- feedback;
- report outdated;
- owner contact;
- comments/subscription.

### 10.6. Versions

Version item должен быть ссылкой. Добавить routes:

```text
GET  /documents/{id}/revisions
GET  /documents/{id}/revisions/{revision}
GET  /documents/{id}/compare?from=&to=
POST /documents/{id}/revisions/{revision}/restore
```

### Критерий приёмки

- документ читается без технических metadata;
- TOC показывает текущий раздел;
- версия открывается и сравнивается;
- reader layout не имеет горизонтального scroll на 320 px.

## 11. Этап 5. Основа нового редактора

**Срок:** 3–5 недель.  
**Зависимости:** новая document/revision модель.

### 11.1. Подключить DocumentService

Создать новые handlers, использующие `internal/application/document_service.go`, вместо прямого SQL в `server.go`.

Routes:

```text
GET  /documents/new
POST /documents
GET  /documents/{id}/edit
PUT  /api/v1/documents/{id}/draft
POST /api/v1/documents/{id}/revisions
```

Старые `/new`, `/edit/{slug}`, `/save` временно оставить redirects/compatibility routes.

### 11.2. Editor page model

```go
type EditorPageVM struct {
    Shell        AppShellVM
    Document     EditorDocumentVM
    Spaces       []SpaceOptionVM
    Templates    []TemplateOptionVM
    Permissions  DocumentPermissionsVM
    Attachments  []AttachmentVM
    CSRFToken    string
}
```

### 11.3. Layout

- заголовок как главный input;
- content canvas;
- toolbar;
- edit/preview/split modes;
- properties drawer;
- attachment panel;
- sticky action bar;
- status and save indicator.

Slug сделать автоматически генерируемым и скрыть в Advanced settings.

### 11.4. Autosave

Реализовать `autosave.js`:

- debounce 800–1500 мс;
- AbortController;
- sequence/version protection;
- states idle/dirty/saving/saved/error/offline/conflict;
- retry с backoff;
- local recovery draft;
- `beforeunload` только при несохранённых данных.

Autosave обновляет draft snapshot, а не создаёт permanent revision на каждый запрос.

### 11.5. Optimistic locking

Передавать `lock_version`. API при конфликте возвращает `409`:

```json
{
  "error": "edit_conflict",
  "server_lock_version": 8,
  "server_revision_id": 42
}
```

Открывать conflict dialog:

- сравнить;
- скопировать мой вариант;
- открыть новую версию;
- объединить вручную.

### 11.6. Preview

- AbortController;
- request sequence;
- debounce;
- local loading state;
- error panel;
- повторная инициализация tables/Mermaid/media;
- одинаковый renderer для preview и published content.

### Критерий приёмки

- autosave отображает достоверное состояние;
- закрытие вкладки не теряет текст;
- два редактора не перезаписывают друг друга тихо;
- editor работает на документах 100+ КБ без заметных задержек.

## 12. Этап 6. Форматирование, шаблоны и wiki-links

**Срок:** 3–5 недель.  
**Зависимости:** этап 5.

### 12.1. Toolbar

- heading;
- bold/italic/strike;
- lists/checklist;
- link/wiki-link;
- code;
- quote/callout;
- table;
- image/file;
- Mermaid;
- undo/redo.

Toolbar должен иметь keyboard shortcuts и tooltips.

### 12.2. Slash commands

`/` открывает список блоков с поиском. Команды фильтруются по введённому тексту и доступны с клавиатуры.

### 12.3. Wiki-link autocomplete

Endpoint:

```text
GET /api/v1/documents/suggest?q=&space_id=&limit=10
```

Показывать title, space и status. Не возвращать недоступные документы.

### 12.4. Templates

Подключить `TemplateService` к реальному UI:

- выбор шаблона до создания;
- preview структуры;
- required metadata;
- default space;
- owner/reviewer policy;
- review interval.

### 12.5. Paste handling

Добавить обработку вставки из Word/Google Docs:

- очистка styles;
- сохранение headings/lists/tables/links;
- импорт pasted images через upload pipeline;
- предупреждение о неподдерживаемых элементах.

### Критерий приёмки

- пользователь создаёт документ без ручного синтаксиса Markdown;
- link autocomplete не раскрывает private titles;
- paste из Word не создаёт небезопасный HTML.

## 13. Этап 7. Таблицы

**Срок:** 2–4 недели.  
**Зависимости:** этапы 5–6.

### 13.1. Исправить renderer и sanitizer

1. Разрешить только allowlisted classes/attributes для table blocks.
2. Добавить semantic `caption` и `scope`.
3. После preview вызывать `enhanceTables(container)`.
4. Удалить недостижимые CSS-классы либо сделать их доступными через block model.

### 13.2. Table builder

Создать dialog:

- grid selection;
- rows/columns;
- header row/column;
- alignment;
- compact mode;
- wrapping;
- caption;
- вставка/удаление строк;
- clipboard paste.

### 13.3. Reader table toolbar

Для таблиц выше заданного размера:

- fullscreen;
- copy;
- CSV export;
- sticky first column;
- search внутри таблицы.

### 13.4. DataTable block

Для CSV/XLSX создать отдельный интерактивный блок с virtualization, sorting, filtering и pagination. Не загружать все 10 000 строк в DOM.

### Тесты

- 100 колонок;
- 10 000 строк;
- mobile scroll;
- keyboard navigation;
- screen reader headers;
- CSV formula injection при экспорте;
- ACL export.

## 14. Этап 8. Mermaid и граф знаний

**Срок:** 2–4 недели.

### 14.1. Mermaid

1. Удалить CDN из `base.html`.
2. Хранить фиксированный local bundle.
3. Подключать editor bundle на странице редактирования всегда.
4. Использовать AST fenced block, а не regex с лимитом 1000 символов.
5. Валидировать syntax.
6. Рендерить light/dark theme корректно.
7. Добавить error state с line/column.

### 14.2. Diagram viewer

- zoom/pan;
- fit;
- fullscreen;
- export SVG/PNG;
- copy source;
- caption;
- alt description;
- fallback source.

### 14.3. Graph API

Endpoint должен принимать:

```text
GET /api/v1/graph?document_id=&depth=2&space_id=&tag=&status=&limit=
```

Все nodes и edges фильтруются по ACL до ответа.

### 14.4. Graph UI

- search node;
- filters;
- zoom/pan;
- focus neighborhood;
- cluster by space/tag;
- legend;
- details panel;
- loading/error/empty;
- доступная list alternative.

### Критерий приёмки

- graph полезен при тысячах документов за счёт neighborhood loading;
- private metadata не раскрываются;
- Mermaid не загружает внешний код и не исполняет unsafe HTML.

## 15. Этап 9. Медиа и attachment manager

**Срок:** 3–5 недель.

### 15.1. Подключить AttachmentService

Создать routes:

```text
POST   /api/v1/attachments
GET    /api/v1/attachments/{id}
GET    /api/v1/attachments/{id}/content
PATCH  /api/v1/attachments/{id}
DELETE /api/v1/attachments/{id}
GET    /api/v1/documents/{id}/attachments
POST   /api/v1/documents/{id}/attachments/{attachmentID}
```

### 15.2. Upload UI

- progress;
- cancel;
- retry;
- multiple uploads;
- thumbnails;
- scanning/ready/rejected states;
- компактный drop overlay только при dragenter;
- понятные сообщения об ограничениях.

### 15.3. Image dialog

- alt text;
- caption;
- size preset;
- alignment;
- lightbox;
- replace;
- usage references.

### 15.4. Audio/video

- title/caption;
- poster;
- subtitles WebVTT;
- transcript;
- chapters;
- timestamp links;
- download permission.

### 15.5. Attachment panel

В editor и reader:

- список файлов;
- type icon;
- size/status;
- open/insert/copy/download/replace/delete;
- referenced/unreferenced indicator.

### Критерий приёмки

- upload progress не блокирует editor;
- недоступный attachment невозможно получить по прямому URL;
- удаление предупреждает обо всех использованиях.

## 16. Этап 10. PDF viewer и поиск по PDF

**Срок:** 4–6 недель.  
**Зависимости:** этап 9.

### 16.1. Upload

Разрешить `application/pdf` как отдельный attachment kind. Проверять magic bytes, MIME, size, encryption и scan status.

### 16.2. Processing API

Подключить `attachment_pages` к background jobs:

```text
upload -> quarantine -> antivirus -> metadata -> text extraction
       -> OCR if needed -> thumbnails -> indexing -> ready
```

Endpoint статуса:

```text
GET /api/v1/attachments/{id}/processing
```

### 16.3. Viewer

Локально разместить PDF.js. Реализовать:

- thumbnails;
- page navigation;
- zoom;
- fit width/page;
- fullscreen;
- text search;
- result navigation;
- `#page=N` deep link;
- print/download permissions;
- mobile toolbar;
- loading/error/password states.

### 16.4. PDF block

В редакторе пользователь выбирает:

- file card;
- inline viewer;
- start page;
- height;
- caption;
- разрешить download при наличии permission.

### 16.5. Search integration

Результат содержит file title, document title, page и snippet. Клик открывает viewer на странице результата.

### Тесты

- text PDF;
- scan/OCR;
- encrypted;
- corrupted;
- embedded JS/files;
- 1000+ страниц;
- Cyrillic;
- denied read/download/print;
- mobile.

## 17. Этап 11. Workflow, revisions и comments UI

**Срок:** 3–5 недель.

### 17.1. Workflow actions

Подключить `WorkflowService`:

```text
POST /api/v1/documents/{id}/submit-review
POST /api/v1/documents/{id}/approve
POST /api/v1/documents/{id}/reject
POST /api/v1/documents/{id}/publish
POST /api/v1/documents/{id}/archive
```

Каждая кнопка отображается по permission и текущему status.

### 17.2. Review dialog

- выбрать reviewers;
- deadline;
- message;
- required approvals;
- notify options.

### 17.3. Revision history

- список;
- published marker;
- change summary;
- author/date;
- compare selection;
- restore as new revision.

### 17.4. Comments

Добавить document comments, mentions, resolve/reopen и notification inbox. Inline comments внедрять после стабилизации block IDs.

### Критерий приёмки

- draft не меняет published content;
- reviewer может одобрить только разрешённую revision;
- restore создаёт новую revision, не удаляя историю.

## 18. Этап 12. Административный интерфейс

**Срок:** 3–5 недель.

### 18.1. Разделить admin routes

```text
GET /admin
GET /admin/users
GET /admin/groups
GET /admin/spaces
GET /admin/permissions
GET /admin/documents
GET /admin/files
GET /admin/audit
GET /admin/backups
GET /admin/authentication
GET /admin/settings
```

### 18.2. Data grids

Общий компонент:

- server pagination;
- search;
- filters;
- sorting;
- selection;
- bulk actions;
- column visibility;
- empty/loading/error states.

### 18.3. User management

- отдельная create/edit page или dialog;
- группы;
- роли;
- status;
- last login;
- active sessions;
- revoke sessions;
- reset password;
- audit trail.

### 18.4. Permissions explorer

- subject/resource search;
- effective permissions;
- источник права;
- inheritance;
- excessive access report;
- export.

### 18.5. Files

- processing status;
- type/size/uploader;
- usage count;
- quarantine/rejected;
- orphan files;
- retry extraction;
- delete.

### 18.6. Audit

- filters actor/action/resource/date/IP;
- immutable detail drawer;
- export с отдельным permission;
- pagination.

### Критерий приёмки

- admin UI работает при 10 000 пользователей и 100 000 документов;
- опасные действия требуют confirmation и повторной server authorization;
- parent container не обрезает data grids на мобильных.

## 19. Этап 13. OIDC и login UX

**Срок:** 1–2 недели после готовности authn backend.

### Задачи

1. Подключить OIDC routes.
2. Добавить SSO button на login.
3. Показывать локальный login только если разрешён policy.
4. Добавить название организации и provider.
5. Добавить состояния redirect/error/account disabled.
6. Добавить страницу active sessions.
7. Улучшить form labels и error summary.

### Критерий приёмки

- пользователь понимает предпочтительный способ входа;
- SSO errors не раскрывают чувствительные детали;
- login полностью доступен с клавиатуры и screen reader.

## 20. Этап 14. Мобильный UX и адаптивность

**Срок:** 2–3 недели, частично выполняется в каждом этапе.

### Breakpoints

Проектировать минимум для:

- 320–479;
- 480–767;
- 768–1023;
- 1024–1439;
- 1440+.

### Сценарии

- mobile reader;
- mobile search;
- mobile editor;
- mobile table;
- mobile PDF;
- mobile graph;
- mobile admin read-only/basic actions.

### Изменения

- bottom editor action bar;
- collapsible properties;
- single-pane edit/preview toggle;
- touch targets не менее 44×44;
- dialogs как bottom sheets где уместно;
- horizontal table affordance;
- PDF compact toolbar;
- никакого page-level horizontal scroll.

## 21. Этап 15. Доступность WCAG 2.2 AA

**Срок:** 2–3 недели финального hardening и постоянные gates.

### Обязательные изменения

- skip link;
- landmarks;
- heading hierarchy;
- labels/descriptions;
- error summary;
- aria-current;
- focus management;
- dialogs;
- live regions autosave/upload;
- reduced motion;
- contrast;
- 200/400% zoom;
- keyboard table/graph/viewer;
- alt/caption workflows;
- transcripts/subtitles;
- accessible TOC;
- accessible PDF controls.

### CI

- axe on ключевых pages;
- Lighthouse accessibility threshold;
- visual focus snapshots;
- keyboard E2E.

## 22. Этап 16. Frontend-производительность и безопасность

**Срок:** 1–2 недели hardening.

### Производительность

- hashed assets;
- compression;
- correct cache headers;
- lazy loading PDF/Mermaid/graph;
- image srcset;
- virtualization large data grids;
- AbortController search/preview;
- avoid layout shifts;
- bundle budgets.

### Security

- убрать inline scripts;
- строгая CSP без `unsafe-inline`;
- local Mermaid/PDF.js;
- Trusted Types при целесообразности;
- не использовать непроверенный `innerHTML`, кроме server-sanitized renderer boundary;
- sanitize pasted HTML;
- CSRF-aware API client;
- no sensitive data in frontend logs.

### Budgets

| Bundle | Gzip budget |
|---|---:|
| Core shell | 80 КБ |
| Editor initial | 250 КБ |
| Mermaid lazy chunk | 300 КБ |
| PDF viewer lazy chunk | 500 КБ |
| Graph lazy chunk | 300 КБ |

Точные budgets скорректировать после выбора библиотек, но контролировать их в CI.

## 23. Тестовая стратегия

### 23.1. Unit

- view model builders;
- permission-based actions;
- autosave state machine;
- search debounce/cancel;
- table serialization;
- attachment block serialization;
- URL/deep links;
- locale formatting.

### 23.2. Integration

- handlers с application services;
- templates render для ролей;
- document workflow;
- revision conflicts;
- upload processing states;
- PDF extraction status;
- search ACL.

### 23.3. E2E

1. Reader находит и читает документ.
2. Editor создаёт документ из шаблона.
3. Autosave восстанавливает текст.
4. Два редактора получают conflict.
5. Table builder вставляет таблицу.
6. Mermaid показывает preview/error.
7. Image upload задаёт alt/caption.
8. PDF открывается на найденной странице.
9. Reviewer approve/publish.
10. Admin создаёт group и выдаёт space role.
11. Reader не видит create/edit/admin actions.
12. Keyboard-only сценарии.
13. Mobile viewport.

### 23.4. Visual regression

Для light/dark:

- login;
- home;
- search;
- document;
- long document;
- editor;
- table;
- Mermaid;
- PDF viewer;
- graph;
- admin grid;
- dialogs/empty/error/loading.

### 23.5. Usability

После Milestone B и перед GA провести тесты минимум с:

- техническим автором;
- обычным сотрудником;
- reviewer/manager;
- администратором;
- mobile user.

## 24. Рекомендуемый порядок pull requests

1. `test/ui-baseline-playwright-axe`
2. `ui/design-tokens-light-dark`
3. `ui/component-foundations`
4. `refactor/template-layout-components`
5. `feat/spaces-navigation-breadcrumbs`
6. `feat/home-dashboard-document-cards`
7. `feat/search-page-suggestions-filters`
8. `feat/document-reader-metadata-toc`
9. `feat/revision-history-compare-ui`
10. `refactor/connect-document-service-handlers`
11. `feat/editor-shell-autosave-locking`
12. `feat/editor-toolbar-slash-commands`
13. `feat/templates-wikilink-autocomplete`
14. `feat/table-builder-reader-tools`
15. `feat/mermaid-local-viewer`
16. `feat/graph-explorer`
17. `refactor/connect-attachment-service`
18. `feat/media-manager`
19. `feat/pdf-upload-processing-viewer`
20. `feat/pdf-search-page-links`
21. `feat/workflow-review-publish-ui`
22. `feat/comments-notifications-ui`
23. `refactor/admin-separate-pages`
24. `feat/admin-data-grids-permissions-audit`
25. `feat/oidc-login-session-ui`
26. `fix/mobile-accessibility-hardening`
27. `perf/frontend-bundles-caching`
28. `chore/ui-ga-visual-regression-usability`

Не объединять redesign целиком в один PR. Каждый PR должен иметь screenshots, E2E/visual tests и описание миграции UX.

## 25. Релизные этапы

### Milestone UI-A: исправленная основа

**Срок:** 3–4 недели.

- design tokens;
- исправленная light theme;
- компоненты;
- новая shell/navigation;
- responsive/focus foundations;
- Playwright/axe/visual baseline.

### Milestone UI-B: современное чтение и поиск

**Срок:** 6–8 недель от начала.

- spaces;
- dashboard;
- search;
- новый reader;
- TOC;
- revisions/compare;
- attachments display.

### Milestone UI-C: корпоративный редактор

**Срок:** 10–14 недель.

- DocumentService integration;
- autosave/locking;
- toolbar/slash commands;
- templates;
- table builder;
- Mermaid;
- media manager;
- workflow UI.

### Milestone UI-D: PDF и Enterprise admin

**Срок:** 14–18 недель.

- PDF processing/viewer/search;
- graph explorer;
- отдельные admin pages;
- permissions explorer;
- audit/files screens;
- OIDC UI.

### Milestone UI-GA

**Срок:** 18–22 недели.

- WCAG 2.2 AA;
- mobile hardening;
- performance budgets;
- usability testing;
- external accessibility review;
- отсутствие UI P0/P1;
- complete visual regression suite.

## 26. Команда

Минимальная рекомендуемая команда:

- 1 senior product/UX designer;
- 1 senior frontend engineer;
- 1 frontend/full-stack engineer;
- 1 Go backend engineer;
- 1 QA/SDET;
- part-time accessibility specialist;
- product owner для приоритизации сценариев.

При одном разработчике сначала реализовать UI-A, reader/search, autosave/locking и базовый PDF viewer. Graph explorer, comments, расширенные data tables и сложный block editor перенести после стабильного MVP.

## 27. Definition of Done интерфейса

Интерфейс можно считать готовым к коммерческому применению, когда:

- нет смешения языков и технических значений в пользовательском UI;
- light/dark themes проходят contrast AA;
- навигация использует spaces, tree и breadcrumbs;
- действия отображаются по effective permissions;
- главная поддерживает drafts/reviews/recent/favorites;
- поиск возвращает snippets и PDF pages;
- reader показывает статус, владельца, актуальность и published revision;
- revisions открываются, сравниваются и восстанавливаются;
- editor имеет autosave, locking, toolbar, templates и attachment manager;
- таблицы создаются без ручного Markdown;
- Mermaid поддерживает validation, zoom и export;
- изображения имеют alt/caption/size;
- PDF загружается, читается и индексируется;
- workflow доступен из editor/reader;
- admin разбит на масштабируемые страницы;
- все ключевые сценарии работают на 320 px и с клавиатуры;
- axe и visual regression являются обязательными CI checks;
- SUS не ниже 80 и task completion не ниже 90%;
- отсутствуют открытые P0/P1 UX-дефекты.

