# Проверка этапа 2 — 2026-10-11

Ветка: `update`. Исходники: SawaMEN/hiddify-core `b5749002266b9782803669138494ef6937eb4e95`,
SawaMEN/hiddify-sing-box `8a82b70836022a1b56eca15df12f56f903cfb27d`.

## Подтверждённые результаты

| Проверка                                                                 | Результат                                                                                                                              |
| ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| Аудит owner origin, commits, gitlinks, nested submodules, source anchors | Успешно                                                                                                                                |
| Сборка настоящего executable linux/amd64                                 | Успешно, Go 1.26.8, CGO disabled                                                                                                       |
| CLI version/revision/tags                                                | Успешно; version sawamen-b5749002266b и полный source revision                                                                         |
| CLI check со статистикой; отказ без config                               | Успешно                                                                                                                                |
| Overlay Clash metadata.user                                              | Тест непосредственно в пакете ядра прошёл; поле находится внутри metadata                                                              |
| Generated configs настоящим check                                        | Encrypted VLESS/XHTTP, Snell6, FinalMask, MASQUE+TLS, XHTTP download+TLS, Hysteria2 gecko+TLS приняты                                  |
| Lifecycle adapter                                                        | Start/restart/stop, жизнь после отмены запроса и остановка с отменённым контекстом; неверный кандидат оставляет текущий процесс/config |
| Проверка идентичности                                                    | Неверный prefix/revision и tag-подстрока отклоняются; режим run не принимается как srun                                                |
| gRPC wire                                                                | Обе закреплённые схемы имени сервиса проверены на обычном protobuf gRPC-сервере; response bytes и reset flag сохранены                 |
| Config из БД панели                                                      | Настоящий бинарник принимает конфиг с клиентом, XHTTP и Gateway; прочие TestHiddify проходят                                           |
| Race tests                                                               | internal/core, internal/hiddify, internal/singbox и целевые lifecycle/config tests internal/web/service прошли                         |
| Go vet и сборка панели                                                   | Успешно                                                                                                                                |

## Ограничение реального запуска

`TestPinnedBinaryRawRuntimeAndStatistics` использует только loopback и пытается
проверить настоящий SOCKS TCP, user/inbound bytes, Clash identity, журнал и
restart/stop. В этой среде Start возвращает:

```
start service: create netlink socket: operation not permitted
```

Это наблюдаемый отказ на запуске, а не успешный end-to-end тест. Никакие TCP/UDP,
квоты, REALITY, Gateway/TUN или per-peer bytes этим прогоном не подтверждены.
Тест оставлен opt-in для Linux-хоста, где ядру доступен netlink:

```bash
HIDDIFY_RUNTIME_BINARY=/path/to/bin/hiddify-core-linux-amd64 \
  go test ./internal/hiddify -run '^TestPinnedBinaryRawRuntimeAndStatistics$' -count=1 -v
```

ARM64, musl и Docker runtime отдельно не проверены. Naive outbound требует
подходящую libcronet. Ограничения этапа 1 и последующие этапы миграции сохраняются.

## Повторяемые проверки

```bash
python3 tools/audit-hiddify-migration.py --upstream /path/to/modified-sing-box --fork /path/to/modified-sing-box
bash -n tools/build-hiddify-core.sh
bash tools/build-hiddify-core.sh /path/to/bin
HIDDIFY_CONFIG_CHECK_BINARY=/path/to/bin/hiddify-core-linux-amd64 \
  go test -race ./internal/core ./internal/hiddify ./internal/singbox
HIDDIFY_CONFIG_CHECK_BINARY=/path/to/bin/hiddify-core-linux-amd64 \
  go test -race ./internal/web/service -run '^TestHiddify|TestSingBox.*(Lifecycle|Crash|Config|Candidate|Restart|Start|Stop)' -count=1
go vet ./internal/hiddify ./internal/singbox ./internal/core
go build .
```

Реализация lifecycle второго этапа подключена к панели. Проверка настоящего
server runtime остаётся открытой до запуска на подходящем Linux-хосте;
автоматическая миграция и удаление старых ядер не включены.
