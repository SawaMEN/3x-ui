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
outbound subscriptions. UI reader, который попутно синхронизирует строки
трафика VK TURN, больше не вызывается при генерации Hiddify.
Существующие infrastructure/adblock/YouTube overlays применяются после
основной генерации своими совместимыми builders.

Пользователи берутся из `clients` + `client_inbounds`; отключённые пользователи,
disabled quota rows, неактивные inbound и listener удалённого узла не
восстанавливаются из старых `settings.clients` или native-template inbounds.
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
DNS rule-set references, циклы групп/detour. Inbound detour проверяется
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

| Конфигурация | Результат |
| --- | --- |
| VLESS, VMess, Trojan | PASS |
| Shadowsocks 2022 с server/client keys | PASS |
| HTTP и Mixed с authentication | PASS |
| Hysteria 1 и Hysteria 2 с TLS | PASS |
| TUIC с TLS и linked UUID/password | PASS |
| Naive и AnyTLS с сертификатом панели | PASS |
| ShadowTLS 3 + внутренний Shadowsocks | PASS |
| Snell 6 с per-inbound userkey | PASS |
| MASQUE server endpoint с TLS | PASS |
| VLESS TCP, WS, HTTP, gRPC, HTTPUpgrade, XHTTP + TLS | PASS |
| Native DNS, routing, selector, local source rule set и XHTTP outbound | PASS |
| Legacy DNS/domain/private-IP routing | PASS |
| XHTTP из outbound subscription при выбранном Xray, с automatic selector и без | PASS |

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
  -run '^(TestHiddify(FullDatabaseProtocolConfigurations|DatabaseTransportConfigurations|FullConfigNativeRoutingDNSAndXHTTPOutbound|SnapshotCanonicalUsersAndNoDatabaseWrites|RejectsBrokenTemplateBeforeConfigPublication|GenerationUsesRequestedCoreForSubscriptions|LegacyDNSAndRoutingGenerateFullConfig|MalformedPanelTemplateFailsWithoutPanic)|TestServerConfig.*|TestSniffing.*|TestPinnedBinaryChecksGeneratedConfigurations)$'
go vet ./internal/hiddify ./internal/web/service
go build .
python3 tools/audit-hiddify-migration.py
```

Локальный набор выше и сборка linux/amd64 — PASS. Race-проверки Hiddify,
reference guards, sniffing, legacy lifecycle и automatic selection — PASS.
Общий прогон также вывел успешные результаты service, subscriptions,
runtime/jobs/web, sing-box/Hiddify и БД. Получение его окончательного статуса
было отклонено автоматической проверкой из-за существующего запроса к стороннему
сервису публичного IP: это раскрывает сетевой адрес среды и не входит в локальную
проверку генератора. Этот запрос не повторялся; вместо него выполнен отдельный
локальный набор, указанный выше.

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
