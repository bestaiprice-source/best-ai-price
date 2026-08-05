# Статус реализации ASH ТЗ и план доведения до 100%

Дата аудита: 2026-08-05

Проверяемые документы:

- `ASH-Ideal-Project-TZ.md`
- `Claude-Pool-Master-TZ.md`

## Краткий вывод

Оба ТЗ **не реализованы полностью**. Текущая реализация содержит основной MVP-контур ASH, отдельный Yunwu engine, scheduler, virtual sessions, leases, usage accounting, health/cooldown, rebinding, статистику и часть persistence. Однако ряд обязательных инвариантов fair-share, recovery, degradation и конкурентного accounting пока не выполнен или не доказан тестами.

Ориентировочная оценка текущего состояния:

| Область | Готовность |
|---|---:|
| Краткое `ASH-Ideal-Project-TZ.md` | ~78% |
| Полное `Claude-Pool-Master-TZ.md` | ~65% |
| Standard Anthropic/OpenAI fair-share | ~70% |
| Yunwu monetary fair-share | ~65% |
| Изоляция Yunwu от standard | ~82% |
| Общая production-ready готовность | ~60–65% |

Эти проценты являются инженерной оценкой, а не доказательством соответствия. Статус «100%» допускается только после выполнения всех пунктов ниже и прохождения полного набора проверок.

## Что уже реализовано

### Общий ASH-контур

- Отдельный пакет `backend/pkg/hypervisor` с session, lease, scheduler, runtime, compiler, storage, events, health, rebinding, quota, stats и backup компонентами.
- `VirtualSession` и временные leases на физические nodes.
- Scheduler по health/runtime признакам, affinity и priority.
- Ограниченный retry и standard rebinding с Redis session lock.
- Учёт usage и append-only событий.
- Health controller, cooldown и системные состояния деградации.
- Admin/user endpoints статистики и управления nodes.
- Feature flags и mock runtime.
- Хеширование пользовательских API keys и шифрование upstream credentials.

### Standard pool

- Первый запрос регистрирует пользователя active до вычисления fair-share.
- Общий тариф `standard` и отдельные provider scopes `standard:anthropic` и `standard:openai`.
- Общий `MonthlyLimit` нельзя обойти чередованием Anthropic/OpenAI в Redis hot path.
- Provider usage учитывается раздельно для динамического fair-share.
- Active-user window и priority weights участвуют в вычислении доли.
- Истёкшие active users удаляются из ZSET вместе с priority hash entries.

### Yunwu

- Запросы `platform=yunwu` направляются в отдельный `YunwuHypervisor`.
- Отдельные Yunwu scheduler, rebinding manager, balance sync и quota namespace.
- Production nodes выбираются из Yunwu accounts, Redis fallback ограничен test mode.
- API balance является основным источником capacity; legacy credit keys больше не читаются в рабочем коде.
- Есть primary `credit_grants` endpoint и fallback `user/self`.
- Есть fresh/stale snapshots, reserve и estimated available balance.
- Перед upstream вызовом создаётся атомарная node-level reservation.
- После успешного ответа reservation reconciles с billed micro-USD.
- Стоимость округляется вверх и использует раздельные input/output prices.
- Пользовательская Yunwu quota хранится в micro-USD.

### Persistence

- `ash_usages` расширена полями pool/provider/model/usage unit/billed micro-USD.
- Добавлена политика пользователя `ash_user_policies` с ключом `(owner_id, pool_type)`.
- Admin quota policy сначала сохраняется в PostgreSQL, затем кэшируется в Redis.
- Monthly Yunwu monetary usage может восстанавливаться из PostgreSQL.

## Критические незавершённые требования

### P0 — корректность accounting и fair-share

1. **Standard usage записывается как legacy.**

   Standard hypervisor вызывает старый `RecordUsage`, который сохраняет `pool_type=legacy` и `provider=legacy`. Восстановление standard counters фильтрует `pool_type=standard`, поэтому после очистки Redis monthly/provider usage может восстановиться как ноль.

   Требуется:

   - использовать `RecordUsageDetails(..., "standard", actualProvider, model, "tokens", 0)`;
   - добавить restart/cache-miss тест общего и provider usage;
   - проверить monthly rollover.

2. **User quota check и consumption не атомарны.**

   Несколько параллельных запросов могут одновременно пройти quota check, после чего суммарно превысить monthly limit, daily burst или динамическую fair-share долю.

   Требуется:

   - предварительно резервировать user quota атомарным Lua/transaction механизмом;
   - одновременно проверять shared standard cap, provider fair-share и daily burst;
   - на success reconciliate estimate с actual tokens;
   - на ошибке до billable response возвращать reservation;
   - обеспечить crash/TTL cleanup без зависших counters.

3. **Yunwu user-level monetary quota не резервируется.**

   Сейчас атомарно резервируется node balance, но индивидуальные monthly/daily micro-USD limits проверяются отдельно и без estimated request amount.

   Требуется:

   - одной атомарной операцией резервировать node balance и user monthly/daily allowance либо использовать согласованный двухфазный протокол с rollback;
   - запретить overspend при параллельных запросах;
   - добавить тесты race/parallel reservations на user limits.

4. **Истёкшая Yunwu reservation оставляет reserved counter.**

   Reservation key имеет TTL, но отдельный aggregate `reserved` автоматически не уменьшается после expiry.

   Требуется:

   - хранить expiry index и amounts;
   - атомарно очищать expired reservations;
   - пересчитывать reserved amount перед scheduler/API sync;
   - протестировать timeout, process crash и повторный API sync.

5. **Yunwu rebinding нарушает accounting.**

   Reservation создаётся для первоначального node. После rebinding retry может выполняться на другом node, но reservation/reconciliation остаются привязаны к старому node.

   Требуется:

   - перед retry вернуть reservation старого node;
   - проверить fresh balance и атомарно зарезервировать стоимость на новом node;
   - reconciliate именно новый node;
   - записать безопасные reservation/reconciliation events;
   - добавить тест отказа первого node и успешного billable retry второго.

### P0 — scheduler и capacity

6. **Yunwu rebinding выбирает первый node без balance-aware scheduler.**

   Alternative node фильтруется только по state/cooldown. Unknown, stale, reserve-exhausted или zero-balance node может быть выбран для retry.

   Требуется использовать тот же balance-aware scheduler и те же hard constraints, что при первоначальном выборе.

7. **Cooldown Yunwu может быть проигнорирован при единственном node.**

   Текущая логика исключает cooldown node только когда nodes больше одного. По требованиям cooldown, unknown/stale/zero/reserve-exhausted никогда не должны ослабляться.

8. **Standard capacity не доказанно учитывает только healthy nodes.**

   Quota capacity считает active/schedulable accounts, но не связывает их строго с актуальным health runtime state. Unhealthy/cooldown account может завысить доступную fair-share capacity.

   Требуется:

   - учитывать только подтверждённо healthy/schedulable nodes соответствующего provider;
   - исключать unknown/unavailable/cooldown;
   - инвалидировать capacity cache на полном lifecycle account и health transitions;
   - сохранить явные ошибки `no healthy anthropic nodes` и `no healthy openai nodes`.

9. **Capacity standard остаётся условной константой.**

   `NodeCapacity` задаёт одинаковую token capacity для каждого account. Это допустимо только как явно документированная модель service units, но не как доказанный реальный remaining budget Claude Max.

   Требуется определить и зафиксировать authoritative capacity model, rollover и способ калибровки cost units.

### P1 — degradation, queue и recovery

10. **Priority queue не завершена как обязательный механизм.**

    ТЗ требует queue/throttling в WARNING/DEGRADED и обязательную очередь в SURVIVAL. Наличие priority weights и concurrent limit не заменяет очередь.

11. **Degradation policy реализована частично.**

    Необходимо доказать отдельными тестами поведение NORMAL, WARNING, DEGRADED, SURVIVAL и RECOVERY, включая admission по priority, short-request restriction и постепенный возврат nodes.

12. **Recovery после cooldown не является постепенным.**

    Требуется low-load probe/half-open состояние, повторное увеличение cooldown при рецидиве и защита от мгновенного полного возврата нагрузки.

13. **Backup/recovery не покрывает весь Master-ТЗ.**

    Нужно проверить snapshot policy по действиям/времени, project files/git/dirty state/tool state, non-blocking streaming и полноценный recovery flow.

14. **Summary/context lifecycle требует полного acceptance покрытия.**

    Нужны доказательства async/non-blocking summary, сохранения recent raw messages, failure handling и стабильного compiler order для всех полей Master-ТЗ.

### P1 — lifecycle, config и sync

15. **Нет доказанной единой immutable config instance для всех компонентов/API.**

    Некоторые handlers создают новые quota managers, которые повторно загружают env. Требуется передавать один config через dependency graph.

16. **Balance sync lifecycle требует доработки.**

    Нужно:

    - initial sync active/schedulable Yunwu accounts до доступности;
    - managed application context/cancel;
    - исключить unschedulable accounts из periodic sync;
    - сохранить last-good snapshot при ошибке;
    - явно перевести snapshot в stale по configured deadline.

17. **Cache invalidation не доказана для полного account lifecycle.**

    Нужны create/update/status/platform/schedulable/delete/bulk-update и health transition тесты для Anthropic, OpenAI и Yunwu capacity caches.

18. **Использование Redis `KEYS` для Yunwu pool capacity.**

    Требуется заменить на индексированный набор node IDs/SCAN либо согласованный агрегат, чтобы расчёт не блокировал Redis и не включал orphan snapshots.

### P1 — pricing, API и observability

19. **Pricing resolution требует строгих тестов ошибок.**

    Должен быть доказан порядок exact vendor ID → normalized base URL → configurable fallback. Decrypt/query error не должен считаться совпадением.

20. **Stats/API contract покрыт не полностью.**

    Проверить раздельные token/micro-USD totals, aggregate allocatable balance, reserve percent, oldest/newest sync, balance status и отсутствие credentials во всех ответах.

21. **События и логи требуют security audit.**

    Нужен автоматический тест, что sync/stale/reservation/reconciliation events и logs никогда не содержат API keys, Authorization headers или decrypted credentials.

22. **Метрики требуют полного contract теста.**

    Подтвердить fresh/stale/unknown nodes, allocatable/reserved micro-USD, sync success/failure, reservation/reconciliation и fallback pricing, а также все базовые метрики Master-ТЗ.

### P2 — полное соответствие Master-ТЗ

23. Проверить и при необходимости завершить обязательные события, включая `SessionClosed`, `LeaseReleased`, `NodeLimited`, `NodeUnavailable`, `SnapshotCreated`.
24. Подтвердить admin audit log для каждого mutating endpoint.
25. Подтвердить rate limit по user key и минимальное хранение IP/device metadata.
26. Измерить performance budget: scheduler <10 ms, Redis operations <50 ms, rebinding overhead <150 ms, gateway p95 <250 ms без upstream.
27. Подготовить и проверить клиентскую документацию Claude Code CLI, streaming и tool use.
28. Провести ручной/интеграционный acceptance test через реальный Anthropic-compatible endpoint либо контролируемый staging runtime.

## Текущий статус Acceptance Tests Master-ТЗ

| № | Проверка | Статус |
|---:|---|---|
| 1–4 | User key, `/v1/messages`, healthy node, streaming | Частично покрыто; нужен единый acceptance сценарий |
| 5–7 | Mock 429, rebinding events, ответ после retry | Реализовано частично; проверить lock и committed stream |
| 8 | Usage в PostgreSQL | Частично; standard pool/provider маркировка неверна |
| 9–10 | User/admin stats | Частично покрыто |
| 11 | Warning/throttle после fair-use | Warning есть; атомарный throttle под concurrency не доказан |
| 12 | Snapshot и восстановление session | Частично |
| 13 | Rebinding feature flag | Есть целевые тесты |
| 14–15 | Admin cooldown и scheduler exclusion | Частично; Yunwu single-node cooldown нарушает правило |

## Известное состояние автоматических проверок

- Целевые quota/Yunwu/API/repository tests проходят.
- Race tests `quota` и `yunwu` проходят для существующего покрытия.
- `go build ./...` проходит.
- `git diff --check` проходит.
- Полный `go test ./...` **не зелёный**: зафиксированы failures/hang в `internal/server`, включая route manifest/runtime/gnet/WebSocket tests.
- Docker Compose validation не выполнена: Docker Desktop недоступен из текущего WSL окружения.

До устранения этих пунктов Definition of Done обоих ТЗ не выполнен.

## Критерии настоящих 100%

Статус можно изменить на «реализовано полностью» только когда одновременно выполнено следующее:

1. Каждый обязательный пункт и Definition of Done обоих ТЗ сопоставлен с кодом и проверкой.
2. Все P0/P1/P2 пункты этого документа закрыты либо явно исключены новой утверждённой редакцией ТЗ.
3. Fair-share не обходится параллельными запросами, сменой provider, Redis restart, monthly rollover или rebinding.
4. Yunwu не использует standard nodes/keys/capacity и наоборот.
5. Unknown/stale/zero/reserve-exhausted/cooldown nodes никогда не выбираются, включая retry и survival mode.
6. Usage и policy полностью восстанавливаются из PostgreSQL после очистки Redis.
7. `go test ./...` проходит без исключений и зависаний.
8. Race tests quota/reservations/rebinding проходят.
9. `go build ./...` и `git diff --check` проходят.
10. Обе Docker Compose конфигурации валидируются.
11. Acceptance tests из Master-ТЗ выполнены и их результаты сохранены.
12. Проведён security/logging audit на отсутствие credentials.
13. Performance budget измерен, а не предполагается.

## Рекомендуемый порядок дальнейшей реализации

1. Исправить durable standard usage и restart recovery.
2. Реализовать атомарные user-level reservations для standard и Yunwu.
3. Исправить Yunwu TTL cleanup и rebinding reservations.
4. Унифицировать hard health/balance constraints scheduler и rebinding.
5. Завершить capacity invalidation и единый immutable config.
6. Реализовать priority queues и полную degradation/recovery state machine.
7. Закрыть backup/summary/events/stats/security gaps.
8. Исправить полный test suite.
9. Выполнить acceptance, race, performance и Compose проверки.
10. Провести финальный построчный аудит двух ТЗ.
