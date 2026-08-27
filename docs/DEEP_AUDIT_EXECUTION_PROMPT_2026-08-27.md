# Execution Prompt — Docs_Hub Deep Audit Remediation

Используй этот промпт для последовательной реализации:

`docs/DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md`

---

## PROMPT

Ты — ведущий системный архитектор, Go engineer, application-security engineer, database engineer и test/reliability engineer. Ты работаешь непосредственно с репозиторием:

`https://github.com/Homiakus/Docs_Hub`

Твоя задача — **полностью и поэтапно реализовать** план:

`docs/DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md`

Главная цель — довести Docs_Hub до доказуемо безопасной архитектуры `Organization -> Domain -> Project -> Document`, где tenant isolation, membership, project restrictions и query scope являются реальными runtime invariants, а не только моделью данных или UI.

### 1. Главный источник истины

1. Перед началом прочитай весь `DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md`.
2. Затем изучи связанные документы, но не позволяй старым `[ВЫПОЛНЕНО]` противоречить текущему коду:
   - `docs/ROADMAP.md`;
   - `Docs_Hub_Enterprise_Roadmap.md`;
   - `docs/PARETO_IMPLEMENTATION_PLAN_2026-08-26.md`;
   - `docs/SECUREACCESS_DOMAINS_PROJECTS_EDITOR_PLAN_2026-08-26.md`;
   - `docs/COLLAB_PRESENTATIONS_AUTOTRACE_PLAN_2026-08-26.md`;
   - `docs/MARKDOWN_EDITOR_MASTER_SPEC.md`.
3. Текущий runtime, тесты и фактический Git state всегда важнее устаревшей отметки roadmap.
4. Если находишь новое существенное несоответствие, сначала добавь его в remediation plan с приоритетом, evidence и acceptance criteria, затем исправляй.

### 2. Работай строго по фазам

Порядок нельзя переставлять без доказанной технической причины:

```text
P0.0 Green baseline / CI
P0.1 Trusted Principal / tenant identity
P0.2 Project access boundary
P0.3 Comments authorization/invariants
P0.4 Session hardening
P0.5 Data/model invariants
P1.1 Query authorization invariant
P1.2 HTTP -> Application convergence
P1.3 SecureAcces parity gate
P1.4 Markdown/Slides hardening
P1.5 AutoTrace adapter
P1.6 Repository governance cleanup
P2.x Product/UX/observability/performance
P3 Release hardening
```

Не переходи к следующей фазе, пока acceptance criteria текущей не доказаны тестами.

### 3. Security rules — обязательны

Всегда соблюдай:

- fail closed;
- никаких production `OrganizationID: 1` или аналогичных tenant fallback;
- global local role не заменяет organization membership;
- `restricted` Project действительно разрывает inheritance;
- authorization выполняется до SQL ranking/aggregation/`LIMIT`;
- security-service error не превращается в allow;
- HTTP handler не принимает security decision самостоятельно;
- client-controlled organization/workspace ID никогда не считается доверенным без server-side validation;
- comment mutations требуют permission к связанному document/project;
- repository не скрывает policy logic внутри persistence methods;
- restart процесса не должен менять permission semantics;
- не создавать второй долгоживущий security authority внутри Docs_Hub, если ответственность принадлежит SecureAcces.

Для каждого security bug сначала создай тест, воспроизводящий нарушение. Убедись, что тест падает до fix, затем исправь код и добейся зелёного результата.

### 4. Архитектурное правило

Целевая зависимость:

```text
HTTP/Web/Bot
    -> Application
        -> Domain + Ports
            -> Repository adapters
            -> SecureAcces/security adapters
```

Запрещено добавлять новый business SQL или трактовку permissions непосредственно в HTTP handlers.

Сохраняй модульный монолит. Не вводи микросервисы без отдельной доказанной необходимости.

### 5. Миграции

- Никогда не изменяй уже существующие миграции `001`–`010` задним числом.
- Новые изменения схемы начинай с `011_...` или следующего свободного номера.
- Для каждой migration добавляй upgrade/integration test.
- Для destructive изменений используй expand -> backfill -> verify -> contract.
- Не выполняй contract, пока consistency/parity tests не доказали отсутствие legacy anomalies.
- Перед необратимыми изменениями должна существовать backup/rollback стратегия.

### 6. Testing gate каждого этапа

Минимум запускай релевантные targeted tests и затем общий gate:

```bash
go test -race ./...
go vet ./...
govulncheck ./...
```

Также запускай релевантный Playwright/E2E matrix.

Для изменений security обязательно должны быть отрицательные тесты, например:

```text
cross-organization deny
restricted project deny
revoked membership deny
metadata excluded from search/graph/backlinks
comment mutation deny
security backend failure -> deny
```

Не исправляй красный CI удалением assertions, исключением browsers или бессмысленным увеличением timeout.

### 7. Самодокументирование

После каждого завершённого phase обнови раздел `Progress ledger` в:

`docs/DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md`

Для завершённой фазы обязательно запиши:

```text
Status: DONE
Commit: <SHA>
Evidence:
- конкретные test commands
- targeted test names
- CI run/check
Residual risks: ...
```

Нельзя ставить `[x]` или `DONE` только потому, что код «выглядит готовым».

Если обнаружен новый риск, добавь его в plan до перехода дальше.

### 8. Git workflow

После каждого **полностью успешного** этапа:

1. убедись, что рабочее дерево содержит только изменения этого этапа;
2. обнови plan/evidence;
3. сделай осмысленный атомарный commit;
4. доставь результат в `main`;
5. если repository policy позволяет direct push — push в `main`;
6. если protection требует PR — создай/обнови PR и добейся merge в `main` после зелёных required checks;
7. никогда не обходи красные required checks ради merge;
8. после landing заново проверь SHA `main` и CI.

Не объединяй огромный security refactor, UI redesign и migration в один commit.

### 9. Контекст и автономная работа

После каждого успешно завершённого и landed этапа:

1. сожми рабочий контекст до короткой структурированной сводки;
2. сохрани в сводке:
   - что реализовано;
   - commit SHA;
   - какие tests прошли;
   - какие файлы изменены;
   - какие invariants теперь доказаны;
   - следующий незакрытый phase;
   - известные residual risks;
3. перечитай соответствующий следующий раздел master-plan;
4. продолжай самостоятельно без запроса подтверждения, пока нет реального внешнего blocker.

Не повторяй уже решённые вопросы и не трать контекст на длинный пересказ истории.

### 10. Stop conditions

Останови продвижение к новым фичам и исправь проблему немедленно, если обнаружено хотя бы одно:

- cross-tenant read/write;
- restricted content попадает в search/graph/autocomplete;
- security backend error приводит к allow;
- миграция может потерять данные;
- permission меняется после restart из-за process-local state;
- comment/document mutation возможна только по известному ID без resource permission;
- CI после этапа красный;
- для «исправления» требуется ослабить security regression test.

Если внешний dependency действительно блокирует фазу, не симулируй завершение. Зафиксируй blocker в master-plan, реализуй всё возможное до границы dependency и переходи только к независимой работе, которая не нарушает порядок security gates.

### 11. Pareto focus

Если приходится выбирать, в первую очередь обеспечь:

```text
1. green deterministic CI
2. trusted tenant principal
3. membership-aware AccessScope
4. real restricted Project enforcement
5. comment authorization
6. session entropy + idle timeout
7. indirect metadata leak protection
8. architectural convergence
9. SecureAcces parity
```

UI polish и новые product features не имеют приоритета над этими guarantees.

### 12. Формат отчёта после каждого этапа

Отчёт должен быть коротким и доказательным:

```text
Phase: P0.x — <name>
Status: DONE | BLOCKED
Main SHA: <sha>
Changed: <ключевые файлы>
Fixed invariants: <что теперь гарантируется>
Tests: <конкретные команды/результат>
CI: <run/check result>
Plan updated: yes
Residual risk: <none или конкретно>
Next: P0.x — <next>
```

Не писать расплывчатое «всё готово» без evidence.

---

## Критерий завершения всей работы

Работа считается законченной только когда:

- все P0 и P1 закрыты evidence-backed status;
- SecureAcces является единственным реальным security authority или явно зафиксирован проверенный transition state без allow-fallback;
- нет hardcoded tenant identity;
- restricted project изолирован во всех direct и indirect surfaces;
- comments/session security regressions закрыты;
- query authorization выполняется до ranking/`LIMIT`;
- `go test -race ./...`, `go vet ./...`, `govulncheck ./...` и E2E gate зелёные;
- migrations/invariants доказаны upgrade tests;
- старые roadmap statuses синхронизированы с реальностью;
- stale Domain/Project PR history очищена;
- `main` содержит финальный implementation и документацию;
- `DEEP_AUDIT_REMEDIATION_MASTER_PLAN_2026-08-27.md` содержит полный ledger SHA/evidence.

Начинай с проверки актуального `main`, текущего CI и `P0.0`. Не перескакивай через security phases.
