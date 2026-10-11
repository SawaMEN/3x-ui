# Повторная проверка этапов 1–9

Ветка: `update`. База проверки: `6b93a1e5ab705cb3fa8237ada8d983c3eabb4afd`.
Ядро определяется как **sing-box** и берётся из закреплённого репозитория
модифицированной сборки; старое имя `hiddify-core` — входной алиас.
Исходники, overlays и build lock сохранены. Этап 10 не выполнялся.

## Проверенные контракты

| Этап | Что перепроверено | Результат |
| --- | --- | --- |
| 1 | Инвентарь 26 протоколов/alias, owner origins, commits, submodules и build lock | Source audit прошёл |
| 2 | Проверка version/tags/revision, candidate check, supervised start/stop, gRPC statistics contract | Go/regression/race; конфигурации принимает закреплённый binary |
| 3 | Общие DTO, прежние таблицы/индексы/идентификаторы, миграции БД, отсутствие Xray в production deps слоя БД | Полные Go-тесты и go list -deps |
| 4 | Полный конфиг из согласованного снимка БД, связанные пользователи, routing/DNS/TLS, native graph и overlays | Full database config checks настоящим бинарника Sing-box |
| 5 | Local/remote mutation paths, batch apply, restart/rollback, вручную остановленное ядро | Regression/race fixtures |
| 6 | Долговечные checkpoint + traffic totals, повторы запросов, reset/restart, квоты, sessions/IP | Regression/race fixtures; граница последних неопросенных байтов сохраняется |
| 7 | Подписки, HTTP node sync/status/capabilities, legacy compatibility и health UI | Backend-тесты и frontend unit/component tests |
| 8 | Native Gateway/TUN config, listener ownership, pause/resume, firewall rollback и repair | Constructor checks + lifecycle/network fixtures |
| 9 | Backup до InitDB, защита от concurrent panel, trial start до coreType commit, откат файлов/БД/units, interrupted recovery | Offline/installer fixtures; дополнительные исправления ниже |

## Исправленные недоработки

1. **Устаревший подготовленный snapshot.** После `--backup-only` панель могла
   быть запущена вновь и получить новых пользователей или трафик. Apply допускал
   прежнюю копию, а rollback удалял последующие данные. До journal/schema changes
   теперь сверяется SQLite вместе с WAL/SHM/journal. PostgreSQL получает
   fingerprint строк, column schema и sequences в repeatable-read snapshot;
   он проверяется после dump и перед apply. Несовпадение требует нового
   backup directory и не запускает ядро. Старые prepared PG manifests без
   fingerprint могут быть восстановлены, но не использованы для нового apply.
2. **Остаточные процессы.** Offline preflight пропускал `--config=...`,
   `-c=...`, `-config=...` и путь к конфигурации через symlink. Все эти формы
   теперь определяются вместе с относительными путями. Чужие конфигурации
   не блокируют миграцию.
3. **Ошибка остановки перед rollback.** Helper игнорировал отказ stop service.
   Теперь восстановление и замена установки не выполняются, пока stop не
   завершился успешно; recovery snapshot и staged executable остаются.
4. **Legacy alias в совместимости клиентов.** `SyncClientCoreCompatibility`
   принимал старое имя как допустимое, но policy lookup не нормализовал его
   и мог отключить даже VLESS/Naive клиентов. Оба аргумента канонизируются
   до сравнения и записи auto-disable marker.
5. **Пути runtime из EnvironmentFile.** Package globals создавались до чтения
   service environment. Writer использовал новый XUI_BIN_FOLDER, а runtime
   сохранял старый binary/config path. Main теперь пересоздаёт native adapters
   после загрузки окружения и до запуска service goroutines. Тест исполняет
   fake бинарника Sing-box только по окончательному пути.
6. **Зависшая PostgreSQL archive preflight.** Проверка `pg_restore --list`
   игнорировала deadline CLI. Теперь она использует его context; зависший
   subprocess завершается без попытки restore. SQL queries этих migration
   helpers также получают context операции.

7. **Тип ранее сохранённого offline node.** Read DTO возвращал legacy
   `hiddify-core` до следующего heartbeat. Теперь CoreType и RunningCore
   выдаются как `sing-box` без изменения сохранённой строки или статуса узла.

SQLite stale-backup тест действительно добавляет пользователя после backup
и проверяет сохранение свежей БД при отказе. PostgreSQL stale-backup тест
проверяет запрет изменения БД/runtime/journal при изменившемся fingerprint
через fixture; он не заменяет проверку самого SQL на живом PostgreSQL.
Regression совместимости проверяет WireGuard, VLESS и Naive через legacy alias.
Восьмой shell-тест подтверждает, что failed stop исключает restore/start.

## Выполненная проверка

- Полный `go test ./... -count=1 -timeout=180s`, с настоящим бинарника Sing-box для
  opt-in `check -c`; после исправлений повторён.
- Тематические `-race`: migration, Hiddify config/apply/traffic, native/Gateway,
  runtime, controller, process supervision и совместимость клиентов.
- `go vet ./...`, сборка панели и production frontend build.
- Frontend: **150 наборов / 1529 unit/component tests**; typecheck и lint.
- Восемь installer transaction shell tests, `bash -n` install/update/helper/build.
- Source audit обоих закреплённых checkout, production dependencies слоя БД,
  `git diff --check`.

Общий `npm test` дополнительно попытался запустить Storybook browser project
и завершился с environment error: Chromium Playwright отсутствует.
Отдельный `npm test -- --project=unit --project=components` прошёл.
Этот отчёт не считает Storybook browser tests успешно выполненными.

## Открытая эксплуатационная приёмка

Ни один этап не объявлен подтверждённым реальным VPN traffic только на основании
constructor/unit tests. В этом окружении srun ранее получил
`create netlink socket: operation not permitted`. Нужен привилегированный Linux
host для настоящего upgrade старой панели, handshakes TCP/UDP/TLS/REALITY,
Gateway/DNS/forwarding, reboot и внешних protocol runtimes.

Live PostgreSQL backup/fingerprint/schema rollback/restore остаётся непроверенным:
здесь нет сервера и клиентских утилит. При offline migration любые внешние
писатели той же PostgreSQL БД также должны быть остановлены на весь переход;
shared lease защищает процессы этой установки, а не другой сервер.

Учет сохраняет записанные/полученные snapshots; последние неопросенные bytes
при аварийной смерти ядра его текущий API восстановить не может.
Полная сборка всех архитектур и Docker image rollback также не закрыты этим
прогоном. Эти ограничения сохраняют статус открытой приёмки этапов 2/6/8/9.
