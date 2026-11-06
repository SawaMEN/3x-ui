# Этап 5: управление подключениями и применение изменений

Ветка `update`. Собственные исходники ядра и build lock этапа 4 сохранены:
`SawaMEN/hiddify-core@b5749002266b9782803669138494ef6937eb4e95`,
`SawaMEN/hiddify-sing-box@8a82b70836022a1b56eca15df12f56f903cfb27d`.
Использована сборка `sawamen-b5749002266b-panel4`; изменения этапа 5 находятся
в панели, новая версия бинарного профиля не требуется. Go панели — 1.27.1.

## Local runtime и массовые операции

`LocalDeps.RequestConfigApply` явно запрашивает применение конфигурации,
считанной из зафиксированной БД. `RestartCore` оставлен совместимым fallback
для существующих embedders/tests. Native-ветка не вызывает Xray HandlerService.
CoreType читается один раз на проверку режима.

`ConfigBatchRuntime` — дополнительный контракт local runtime. Вложенные и
параллельные scopes объединяются счётчиком; mutations выставляют dirty, а
последний завершившийся scope выдаёт один запрос применения. Повторное закрытие
scope безопасно. Ошибка enqueue возвращается вызывающему коду; service helper
сохраняет pending flag для повтора. Пустая пачка не запрашивает применение.

Scopes охватывают inbound create/update/enable/delete/bulk delete, client
create/update/attach/detach/delete, bulk create/attach/detach/adjust/enable/delete,
локальные post-commit traffic mutations. LDAP sync группирует create/enable/
disable/delete после получения ответа каталога. Посторонний сетевой LDAP fetch
не удерживает scope. Remote runtime сохраняет собственный контракт вызовов.

Native inbound update и enable toggle используют одну операцию полного конфига
вместо DelInbound + AddInbound. Добавление/редактирование native users также
применяет итоговый список один раз. TUIC остаётся native при выбранном Hiddify.
Подготовка payload и lifecycle существующих sidecars сохранены. Переход между
протоколами запрашивает native apply даже при disabled новом listener: старый
listener/bridge должен исчезнуть. Snell включён в native delete dispatch.

## Контролируемое применение

Очередь native apply проверяется отдельным cron раз в секунду. Legacy Xray
сохраняет прежнюю cadence. Фоновые тики не пересекаются, а активный scope
откладывает применение и не расходует pending flag. После ожидания lifecycle
lock повторно проверяются running state и active batch: ручной Stop не отменяется
запаздывающим pending apply. Отменённый контекст не начинает применение.

Hiddify сравнивает сгенерированные bytes с `Process.AppliedConfig`, а не с
editable config file. Неизменившаяся конфигурация не перезапускает процесс и не
сбрасывает statistics API. Если preview изменил файл, но БД соответствует
запущенному состоянию, файл атомарно восстанавливается из applied snapshot.

Для изменённого конфига остаётся preflight + проверка candidate настоящим
`check -c` перед заменой файла и остановкой. Preflight failure не останавливает
core и не расходует его статистику. Ошибка применения сохраняет pending flag.
Если новый процесс не смог запуститься, восстанавливаются байты последнего
applied config с правами 0600 и atomic/durable publication. При остановленном
старом процессе выполняется bounded запуск предыдущей конфигурации. Ошибки
применения и recovery сохраняются вместе. БД остаётся desired state для
последующего повторного применения после исправления ошибки.

## Проверка HotReload

В закреплённом форке есть `/hcore.Core/HotReload`:
`v2/hcore/hotreload.go` использует `static.StartedService`, Hiddify options и
client config builder. Панель запускает raw server command `cmd/cmd_sing_run.go`.
Этот `srun` не регистрирует hcore gRPC server и не предоставляет этот RPC.
Его SIGHUP выполняет close + create/start Box; это полный reload без panel ACK.

Поэтому панель не вызывает client RPC для server-конфига и не объявляет
`HotInboundReload`. Все изменения raw server state применяются контролируемым
restart. Это предусмотренный планом fallback для неподдерживаемого hot apply.
Ядро не модифицировалось ради непроверенного API.

## Проверки

| Проверка                                                   | Результат                                                     |
| ---------------------------------------------------------- | ------------------------------------------------------------- |
| 100 параллельных local user mutations и вложенные scopes   | Один apply, ноль Xray API обращений                           |
| Пустой scope, повторное закрытие, ошибка enqueue           | Без лишнего apply; ошибка сохранена                           |
| Disabled переход MTProto → VLESS                           | Один запрос native apply                                      |
| DB inbound create/update/disable/enable/delete             | Зафиксированное состояние, один apply на изменение            |
| 30 клиентов × 2 inbound: bulk create/disable/enable/delete | Один итоговый конфиг на каждую пачку                          |
| Bulk attach/detach и Snell delete                          | Один apply; удалённая identity/listener не возвращается       |
| Pending apply во время batch                               | Flag сохранён, процесс не перезапущен                         |
| Неизменившийся конфиг и unapplied preview                  | Без restart/reset; восстановлены applied bytes                |
| Ошибка preflight                                           | Старый процесс/файл сохранены, retry поставлен                |
| Сбой запуска candidate и исправление DB                    | Recovery старого процесса/конфига, успешный последующий retry |
| Ручная остановка                                           | Pending apply не запускает core                               |

DB lifecycle/bulk примеры выше проходят полный генератор и настоящий
`hiddify-core check -c`. Startup failure и количество запусков проверяются
контролируемым локальным process fixture и loopback protobuf stats server.
Это два разных уровня проверки; fixture не используется как доказательство
действительного VPN handshake.

Дополнительно выполнены существующие проверки runtime, клиентов, bulk,
inbounds, sing-box, Hiddify config/transport/protocol matrix и binary checks,
LDAP/core capabilities, `go vet`, сборка панели, build-script syntax и аудит
HEAD/gitlinks/origins собственных исходников.

Итоговый race-прогон: runtime — PASS (1.119s), service — PASS (57.908s),
Hiddify — PASS (2.542s). Обычные regression-проверки service, runtime,
LDAP и core capabilities прошли; `go vet` и сборка панели завершились успешно.

Команды воспроизведения:

```bash
HIDDIFY_CONFIG_CHECK_BINARY=/path/to/panel4/hiddify-core-linux-amd64 \
  go test -race ./internal/web/runtime ./internal/web/service ./internal/hiddify \
  -run '^(TestLocal.*|TestHiddify.*|TestRuntimeLifecycleAndInvalidCandidate|TestServerConfig.*|TestSniffing.*|TestPinnedBinaryChecksGeneratedConfigurations)$' \
  -count=1 -timeout=180s
go test ./internal/web/job ./internal/core \
  -run '^(TestLdap.*|TestLDAP.*|TestCapabilitiesFor)$'
go vet ./internal/web/runtime ./internal/web/service ./internal/web/job ./internal/web
go build .
python3 tools/audit-hiddify-migration.py --fork /path/to/pinned/hiddify-core
```

## Ограничение проверки запуска

Повторно выполнен реальный loopback runtime test без внешнего IP lookup:
`HIDDIFY_RUNTIME_BINARY=/path/to/panel4/hiddify-core-linux-amd64 go test
./internal/hiddify -run '^TestPinnedBinaryRawRuntimeAndStatistics$'`.
Он завершился ошибкой `start service: create netlink socket: operation not
permitted`. Реальный Start/handshake/переданный трафик этой средой не подтверждены.

Этап 5 реализует применение изменений в панели и проверяет orchestration.
Crash-safe traffic/limits/online остаются этапом 6; default switch, миграция
установок, удаление старых ядер и перенос остальных sidecars не включены.
