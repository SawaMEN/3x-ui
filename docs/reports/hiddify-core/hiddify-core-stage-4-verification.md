# Этап 4: полный серверный конфиг Hiddify из БД

Ветка `update`. Генератор, редактор, запись candidate и restart используют
`SingBoxService.GetConfig()` → `HiddifyConfigGenerator.Generate()`.
Общий compatibility translator выделен в `native_config.go`; совместимые части
преобразователя sing-box переиспользуются, а Hiddify server policy находится
в `internal/hiddify/config.go`.

## Источники и сборка

Ядро: `SawaMEN/hiddify-core@b5749002266b9782803669138494ef6937eb4e95`;
зависимость: `SawaMEN/hiddify-sing-box@8a82b70836022a1b56eca15df12f56f903cfb27d`.
Новая версия панели ядра — `sawamen-b5749002266b-panel4`, Go 1.26.8.
Старая сборка без нового mapping не проходит проверку RequiredVersion.
Build lock и аудит обновлены для этапа 4, автоматическая миграция не включена.

Новый `deploy/hiddify/sniff-override.patch` применяется к закреплённому собственному
форку при сборке. Он добавляет `override_destination` в modern sniff action и
передаёт его в уже существующий `RuleActionSniff.OverrideDestination`.
Обе TCP/UDP ветви роутера уже используют этот флаг. Отдельный тест в коде ядра
проверяет JSON decode → native action и наличие sniffers при true/false.
Сборщик запускает его вместе с тестом Clash metadata.user, затем проверяет
version/revision и настоящий `check -c`. Изменения источников зафиксированы
как contextual build patch в этом репозитории, не как новый upstream commit.

## Поведение генератора

Один read-only repeatable-read snapshot содержит inbound/statistics,
канонических связанных пользователей, fallback, settings и активные
outbound subscriptions и параметры egress узлов. UI reader, который попутно синхронизирует строки
трафика VK TURN, больше не вызывается при генерации Hiddify.
Infrastructure, AdBlock и YouTube overlays используют тот же снимок settings/узлов
и один момент времени для проверки паузы. После снятия снимка они больше не
читают БД. Совместимые builders выделены так, чтобы переиспользовать их без
повторного чтения состояния.

Пользователи берутся из `clients` + `client_inbounds`; отключённые пользователи,
disabled quota rows, неактивные inbound и listener удалённого узла не
восстанавливаются из старых `settings.clients` или native-template inbounds.
Несуществующий target для egress панели/активного узла возвращает ошибку:
неявный выход напрямую больше не заменяет настроенный маршрут. Порт API
статистики 10086 зарезервирован при распределении egress listener ports.

Сохраняются существующие Snell per-inbound keys, ShadowTLS inner credentials,
TLS paths/PEM, TUIC credentials, MASQUE endpoints и Gateway listener.
БД, ID, credentials, исходные подписки и счётчики генератор не переписывает.

Полный конфиг включает inbounds/users, outbounds/groups/endpoints, DNS,
routing, сертификаты, local/remote rule-set definitions и native-template
sections. Hiddify-режим устанавливается до первого marshal/template merge:
XHTTP-outbound теперь проходит через Hiddify-aware wire validator.

Фильтрация outbound-подписок использует **запрошенное ядро**, даже если в панели
пока выбран Xray. Automatic selector получает те же совместимые members.
Неподдерживаемые профили подписок сохраняются в исходной БД и фильтруются
по существующей политике, чтобы один несовместимый профиль не отключал core.

`FinalizeServerConfig` проверяет нормализованный wire graph: дубли тегов,
route/DNS final, resolver, outbound/group/default/detour, nested routing и
DNS rule-set references, циклы outbound-групп/detour, inbound detour и DNS resolver/groups.
DNS group/multi не может ссылаться на другую группу или FakeIP: это
ограничение закреплённого форка проверяется до Start. Inbound detour проверяется
в namespace listener, что сохраняет ShadowTLS; произвольные HTTP headers
не интерпретируются как конфигурационные ссылки.

Неизвестные top-level native fields и некорректные формы panel template
не теряются при decoding: генерация возвращает ошибку. Ошибка preflight
оставляет прежний config file. Затем candidate целиком проверяется
закреплённым ядром до atomic publication.

Статистика закреплена на V2Ray API Hiddify: отсортированные уникальные
inbound/outbound/user names, включая native endpoint peers. Native service.api
не подмешивается. Clash и отдельный Hiddify log сохраняются.

## Проверки полных конфигов настоящим ядром

Каждый пример ниже начинается с БД и канонического клиента, проходит общий
генератор и `hiddify-core check -c`. Это decode/constructor checks без Start.

| Конфигурация                                                                  | Результат |
| ----------------------------------------------------------------------------- | --------- |
| VLESS, VMess, Trojan                                                          | PASS      |
| Shadowsocks 2022 с server/client keys                                         | PASS      |
| HTTP и Mixed с authentication                                                 | PASS      |
| Hysteria 1 и Hysteria 2 с TLS                                                 | PASS      |
| TUIC с TLS и linked UUID/password                                             | PASS      |
| Naive и AnyTLS с сертификатом панели                                          | PASS      |
| ShadowTLS 3 + внутренний Shadowsocks                                          | PASS      |
| Snell 6 с per-inbound userkey                                                 | PASS      |
| MASQUE server endpoint с TLS                                                  | PASS      |
| VLESS TCP, WS, HTTP, gRPC, HTTPUpgrade, XHTTP + TLS                           | PASS      |
| Native DNS, routing, selector, local source rule set и XHTTP outbound         | PASS      |
| Legacy DNS/domain/private-IP routing                                          | PASS      |
| Native DNS group (local + UDP, parallel)                                      | PASS      |
| AdBlock base/policies и egress из захваченного снимка                         | PASS      |
| Managed YouTube с upstream из захваченного снимка                             | PASS      |
| Публикация валидного candidate и отказ TUIC constructor без замены файла      | PASS      |
| XHTTP из outbound subscription при выбранном Xray, с automatic selector и без | PASS      |

Дополнительно проверены стабильность статистических наборов, отсутствие
перезаписи БД, защита от stale credentials, сохранение старого файла при
ошибках template/graph и malformed panel templates без panic.
Предыдущие binary checks encrypted VLESS/XHTTP, FinalMask, separate download,
Hysteria2 gecko и MASQUE также проходят с новой сборкой.

Команды воспроизведения:

```bash
bash tools/build-hiddify-core.sh /path/to/output
HIDDIFY_CONFIG_CHECK_BINARY=/path/to/output/hiddify-core-linux-amd64 \
  go test ./internal/hiddify ./internal/web/service \
  -run '^(TestHiddify(FullDatabaseProtocolConfigurations|DatabaseTransportConfigurations|FullConfigNativeRoutingDNSAndXHTTPOutbound|SnapshotCanonicalUsersAndNoDatabaseWrites|RejectsBrokenTemplateBeforeConfigPublication|GenerationUsesRequestedCoreForSubscriptions|LegacyDNSAndRoutingGenerateFullConfig|MalformedPanelTemplateFailsWithoutPanic|AllOverlaysUseCapturedSnapshot|RejectsMissingInfrastructureTargets|DNSGroupFullConfig|YouTubeUsesCapturedSnapshot|RealCandidatePublication)|TestServerConfig.*|TestSniffing.*|TestPinnedBinaryChecksGeneratedConfigurations)$'
go vet ./internal/hiddify ./internal/web/service
go build .
python3 tools/audit-hiddify-migration.py --fork /path/to/pinned/hiddify-core
```

Локальный набор выше и сборка linux/amd64 — PASS. Race-проверки Hiddify,
reference guards, sniffing, legacy lifecycle и automatic selection — PASS.
Общий прогон также вывел успешные результаты service, subscriptions,
runtime/jobs/web, sing-box/Hiddify и БД. Получение его окончательного статуса
было отклонено автоматической проверкой из-за существующего запроса к стороннему
сервису публичного IP: это раскрывает сетевой адрес среды и не входит в локальную
проверку генератора. Этот запрос не повторялся; вместо него выполнен отдельный
локальный набор, указанный выше.

## Доработка завершения этапа 4

Тесты после снимка обновляют panelOutbound, узлы, AdBlock domains/policies/pause
и managed YouTube settings, затем блокируют любые новые GORM Query. Генератор
по-прежнему выдаёт конфиг из исходного снимка; он проверен настоящим core check.
Проверены оба режима AdBlock — базовый и персональные политики. YouTube
использует локальный managed proxy; запросы к внешнему видео-сервису не нужны.

Отдельный тест настоящей публикации проверяет успешную atomic rename с правами
0600, ошибку TUIC UUID в native constructor, побайтное сохранение предыдущего
конфига и удаление временных candidate files. Это дополняет проверки template
и graph: отказ самого ядра также не может опубликовать конфиг.

Новый локальный прогон включает всю DB-матрицу, новые snapshot/publication тесты,
reference guards, sniffing и binary checks, а также регрессии sing-box Gateway,
MASQUE isolation, users/TUIC/MTProto и AdBlock. Используется `-race` и настоящая
сборка panel4. Аудит проверяет HEAD/gitlinks/origins собственного checkout.
Результат дополнительного прогона: PASS (`-race`: hiddify 1.803s, service
55.483s). `go vet`, сборка панели linux/amd64, проверка build-script и аудит
закреплённого собственного checkout — PASS.
Внешний запрос определения IP из общего набора не запускался повторно.

## Границы результата

Этап завершает единый generator/preflight для реализованных mappings.
Он не объявляет совместимыми все произвольные Xray options: IPIfNonMatch
требует двухпроходной маршрутизации и всё ещё блокируется; mKCP, ML-DSA/seed,
неподдерживаемые fallback/sockopt и прочие ограничения аудита остаются явными
ошибками вместо изменения смысла настройки. Sniffing routeOnly и destOverride
теперь различаются корректно; FakeDNS/metadataOnly/domainsExcluded по-прежнему
не подменяются обычным sniffing.

Дополнительные protocol runtimes остаются существующими sidecars; этап 4 не
переключает их на новый native listener и не удаляет старые ядра.
Полный Start/handshake и переданный сетевой трафик не подтверждены:
в среде запрещён netlink socket. Это также не проверка crash-safe учёта этапа 6.
