# Повторная проверка этапов 1–5

2026-10-11. Только ветка `update`, исходная панель `04f58ec`.
Ядро по-прежнему собирается из `SawaMEN/hiddify-core@b5749002266b9782803669138494ef6937eb4e95`
с `SawaMEN/hiddify-sing-box@8a82b70836022a1b56eca15df12f56f903cfb27d`.
Профиль `sawamen-b5749002266b-panel4` и overlays сохранены: для этих исправлений
панели новый бинарник ядра не требуется.

## Найденные ошибки и исправления

1. Runtime проверял только 8 из 12 build tags. Теперь требуются все tags
   закреплённого рецепта, включая gvisor, Naive outbound и purego.
   Python-аудит проверяет совпадение runtime requirements с build lock;
   тест отдельно отклоняет бинарник без каждого обязательного tag.
2. Ручной и фоновый restart могли сбросить статистику до отказа настоящего
   `check -c`. Подготовка и проверка candidate теперь предшествуют traffic
   flush, управлению sidecars, публикации и остановке. Проверенный временный
   файл публикуется отдельно; ошибки проверки не затрагивают работающий core.
3. Проверка только `ConfigBatchActive` пропускала пачку, начавшуюся и
   завершившуюся во время проверки candidate. Счётчик поколений отслеживает
   оба события. Устаревший candidate отбрасывается, pending apply сохраняется.
   Пустой post-commit traffic batch не открывает scope; это позволяет очереди
   сходиться после чтения статистики без изменений пользователей.
4. Фоновая очередь и watchdog могли использовать устаревшее наблюдение о
   выбранном/остановленном ядре. Выбор повторно проверяется под lifecycle
   lock; watchdog заново проверяет `DidCrash` перед восстановлением.
   Ручной Stop и переключение ядра не отменяются запоздавшим recovery.
5. Отмена запроса между проверкой и публикацией/запуском могла всё же
   изменить файл или запустить процесс. Добавлены проверки контекста перед
   публикацией, остановкой, запуском и после startup window.
6. Hiddify generator вызывал `EnsureOutbound` и `KeepOutbounds`: preview
   мог запускать, заменять или останавливать внешние VPN-процессы до проверки
   конфига. Теперь generator только планирует стабильные loopback bridges.
   Sidecars применяются после проверки candidate; obsolete children удаляются
   после успешного применения. Частичный сбой или отказ core восстанавливает
   прежние sidecars. Мгновенный выход нового/восстановленного child учитывается
   как ошибка, а не успешный apply. Native template, заменивший bridge,
   одновременно исключает ненужный sidecar из плана запуска.
7. Изменение секретов/контекста внешнего outbound оставляет bridge JSON
   неизменным. Старый no-op shortcut игнорировал такие изменения. Теперь
   сравниваются и фактически применённые sidecar fingerprints; изменение
   только sidecar options проходит процедуру применения и rollback.

## Проверенные области

| Этап | Область повторной проверки                                                                                                |
| ---- | ------------------------------------------------------------------------------------------------------------------------- |
| 1    | Инвентаризация 26 значений протокола, owner origins, HEAD/gitlinks, source anchors, build lock                            |
| 2    | Pinned binary identity/tags, raw `srun`, CLI candidate checks, protobuf stats namespace, отмена запуска и watchdog        |
| 3    | Core-neutral DTO, сохранение JSON/DB-контрактов, полные model tests и сборка потребителей                                 |
| 4    | DB snapshot, canonical users, templates/overlays, DNS/routing preflight, protocol/transport matrix и настоящий `check -c` |
| 5    | CRUD/bulk, parallel/nested batches, no-op, изменения во время validation, rollback core/sidecars, manual Stop             |

Regression-проверки используют настоящий pinned binary для проверки JSON.
Lifecycle и управляемые сбои воспроизводятся локальными process fixtures;
protobuf API проверяется loopback gRPC server. Fixtures не доказывают VPN
handshake или реальный сквозной учёт трафика.

## Воспроизведение

```bash
export HIDDIFY_CONFIG_CHECK_BINARY=/path/to/panel4/hiddify-core-linux-amd64
go test ./internal/web/service \
  -run '^(TestHiddify.*|TestServerConfig.*|Test(Bulk|Client|ManualClient|Inbound|SingBox|TrafficPostCommit|ManagedMASQUE|MASQUECore|NormalizeSingBoxTemplate).*)$' \
  -count=1 -timeout=180s
go test -race ./internal/externalvpn ./internal/web/service \
  -run '^(TestOutboundPlan.*|TestHiddify(BinaryReject|BatchChanges|Cancelled|Watchdog|Preview|CoreFailure|PendingApply).*)$' \
  -count=1 -timeout=120s
go test -race ./internal/singbox ./internal/web/runtime \
  ./internal/database/model ./internal/core ./internal/coremodel \
  -count=1 -timeout=180s
go vet ./internal/hiddify ./internal/singbox ./internal/core \
  ./internal/coremodel ./internal/externalvpn ./internal/web/runtime \
  ./internal/web/service ./internal/web/job ./internal/web
go build .
bash -n tools/build-hiddify-core.sh
python3 tools/audit-hiddify-migration.py \
  --upstream /path/to/pinned/hiddify-core --fork /path/to/pinned/hiddify-core
```

## Результаты

- Широкий обычный service regression-прогон Hiddify/Client/Bulk/Inbound/
  SingBox/traffic post-commit: PASS (см. команды выше).

- Целевые race-проверки apply/recovery/sidecar-only update: externalvpn — PASS
  (3.735s), service — PASS (30.264s).
- Полные race-проверки sing-box, runtime, database/model и core: PASS.
- Source/build audit с обоими `--upstream`/`--fork`: PASS; 26 значений протокола,
  совпадение revisions, gitlinks, origins, source anchors и всех required tags.
- Pinned Hiddify protocol/transport/CLI fixtures и protobuf tests: PASS с `-race`.
- LDAP/job regression: PASS с `-race` (22.208s).
- `go vet`, сборка панели, build-script syntax и `git diff --check`: PASS.

Широкий service race-прогон (включая все Client/Bulk/Inbound regression tests)
упёрся в общий timeout 300s при `database.initUser`/bcrypt в
`TestClientEditChangesTheBillingDay`. До timeout не было assertion failures
или race reports. Этот общий прогон не объявляется успешным; обычные regression
проверки и целевые race-проверки учитываются отдельно.

## Ограничение среды

Реальный runtime ранее упирался в `create netlink socket: operation not
permitted`; бинарник ядра в этой проверке не изменён. Реальные Start,
VPN handshake и переданный трафик здесь не подтверждены. Автоматическое
переключение default core и удаление старых ядер не выполняются этим review.
Проверки traffic/limits/online этапа 6 остаются отдельной задачей.
