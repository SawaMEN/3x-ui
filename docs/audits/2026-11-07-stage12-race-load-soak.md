# Этап 12 — гонки, конкурентная нагрузка, производительность и стабильность

**COMPLETE — этап 12 закрыт: 5/5 групп.** Подтверждено **12/14 этапов, 60/70 групп; осталось 2 этапа и 10 групп**. Пять обязательных jobs завершились success на кодовом SHA `1cb2b4f76c2b30db82e0dcd9777b1cbeb5c62dca`. Изменения только в `update`; `main` не затронут.

## Приёмочная матрица

| Группа | Проверка | Критерий |
| --- | --- | --- |
| 12.1 | WebSocket fanout / disconnect | `go test -race` для hub, 96 конкурентных клиентов, параллельные send/unregister, закрытие каналов и отмена 100 timer generations |
| 12.2 | Серийная запись трафика | `go test -race`, одновременные Start/Stop во время незавершённой записи; 64 рестарта × 16 запросов с точным счётом, восемь повторов |
| 12.3 | Manager и многоузловые обновления | Реальные service-level race-тесты на поколение manager, node bulk, client address claims и heartbeat |
| 12.4 | PostgreSQL contention | Настоящая PostgreSQL 16 в CI и конкурентные транзакции traffic/client sync с race detector |
| 12.5 | Ограниченный soak и throughput | 30 повторов новых тестов под `-race`; 3 прогона параллельного benchmark, ns/op, B/op, allocs/op; без ложного обещания 24-часового теста |

## Подтверждённые кодовые изменения

1. `StartTrafficWriter`: раньше одновременный вызов `StopTrafficWriter` во время дренирования очереди позволял `StartTrafficWriter` просто вернуться, оставив обработчик остановленным. Теперь новый запуск дожидается завершения отменённого поколения **без удержания `twMu`** и затем создаёт новый обработчик.
2. `Hub.fanout`: раньше mutex освобождался между копированием списка клиентов и `trySend`; параллельный `Unregister` или shutdown мог закрыть `Send`, пока шла запись. Даже recovery от panic не делает гонку send/close безопасной для race detector. Теперь RLock удерживается при короткой неблокирующей записи; также устранено выделение копии `targets` на каждый fanout.
3. Регрессии: `TestStage12TrafficWriterRestartDuringStopDrain`, `TestStage12TrafficWriterBurstAcrossRestarts`, `TestStage12ConcurrentFanoutAndUnregister`, `TestStage12HubCancelPendingThrottleTimers` и `BenchmarkStage12TrafficWriterSubmission`.

## CI

Специализированная матрица: [stage12-race-load-soak.yml](https://github.com/SawaMEN/3x-ui/blob/update/.github/workflows/stage12-race-load-soak.yml).

**Финальный проверенный запуск:** [GitHub Actions #42278324055](https://github.com/SawaMEN/3x-ui/actions/runs/42278324055), кодовый SHA `1cb2b4f76c2b30db82e0dcd9777b1cbeb5c62dca`; **общий conclusion success, все 5/5 обязательных jobs PASS**:

- `websocket-fanout-race` — success, реальная параллельная fanout/unregister нагрузка под `-race`.
- `traffic-writer-race` — success, Stop/Start во время дренирования и 64 цикла × 16 операций, восьмикратный запуск тестов.
- `manager-node-concurrency` — success, сервисные гонки в manager/node/client и traffic heartbeat.
- `postgres-contention` — success, PostgreSQL 16 с повторным запуском под `-race`; job завершил `go test` за 76,791 с.
- `bounded-soak-benchmark` — success, 30 повторов тестов и 3 измерения пропускной способности: **755,3–773,0 нс/операцию, 392 B/op, 6 allocs/op** на GitHub Actions.

После проверенного кодового SHA имеется только документальный коммит с этим отчётом (сравнение `1cb2b4f...` → `0812ea9...`), код дополнительных изменений не содержит.

## Пределы применимости

- CI под `-race` не гарантирует отсутствие всех гонок и не заменяет длительную production-нагрузку.
- Повторения и synthetic workload не являются 24-часовым soak на физическом VPS; реальные долговременные измерения остаются вне покрытия этой специализированной матрицы.
- Benchmark предназначен для сравнения будущих коммитов, а не универсального порога производительности.
- Общие CI/Release/Docs CI могут иметь независимые дефекты. Финальный release gate — этап 14.
