# Этап 6: клиенты, трафик, сроки, скорость и подписки

Дата: 2026-11-03 (Екатеринбург). Ветка `update`, draft PR [#279](https://github.com/SawaMEN/3x-ui/pull/279).
Исходный диапазон: v3.9.8 `416a66a3` → зафиксированный main `4aa74411`.
Проверяемый SHA: `c4246dee5723e794ad0aa3b7d2dbc7b6a43f5a96`.

Матрица native-проверок завершена на обеих архитектурах. Повтор полного Go race suite на последнем SHA ещё выполняется; итог этапа будет зафиксирован после завершения.

| Пункт | Выполненная матрица |
| --- | --- |
| 6.1 | CRUD, attach/detach, detached-клиенты, группы/hosts, null/дубликаты/имена, HWID; HTTP/API и компонентные контракты форм |
| 6.2 | Upload/download, local/global/shared квоты, node receipts, повторные и конкурентные heartbeat, checkpoint rollback, открытие БД и перезапуск настоящего ядра |
| 6.3 | Delayed activation только при положительном трафике, calendar/DST/interval boundaries, renewal allowance/reset, закрытие сессий и отказ нового подключения для disabled/expired/spent |
| 6.4 | Upload/download/unlimited, общий бюджет соединений/маршрутов/TCP/UDP, изоляция другого клиента, rollback и перезапуск ядра; настоящие Mieru/Sudoku/MASQUE |
| 6.5 | URI/base64/Xray JSON/sing-box JSON/Hiddify JSON/Clash modern+legacy YAML, настоящие TCP/UDP импорты, escaping/templates, TLS/Reality/ECH fields, headers и capability tokens |

## Исправления и оптимизация

1. **Последний HWID-слот мог выдаваться конкурентно.** PostgreSQL блокирует родительские client rows в стабильном порядке до чтения лимита и COUNT. SQLite использует уже существующий `BEGIN IMMEDIATE`. Защита работает и между процессами панели. Тесты: 24 запроса на один свободный слот; отдельная транзакция, меняющая лимит под удерживаемым row lock. Старый код не ждёт блокировку, новый ждёт commit и использует новый лимит.
2. **Переполнение upload + download обходило квоту.** Общий Go helper насыщает используемую сумму; SQL сравнивает BIGINT без сложения. Исправлены local/global depletion, node merge, API, подписки, Telegram и выбор строк live-presence. Для сортировки больших пар upload/download применяется точное uint64-сравнение. Долговременные индивидуальные счётчики не заменены округлённой суммой.
3. **Групповые агрегаты падали на допустимых больших счётчиках.** Удалён неиспользуемый `SUM(up+down)`. Четыре пути используют один helper. PostgreSQL SUM считает в NUMERIC, SQLite при подтверждённом integer-overflow переходит к потоковому чтению с O(groups) памяти. Отображение и baseline ограничены int64; baseline shifts не превращаются в SQLite REAL и не переполняют PostgreSQL BIGINT. Reset/move сохраняют индивидуальные upload/down по 9e18.
4. **Разные rates одного email создавали разные speed budgets.** План целиком валидируется до listener, broker-файла и закрытия действующих соединений. Конфликт rates, дубликаты route ID, пустые ID/email и отрицательная скорость отвергаются; валидные маршруты одного email переиспользуют общий бюджет.
5. **Expiry в подписке округлялся до секунд.** Используется точная миллисекундная граница. Убраны преждевременное истечение до 999 мс и задержка противоположного сравнения. Ноль остаётся unlimited, отрицательный срок — delayed activation.

HWID status теперь получает COUNT и MAX limit одним SQL вместо двух. Удалены промежуточные in-process mutex stripes: SQLite write transaction и PostgreSQL row locks уже обеспечивают требуемую сериализацию. Обычная агрегация остаётся SQL-запросом; streaming fallback используется только при реальном integer-overflow. Арифметика квот переиспользуется service/API/sub/TG вместо разных небезопасных выражений. Процент ускорения всей панели не заявляется.

## Проверки и воспроизведение

Workflow: `.github/workflows/client-audit.yml`. Проверки обязательных native roots отказывают при отсутствующем тесте, FAIL или SKIP; одно успешное создание конфигурации не считается передачей данных.

- PostgreSQL before-fix gate подставляет `client_hwid.go` и `inbound_disable.go` из `2d7f5f0`, требует именно падений `TestStage6HwidGateParentLock` и `TestStage6QuotaSQL`, восстанавливает текущие файлы и требует PASS. Compile error не принимается за воспроизведение дефекта.
- `TestStage6TrafficArithmetic`: 1000 детерминированных наборов с независимым big.Int oracle. Отдельно проверены насыщенные суммы, точный порядок и миллисекундная expiry boundary.
- Native accounting отправляет 3 × 32 KiB TCP через управляемое панелью ядро. Допуск первичной сверки каждого направления — максимум 512 байт framing на 98304 payload bytes. Повторные polls и открытие БД должны сохранить наблюдённые счётчики точно. После core restart следующий 32 KiB payload должен увеличить оба направления ровно на 32768.
- Native lifecycle повторяет штатную очередь apply и отдельно проверяет disabled, expired и spent quota: существующая сессия закрывается, новое соединение запрещается, другой пользователь может передавать данные.
- Live speed: 256 KiB при 1 Mbps против unlimited клиента; после остановки/старта только core параллельные 256 KiB TCP + 256 KiB UDP одного email не получают два бюджета. Отдельные broker clients Mieru/Sudoku/MASQUE запускаются настоящими pinned binaries.
- Семь форматов экспортируются настоящими service methods. Proxy из parser используется с исходными credentials/transport; меняется только tag для тестового relay. В имени `:/#?&+` и кириллица. Xray JSON импортируется Xray, остальные шесть форматов — owner parser. Каждый формат передаёт TCP и UDP. Также повторяется настоящая subscription chain AnyTLS → ShadowTLS.

До последней правки только test fixture IDs успешно прошёл [contracts job 125515103627](https://github.com/SawaMEN/3x-ui/actions/runs/41514588700/job/125515103627) на `16d0c6b5`: 4545 Go PASS test/subtest events, 0 FAIL; 74 явных SKIP для environment-dependent tests. Обязательные roots прошли без SKIP. PostgreSQL: 13 PASS, 0 FAIL/SKIP. Frontend: 27 файлов, 150 тестов PASS; typecheck и vet PASS. Native проверки вынесены в отдельные обязательные jobs обеих архитектур, их успех не выводится из этих skips.

Native [41524484044](https://github.com/SawaMEN/3x-ui/actions/runs/41524484044), SHA `c4246dee`: обе архитектуры PASS. По 45 PASS test/subtest events, 0 FAIL/SKIP. Core accounting/lifecycle: 5; imports: 31; Mieru: 1; Sudoku: 4; MASQUE: 4.

| Измерение | AMD64 | ARM64 |
| --- | ---: | ---: |
| Alice 256 KiB, 1 Mbps | 1.612 s | 1.643 s |
| Bob 256 KiB, unlimited | 1.642 ms | 1.177 ms |
| После restart, общие 256 KiB TCP + 256 KiB UDP Alice | 4.160 s | 4.166 s |
| Учтено после 3 × 32 KiB + restart + 32 KiB | 131072 up / 131072 down | 131072 up / 131072 down |

Дополнительно на этом SHA прошли [database audit](https://github.com/SawaMEN/3x-ui/actions/runs/41524483696), [runtime AMD64/ARM64](https://github.com/SawaMEN/3x-ui/actions/runs/41524483490), [protocol build](https://github.com/SawaMEN/3x-ui/actions/runs/41524483777) и [общий Go suite/frontend build](https://github.com/SawaMEN/3x-ui/actions/runs/41524483957/job/125548806448).

Повтор полного race/contracts набора на `c4246dee`: ожидается завершение job `125549350652`.

## Версии и границы результата

Owner stream1 binaries из [41421583366](https://github.com/SawaMEN/3x-ui/actions/runs/41421583366) проверяются по SHA-256; Xray v26.10.21, upstream sing-box v1.14.4. Native Linux AMD64/ARM64, Go 1.27.3, PostgreSQL 16 и SQLite. Mieru source `b961978c3be9dd26b94158487c760858e19d1db2`, Sudoku source `56ce6f1aeea18aa69b9bc1d57557ed312b59e5f5`; hashes overlays и binaries входят в artifacts. Новые изменения соседнего hiddify-core не подмешиваются в закреплённые binaries.

- Общая квота нескольких nodes подтверждена DB/receipt/heartbeat contracts; rate budget общий внутри одного panel runtime/broker. Глобальное распределённое shaping нескольких физических nodes не заявляется.
- Подтверждённый RPC snapshot и traffic записываются атомарно. Байты, не попавшие ни в один snapshot до аварийной смерти ядра, его API восстановить не позволяет. Committed snapshots и graceful restart не означают zero-loss при произвольном crash.
- Агрегат выше MaxInt64 отображается с насыщением; индивидуальные counters сохраняются. Точность произвольных BigInt aggregate в интерфейсе не заявляется.
- Семь live import форматов проверены на VMess; AnyTLS/ShadowTLS — отдельная chain. Это не полный cross-product всех protocols/security/clients. TLS/Reality/ECH сохранение дополнительно покрыто форматными контрактами и матрицей этапа 5.
- Hiddify parser не принимает произвольный полный Xray JSON как native JSON: для него использован соответствующий Xray client, без изменения исходных credentials/transport.
- Server HTTP/API и frontend component contracts не заменяют браузерную матрицу этапа 10. Routing/TUN/DNS относится к этапу 7; внешние серверы — к этапу 8.
- Ошибки самого стенда (nil manager, loopback policy, localhost normalization, выбор parser/расширения файла и отсутствующие test IDs и синхронный restart вместо штатной очереди) исправлены отдельно и не включены в число продуктовых дефектов.
