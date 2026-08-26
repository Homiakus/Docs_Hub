# Docs_Hub — Anchored Comments, Markdown Presentations & AutoTrace Integration Plan

**Дата:** 2026-08-26  
**Docs_Hub baseline:** `main@f04bc33d31c635b9ffc776706a43931ab220e0f6`  
**AutoTraceLab baseline:** `main@69b4ebebe3b96295c8e39679130fa946669c5f7b`  
**Связанный master-plan:** [SecureAccess, Domains, Projects & Editor](SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md)

---

# 1. Цель

Добавить в целевую архитектуру Docs_Hub три связанные подсистемы:

1. **Anchored Comments** — комментарии к произвольному месту Markdown-текста с устойчивой привязкой при последующих редакциях.
2. **Presentation View** — возможность открыть тот же `.md`-документ как полноценную презентацию без отдельной копии контента.
3. **AutoTrace diagrams** — специальный Markdown-блок для сложных технических схем, который использует `Homiakus/autotraceLab` для ортогональной трассировки и отображения больших графов.

Общий принцип:

> один Markdown-source → несколько безопасных представлений: Document, Review, Presentation и Diagram.

Новые возможности не должны создавать отдельную систему документов, отдельный ACL или второй Markdown renderer.

---

# 2. Архитектурные инварианты

1. Markdown остаётся каноническим source of truth.
2. Все представления строятся из одной revision документа.
3. Comment, presentation и diagram endpoints проходят через SecureAccess и наследуют effective access документа.
4. Comment bodies не могут утекать через search/activity/notifications пользователю без `PermView` исходного документа.
5. `PermComment` проверяется SecureAccess, а не role string в Docs_Hub.
6. Presentation View никогда не расширяет audience документа.
7. AutoTrace input считается недоверенным пользовательским вводом: размер, количество nodes/edges, строки и вычислительный бюджет ограничены.
8. AutoTrace output проходит validation/sanitization до попадания в HTML.
9. Raw arbitrary HTML/CSS/JavaScript из Markdown-презентации не исполняется.
10. Никакой отдельной копии Markdown для presentation mode.

---

# 3. Anchored Comments — продуктовая модель

Пользователь выделяет любой фрагмент текста в Live Preview или Source editor и нажимает **Comment**.

Desktop:

```text
... выбранный фрагмент текста ...        ┌─────────────────────┐
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━       │ Alice · 14:32       │
                                          │ Нужно уточнить       │
                                          │ значение параметра.  │
                                          │                     │
                                          │ Reply…               │
                                          └─────────────────────┘
```

Mobile:

```text
selected text
    ↓
[Comment]
    ↓
bottom sheet thread
```

Поддержать:

- выделение части слова/предложения/нескольких абзацев;
- комментарий к block/heading без text selection;
- thread replies;
- mentions `@user`;
- resolve / reopen;
- edit/delete собственного сообщения по policy;
- deep-link к thread;
- previous/next unresolved comment;
- фильтр `All / Open / Resolved / Mine`;
- comments внутри Change Request;
- read-only отображение исторических comments на старой revision;
- notification event без отправки секретного текста документа в сторонние каналы.

---

# 4. Устойчивая привязка комментария

Один `start/end offset` недостаточен: после вставки текста выше комментарий съедет. Поэтому использовать **multi-selector anchor**, совместимый по идее с W3C Web Annotation Data Model.

Хранить одновременно:

```text
Document revision identity
+ TextPositionSelector
+ TextQuoteSelector
+ AST/block selector
+ local context hash
```

W3C рекомендует TextQuoteSelector (`exact`, `prefix`, `suffix`) для идентификации выбранного текста и отдельно определяет TextPositionSelector (`start`, `end`); позиционный selector сам по себе хрупок при изменениях ресурса. Reference: https://www.w3.org/TR/annotation-model/

## 4.1. Anchor model

```go
type CommentAnchor struct {
    DocumentID      int64
    BaseRevisionID  int64

    Start           int
    End             int

    Exact           string
    Prefix          string
    Suffix          string

    ASTNodeKind     string
    ASTPath         string
    HeadingStableID string
    BlockHash       string

    Status          AnchorStatus
}
```

Не хранить только DOM XPath: DOM является render projection и может меняться после обновления renderer.

## 4.2. Создание anchor

При выделении:

1. CodeMirror сообщает source offsets.
2. AST index находит enclosing block/heading.
3. сохраняются `start/end`;
4. сохраняется exact selected text;
5. сохраняются bounded prefix/suffix, например до 64–128 Unicode code points;
6. сохраняется AST path и stable heading/block identity;
7. сохраняется `BaseRevisionID`;
8. thread создаётся только после server-side проверки `PermComment` и `PermView`.

Unicode offsets определять в code points/grapheme-safe representation; не разрезать surrogate/grapheme clusters.

---

# 5. Re-anchoring после редактирования

При открытии новой revision система пытается восстановить anchor по каскаду от самого точного метода к менее точному.

```text
1. Same revision + exact position
        ↓ fail
2. Position mapped through revision diff
        ↓ fail
3. Exact quote + prefix/suffix
        ↓ ambiguous/fail
4. Same AST block + quote similarity
        ↓ fail
5. Heading + local context similarity
        ↓ fail
6. ORPHANED — ручное перепривязывание
```

## Правило безопасности/корректности

Если есть два равновероятных matching fragment, **не выбирать молча**.

Thread становится:

```text
Anchor needs attention
[Relocate]
[View original revision]
```

## 5.1. Diff mapping

При создании новой revision сохранять lightweight mapping старых text ranges → новых ranges.

Не требуется CRDT. Для последовательных document revisions достаточно deterministic diff mapping с fallback на quote/AST matching.

## 5.2. Anchor confidence

Internal score:

```text
EXACT
MAPPED
REANCHORED_HIGH
REANCHORED_LOW
ORPHANED
```

`REANCHORED_LOW` не должен визуально притворяться точным — UI показывает muted warning владельцу thread/reviewer.

---

# 6. Схема данных комментариев

Новые migration tables:

```sql
annotation_threads
  id
  document_id
  change_request_id NULL
  base_revision_id
  status            -- open | resolved
  anchor_status
  anchor_json
  created_by
  resolved_by NULL
  created_at
  resolved_at NULL
  updated_at

annotation_messages
  id
  thread_id
  author_id
  body_markdown
  body_rendered
  created_at
  updated_at
  deleted_at NULL

annotation_mentions
  message_id
  mentioned_user_id
  notification_state
```

`anchor_json` versioned:

```json
{
  "version": 1,
  "position": {"start": 412, "end": 458},
  "quote": {"exact": "...", "prefix": "...", "suffix": "..."},
  "ast": {"kind": "paragraph", "path": "h2:architecture/p:3", "blockHash": "..."}
}
```

Не встраивать comment marker непосредственно в Markdown. Комментарии являются collaborative metadata и не должны загрязнять переносимый `.md` source.

---

# 7. Comment API

```text
GET    /api/v1/documents/{id}/comments
POST   /api/v1/documents/{id}/comments
GET    /api/v1/comments/{thread}
POST   /api/v1/comments/{thread}/messages
PATCH  /api/v1/comments/{thread}/resolve
PATCH  /api/v1/comments/{thread}/reopen
PATCH  /api/v1/comments/{thread}/anchor
DELETE /api/v1/comment-messages/{id}
```

Change Request comments используют тот же engine:

```text
GET/POST /api/v1/changes/{id}/comments
```

Thread привязан к `working_revision` change request и при merge проходит re-anchor на новую published revision.

---

# 8. Comment permissions

| Action | Permission |
|---|---|
| Видеть thread | View document |
| Создать thread/reply | Comment |
| Resolve own discussion | Comment + policy |
| Resolve review thread | Review или thread owner по policy |
| Delete чужое сообщение | Manager/Admin policy |
| Export comments | Export/View policy, если export реализован |

SecureAccess остаётся authority.

Comment count в списках/search также является protected metadata и не показывается без View.

---

# 9. Comments UX в редакторе и reader

## 9.1. Editor

Selection toolbar расширяется:

```text
B  I  Code  Link  Comment
```

Shortcut:

```text
Ctrl/Cmd + Alt + M → Add comment
```

Комментарий не отбирает keyboard focus навсегда: после submit Escape/close возвращает focus в исходную selection.

## 9.2. Reader

Reader может выделить текст и комментировать без перехода в edit mode, если имеет `PermComment`.

Правое поле показывает compact markers, а не постоянно открытые карточки.

## 9.3. Review mode

В Review mode unresolved threads становятся более заметными и могут быть фильтром:

```text
Changes | Preview | Comments (7) | Overview
```

`Approve` может иметь configurable gate: например, запрещать approve при unresolved blocking threads.

Не все comments blocking. Добавить optional thread kind:

```text
comment
suggestion
blocking
```

`blocking` доступен Reviewer/Manager.

---

# 10. Presentation View — цель

Любой Markdown document может быть отмечен как presentation-capable и открыт:

```text
Document view
[Present]
```

Один source поддерживает:

```text
Reader view
Editor Live Preview
Presentation view
Print/PDF presentation
Speaker view
```

Не создавать `document.pptx` или вторую editable copy как source of truth.

---

# 11. Presentation Markdown syntax

Использовать **portable constrained syntax**, близкий к Marp/reveal.js, но рендерить через собственный Markdown AST pipeline Docs_Hub.

Marp использует YAML front matter/directives и разделяет slides горизонтальным ruler `---`; reveal.js Markdown также поддерживает separators и speaker-note delimiter. References:

- https://github.com/marp-team/marp/blob/main/website/docs/guide/directives.md
- https://github.com/marp-team/marp/blob/main/website/docs/guide/how-to-write-slides.md
- https://revealjs.com/markdown/
- https://revealjs.com/speaker-view/

## 11.1. Front matter

```markdown
---
title: HP4 Architecture
presentation:
  enabled: true
  theme: engineering-light
  ratio: 16:9
  transition: fade
  paginate: true
  headingDivider: 2
---
```

Allowed presentation fields строго whitelist-нуты.

Запретить user-provided arbitrary JavaScript и unrestricted CSS.

## 11.2. Slide boundary

Поддержать два способа:

### Explicit

```markdown
# First slide

---

# Second slide
```

### Heading divider

```yaml
presentation:
  headingDivider: 2
```

Тогда каждый H2 начинает новый slide.

`---` внутри fenced code block никогда не разделяет slides.

Front matter отделяется parser state и не считается slide separator.

## 11.3. Speaker notes

Canonical syntax:

```markdown
:::notes
Здесь приватные заметки докладчика.
Напомнить о результате испытаний.
:::
```

Presentation renderer превращает их в speaker notes.

В обычном Reader view `notes` скрыты по default и доступны только автору/reviewer по UI action.

При export можно выбрать:

```text
Slides only
Slides + notes
```

## 11.4. Fragments

Минимальный portable extension:

```markdown
- First <!-- fragment -->
- Second <!-- fragment -->
```

Parser распознаёт только whitelist directive comment. Остальные raw HTML comments не получают special execution semantics.

## 11.5. Slide metadata

```markdown
<!-- slide: class=section background=asset:architecture-cover -->
```

Whitelist:

- predefined `class`;
- managed asset background;
- transition enum;
- optional duration;
- speaker-only flags.

Никаких произвольных `style=` / `on*=`.

---

# 12. Presentation renderer architecture

Рекомендуемый pipeline:

```text
Markdown revision
      ↓
Goldmark + Docs extensions AST
      ↓
PresentationSplitter
      ↓
Slide AST[]
      ↓
Canonical sanitized HTML renderer
      ↓
Presentation shell
      ↓
Reveal.js runtime (embedded static assets)
```

Почему не делать Marp runtime обязательным:

- Docs_Hub уже имеет Go canonical renderer;
- runtime должен остаться one Go binary;
- Node нужен только build-time;
- одна sanitization policy должна работать и для reader, и для slides.

Reveal.js используется как **presentation interaction runtime**: navigation, overview, fragments, speaker notes, print/PDF behavior. Markdown parsing остаётся у Docs_Hub.

---

# 13. Presentation UX

Reader header:

```text
HP4 Architecture
[Edit] [Present ▷] [•••]
```

Presentation launch modes:

```text
Present here
Open fullscreen
Presenter view
Print / PDF
```

Presentation view:

- keyboard arrows/PageUp/PageDown/Space;
- touch swipe;
- overview grid;
- progress;
- slide number;
- presenter notes;
- current/next slide;
- timer;
- deep-link `#/slide-number`;
- Esc exits cleanly back to document.

Accessibility:

- focus states;
- semantic headings;
- reduced motion;
- keyboard complete;
- contrast-compliant themes;
- no information conveyed only by animation.

---

# 14. Presentation in editor

Editor modes become:

```text
Write | Preview | Slides
```

`Slides` показывает thumbnails слева и текущий slide preview справа.

Для presentation document slash palette добавляет:

```text
/new slide
/speaker notes
/fragment
/title slide
/two columns
/image slide
/diagram slide
```

Новая slide вставляется как Markdown separator/directive, а не proprietary block.

Outline умеет переключаться:

```text
Document outline
Slide outline
```

---

# 15. Presentation validation

Перед Present/Publish диагностировать:

- slide overflow;
- слишком мелкий computed font;
- слишком много строк/буллетов;
- broken image/attachment;
- failed Mermaid/AutoTrace diagram;
- missing alt/accessibility description;
- notes directive outside slide;
- duplicate slide IDs;
- unsafe directive;
- code/table overflow.

Warnings не должны блокировать обычный document save.

Errors, делающие presentation unsafe/unrenderable, блокируют Presentation Publish/Export, но source Markdown остаётся доступен.

---

# 16. AutoTrace — роль в Docs_Hub

Mermaid остаётся для простых диаграмм и sequence/flow charts.

**AutoTrace применяется для сложных схем**, где важны:

- большое количество blocks;
- ports/pins;
- ортогональная трассировка;
- минимизация пересечений;
- техническая читаемость;
- устойчивое положение маршрутов;
- интерактивный zoom/pan;
- отображение на document page и slide.

AutoTraceLab уже содержит importable Go core `github.com/Homiakus/autotraceLab/go_engine/core` с `RouteRequest`, nodes, ports, edges, routing options, metrics и contract version. Его `Route` валидирует scene и возвращает routed edges/metrics. Интеграция должна использовать **Go core**, а не React-приложение AutoTraceLab как iframe.

---

# 17. AutoTrace Markdown block

Добавить fenced code language:

```markdown
```autotrace
version: 1
flow: left-to-right
nodes:
  - id: controller
    title: Controller
    outputs: [modbus]
  - id: selector
    title: Selector
    inputs: [modbus]
    outputs: [motor]
  - id: motor
    title: Motor
    inputs: [motor]
edges:
  - from: controller.modbus
    to: selector.modbus
    label: RS-485
  - from: selector.motor
    to: motor.motor
    label: STEP/DIR
```
```

Это **Docs AutoTrace DSL v1**, а не прямой Go JSON contract.

Причина: автору документа не нужно задавать `x/y/width/height` и внутренние engine fields вручную.

---

# 18. AutoTrace DSL compiler

Pipeline:

```text
```autotrace source
       ↓
strict YAML/compact DSL parser
       ↓
Docs diagram IR
       ↓
layout stage
       ↓
AutoTrace core BlockNode/Port/EdgeConnection
       ↓
core.Route / SceneEngine
       ↓
route validation
       ↓
SVG scene model
       ↓
sanitized interactive viewer
```

## 18.1. Docs diagram IR

```go
type Diagram struct {
    Version int
    Flow    FlowDirection
    Nodes   []DiagramNode
    Edges   []DiagramEdge
    Options DiagramOptions
}
```

DSL compiler отвечает за:

- stable IDs;
- default block dimensions;
- default ports;
- initial layered positions;
- style tokens → allowed AutoTrace fields;
- limits;
- diagnostics с source line/column.

## 18.2. Layout before routing

Текущий AutoTrace `core.Route` ожидает geometry `X/Y/Width/Height`; поэтому Docs adapter должен либо:

1. использовать стабильный layout stage до Route;
2. либо после развития AutoTraceLab перейти на официальный layout+route API.

Не скрывать этот контракт внутри Markdown renderer.

Добавить в AutoTraceLab integration backlog отдельный stable API:

```go
LayoutAndRoute(SceneSpec) (Scene, error)
```

когда AutoTrace core будет готов сделать layout production contract.

---

# 19. AutoTrace package boundary

Новый пакет Docs_Hub:

```text
internal/diagram/autotrace/
  parser.go
  ir.go
  compiler.go
  layout.go
  engine.go
  renderer.go
  cache.go
  limits.go
  diagnostics.go
```

`engine.go` — единственное место, импортирующее AutoTrace core.

Остальной Docs_Hub зависит от собственного `DiagramEngine` interface:

```go
type DiagramEngine interface {
    Render(ctx context.Context, source string, opts RenderOptions) (RenderedDiagram, error)
}
```

Это позволяет обновлять AutoTrace отдельно и тестировать adapter fixtures.

---

# 20. AutoTrace versioning

Не зависеть от плавающего `main`.

До production integration в `autotraceLab`:

1. довести Go Core production path согласно его Pareto plan;
2. выпустить semver tag для `go_engine` module;
3. зафиксировать `ContractVersion`;
4. добавить compatibility tests Docs_Hub ↔ AutoTrace;
5. pin exact module version в `go.mod`.

При несовместимом engine contract Docs_Hub должен показать диагностируемый fallback, а не panic.

---

# 21. AutoTrace rendering

Server генерирует **scene model**, browser рисует SVG.

Не хранить generated SVG как authoritative content.

Cache key:

```text
sha256(
  source DSL
  + AutoTrace engine version
  + diagram adapter version
  + theme geometry version
  + effective routing options
)
```

Кэш содержит validated scene/render projection.

Viewer:

- responsive SVG;
- fit-to-view;
- zoom/pan;
- reset;
- fullscreen;
- keyboard zoom;
- optional minimap для больших схем;
- node/edge tooltip;
- searchable node labels;
- copy diagram as SVG/PNG only if permitted;
- static print mode.

На slide viewer AutoTrace должен автоматически fit в safe slide area без пересчёта topology при каждом resize; topology считается в logical coordinates, browser масштабирует viewport.

---

# 22. AutoTrace editor UX

Slash command:

```text
/diagram
  Mermaid — simple diagram
  AutoTrace — complex technical diagram
```

При вставке AutoTrace block показывать side-by-side modal/editor:

```text
DSL source        Live diagram
```

Для новичка добавить form-assisted builder:

- Add block;
- Add port;
- Connect;
- label;
- flow direction;
- Apply.

Form всё равно генерирует readable DSL в Markdown.

Power user редактирует DSL напрямую.

Diagnostics связываются с строкой source, например:

```text
Line 14: edge references unknown port `motor.step`
```

---

# 23. AutoTrace limits / DoS protection

Diagram rendering CPU-bound, поэтому нужны hard bounds.

Начальные configurable limits, уточняемые benchmark corpus:

```text
max source bytes
max nodes
max ports per node
max edges
max label length
max routing time
max generated points
max cache entry bytes
```

При превышении:

```text
Diagram is too complex to render interactively.
[View source]
[Open diagnostics]
```

Никакого бесконечного server request.

Для тяжёлых схем разрешить background-free synchronous bounded processing только в рамках request budget; если нужен job runner в будущем — это отдельное evidence-driven решение.

---

# 24. AutoTrace fallback strategy

```text
AutoTrace render success
    → interactive diagram

AutoTrace route/layout failure
    → diagnostic card + source

AutoTrace package unavailable/version mismatch
    → static safe error state
```

Не fallback-ить автоматически на Mermaid: DSL и semantics различаются, а такой fallback может нарисовать неправильную схему.

Для previously cached validated scene допускается stale-cache fallback только если source hash и engine contract совместимы, с явной diagnostic metadata.

---

# 25. AutoTrace + security

AutoTrace diagram является частью документа.

Следовательно:

- viewer доступен только при `PermView` документа;
- export требует соответствующий permission;
- DSL не отправляется во внешние API;
- labels/node metadata не попадают в публичную telemetry;
- diagram cache keyed с document/security-independent content hash, но выдача cache entry всё равно происходит только после document authorization;
- direct cache URL отсутствует.

---

# 26. Presentation + AutoTrace

AutoTrace block внутри slide работает без отдельной разметки:

```markdown
## System topology

```autotrace
...
```
```

Presentation renderer получает diagram scene до slide layout.

Правила:

- max diagram occupies bounded slide region;
- fit-to-view по logical bounds;
- interactive zoom отключён в обычном audience mode, чтобы scroll/gesture не ломали navigation;
- presenter может открыть diagram focus/fullscreen;
- print/PDF получает deterministic static SVG;
- animation edge-by-edge пока не входит в P0/P1.

---

# 27. Comments + Presentation

Audience presentation **не показывает collaborative comments**.

Presenter/Reviewer может открыть private overlay:

```text
Speaker notes
Review comments
```

Но comment body никогда не попадает в audience DOM, если overlay не запрошен и пользователь не имеет нужного access.

Для slide-specific review comment anchor использует underlying Markdown text anchor, а не slide pixel coordinates.

Так comment переживает смену theme/ratio.

---

# 28. File map

## Comments

```text
internal/domain/annotation.go                         NEW
internal/application/comment_service.go              NEW
internal/repository/comment_repository.go            NEW
internal/repository/sqlite/comment_repo.go            NEW
internal/annotation/anchor.go                         NEW
internal/annotation/reanchor.go                       NEW
internal/annotation/diffmap.go                        NEW
internal/httpapp/handlers/comments.go                 NEW
internal/web/static/js/editor/comments/*              NEW
internal/web/static/css/components/comments.css       NEW
```

## Presentations

```text
internal/presentation/model.go                        NEW
internal/presentation/parser.go                       NEW
internal/presentation/splitter.go                     NEW
internal/presentation/validator.go                    NEW
internal/httpapp/handlers/presentation.go             NEW
internal/web/templates/presentation.html              NEW
internal/web/static/vendor/reveal/*                    NEW/PINNED
internal/web/static/js/presentation/*                 NEW
internal/web/static/css/components/presentation.css   NEW
```

## AutoTrace

```text
internal/diagram/engine.go                            NEW
internal/diagram/autotrace/parser.go                  NEW
internal/diagram/autotrace/ir.go                      NEW
internal/diagram/autotrace/compiler.go                NEW
internal/diagram/autotrace/layout.go                  NEW
internal/diagram/autotrace/engine.go                  NEW
internal/diagram/autotrace/renderer.go                NEW
internal/diagram/autotrace/cache.go                   NEW
internal/diagram/autotrace/limits.go                  NEW
internal/diagram/autotrace/diagnostics.go             NEW
internal/web/static/js/diagrams/autotrace-viewer.js   NEW
internal/web/static/css/components/diagram.css        NEW
```

---

# 29. SQL migrations

Продолжить после ранее зарезервированных migrations:

```text
016_annotation_threads.sql
017_annotation_messages.sql
018_presentation_metadata.sql       // только server-side metadata/cache refs
019_diagram_render_cache.sql        // optional; предпочтительно content-addressed cache metadata
```

Presentation source directives находятся в Markdown, поэтому отдельная таблица slides не нужна.

AutoTrace source также находится в Markdown; БД не становится вторым source of truth.

---

# 30. API additions

## Comments

```text
GET    /api/v1/documents/{id}/comments
POST   /api/v1/documents/{id}/comments
POST   /api/v1/comments/{id}/messages
PATCH  /api/v1/comments/{id}/resolve
PATCH  /api/v1/comments/{id}/reopen
PATCH  /api/v1/comments/{id}/anchor
```

## Presentation

```text
GET    /documents/{id}/present
GET    /api/v1/documents/{id}/presentation
POST   /api/v1/documents/{id}/presentation/validate
GET    /api/v1/documents/{id}/presentation/print
```

## Diagrams

Prefer server-rendered page pipeline. Для editor live preview:

```text
POST   /api/v1/preview/diagram/autotrace
```

Request содержит только DSL source + presentation/document rendering context, но не security Workspace ID от клиента.

---

# 31. Testing — comments

## Functional

- comment exact selected word;
- comment multi-paragraph range;
- reply;
- resolve/reopen;
- mention;
- deep link;
- deletion/edit;
- comment on draft;
- comment on published reader view;
- comment inside change request;
- merge change request with comments.

## Re-anchor corpus

Fixtures:

1. insertion before selection;
2. deletion before selection;
3. edit inside selection;
4. duplicated exact phrase;
5. moved paragraph;
6. heading renamed;
7. paragraph split;
8. paragraphs merged;
9. list reordered;
10. Unicode/emoji;
11. code block edit;
12. complete deletion of selected fragment.

Acceptance:

- no silent attachment to wrong text;
- deterministic result;
- orphaned state instead of ambiguous guess;
- original revision always recoverable.

## Security

- no View → comments invisible;
- View without Comment → cannot create;
- revoked user next request → deny;
- restricted project → thread metadata not leaked;
- notification never reveals comment/document snippet to unauthorized recipient.

---

# 32. Testing — presentations

- front matter parse;
- explicit separators;
- headingDivider;
- separator inside code block;
- notes;
- fragments;
- image slides;
- tables/code overflow;
- Mermaid;
- AutoTrace;
- reader → presentation → reader navigation;
- fullscreen;
- presenter view;
- print/PDF CSS;
- direct presentation URL ACL;
- restricted project/document ACL;
- CSP/sanitization;
- reduced motion;
- keyboard-only operation;
- mobile landscape.

Snapshot tests фиксируют slide count, semantic tree и safe rendered HTML, а не browser-specific pixels для каждого текста.

---

# 33. Testing — AutoTrace

## Contract

- pin AutoTrace module version;
- assert `ContractVersion`;
- native deterministic fixtures;
- invalid IDs/ports;
- malformed DSL;
- empty graph;
- duplicate IDs;
- non-finite/overflow values rejected compiler-side.

## Quality

- 10 / 50 / 100 / 300 nodes;
- fan-in/fan-out;
- dense ports;
- narrow channels;
- crossing pressure;
- long labels;
- slide-size aspect ratios;
- resize without topology change.

Hard gate:

- invalid routed path is never rendered as valid technical diagram;
- engine error never crashes document/presentation page;
- diagram render cannot bypass request timeout/budget.

## Performance

Measure:

```text
DSL parse
layout
routing
SVG scene serialization
browser first paint
zoom/pan FPS
cache hit/miss
```

Budget thresholds установить только после baseline на AutoTrace benchmark corpus.

---

# 34. Revised implementation phases

Эти этапы вставляются в основной master-plan после базового editor engine и до legacy cleanup.

## Phase 6A — Editor foundation

Существующий DH editor plan без изменений: CodeMirror, Markdown AST, Live Preview, slash, wiki links, attachments.

## Phase 6B — Anchored comments

1. W3C-inspired selector model;
2. migrations;
3. repository/service;
4. CodeMirror selection → source anchor;
5. reader selection → source anchor mapping;
6. thread UI;
7. reply/resolve/reopen;
8. mentions;
9. revision diff mapping;
10. re-anchor engine;
11. orphan relocation UX;
12. change-request integration;
13. security/leak tests.

**Gate:** ни один test fixture не привязывает thread молча к неправильному фрагменту.

## Phase 6C — Presentation parser/runtime

1. presentation front matter schema;
2. AST slide splitter;
3. notes/fragments whitelist extensions;
4. safe slide renderer;
5. embedded Reveal runtime;
6. viewer/presenter/print modes;
7. editor Slides tab;
8. slide diagnostics;
9. mobile landscape;
10. ACL tests.

## Phase 6D — AutoTrace contract preparation

В `autotraceLab`:

1. завершить Go Core production convergence из AutoTrace Pareto plan;
2. published semver for `go_engine`;
3. stable ContractVersion policy;
4. expose/confirm validated route API;
5. optional production `LayoutAndRoute` API или documented geometry contract;
6. benchmark/correctness release gate.

## Phase 6E — AutoTrace Docs adapter

1. `DiagramEngine` interface;
2. DSL v1;
3. strict parser;
4. layout adapter;
5. pinned AutoTrace core;
6. SVG scene renderer;
7. cache;
8. diagram editor preview;
9. reader integration;
10. presentation integration;
11. export/print;
12. complexity budgets;
13. E2E/performance/security gates.

---

# 35. Revised PR sequence

Добавить к ранее определённому набору:

```text
DH-11A anchored-comment schema + service
DH-11B comment editor/reader UX
DH-11C re-anchor engine + change-request integration
DH-12A presentation AST + syntax
DH-12B presentation runtime + presenter/print UX
AT-01 stable Go Core module/contract release
AT-02 optional LayoutAndRoute contract + quality gate
DH-12C AutoTrace DSL + adapter
DH-12D AutoTrace viewer + editor preview
DH-12E AutoTrace presentation/export integration
```

Нумерацию последующих DH PR можно сдвинуть; важнее зависимости:

```text
Editor AST
   ├── Comments
   ├── Presentation
   └── AutoTrace

SecureAccess
   └── protects all three
```

---

# 36. UX acceptance criteria

## Comments

- комментарий к выделенному тексту создаётся максимум за 2 действия;
- thread виден рядом с фрагментом, но не закрывает writing canvas;
- после обычных соседних редакций комментарий остаётся на правильном тексте;
- ambiguous anchor никогда не перепривязывается скрытно;
- unresolved comments доступны одной keyboard command/navigation;
- mobile comment thread не уменьшает editor width до непригодного состояния.

## Presentation

- существующий Markdown превращается в deck без копирования документа;
- автор может создать новую slide через `/new slide`;
- `Present` запускается одним действием;
- speaker notes не видны audience;
- print/PDF deterministic;
- любой slide URL соблюдает тот же ACL, что документ.

## AutoTrace

- сложная схема вставляется как readable Markdown fenced block;
- обычному автору не нужно вручную рассчитывать coordinates;
- preview показывает diagnostic line, а не generic 500;
- большие схемы можно zoom/pan;
- presentation автоматически fit-ит diagram;
- invalid route не отображается как корректная инженерная схема.

---

# 37. Definition of Done extension

К исходному Definition of Done master-plan добавить:

17. Пользователь может поставить thread comment на произвольный текстовый range в editor и reader.
18. Anchors устойчивы к типичным редакциям и переходят в explicit orphan state при неоднозначности.
19. Comments полностью наследуют document security и используют SecureAccess `PermComment`.
20. Один `.md` source может быть открыт как presentation без fork контента.
21. Presentation syntax имеет versioned safe subset, speaker notes, slide boundaries и fragments.
22. Presentation использует canonical Docs Markdown renderer и embedded presentation runtime, а не второй Markdown implementation.
23. Presentation direct URLs/print/presenter modes не обходят ACL.
24. Docs поддерживает fenced `autotrace` blocks для сложных technical diagrams.
25. AutoTrace интегрирован через pinned importable Go Core contract, а не iframe/React application coupling.
26. AutoTrace DSL strict/versioned, render bounded, failures diagnostic/fail-safe.
27. AutoTrace diagrams одинаково работают в Reader, Editor preview, Presentation и Print.
28. Comment/presentation/diagram regression tests входят в blocking CI matrix.

---

# 38. Итоговая продуктовая модель

```text
Organization
└── Domain                         SecureAccess boundary
    └── Project                    inherited/restricted access
        └── Markdown Document      one canonical source
            ├── Reader View
            ├── Live Editor
            │   └── Anchored Comments
            ├── Change Request / Review
            │   └── Anchored Comments
            ├── Presentation View
            │   ├── Speaker Notes
            │   └── AutoTrace / Mermaid diagrams
            └── Print / Export
```

Итоговая цель: Docs_Hub должен позволять **писать, обсуждать, ревьюить, показывать и объяснять техническую информацию из одного Markdown-документа**, сохраняя предсказуемый доступ, переносимость source и инженерную корректность сложных схем.