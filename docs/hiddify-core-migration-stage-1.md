# Миграция на hiddify-core: этап 1

Дата аудита: 2026-10-09. Ветка: `update`. Снимок панели:
`cf730af652387a2e99fa1cda20674b038a6edfdf`.

Цель всей миграции: один управляемый hiddify-core по умолчанию; удалить отдельные
Xray и sing-box, их установку, переключатель и зависимости после переноса функций.
Этот этап фиксирует цель и ограничения. Работающий runtime, база, установщик и
подписки пока не переключаются. Ни одна несовместимая функция не должна молча
исчезнуть при последующем переключении.

## Зафиксированная версия и проверяемые источники

Машинный lock: [hiddify-core-target.json](hiddify-core-target.json).

| Компонент | Зафиксированное значение |
| --- | --- |
| hiddify-core | `v4.1.0`, commit `c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0` |
| hiddify-sing-box, submodule | `0a02b7729f6a211436bb8bdcd8696c283eb27767` |
| ray2sing, submodule | `f58be84e30d946915a1de437fbcc3d3ffca18a23` |
| Проверенный архив | `hiddify-core-linux-amd64.tar.gz` из релиза `v4.1.0` |
| SHA-256 архива | `79f55725b9c1c0b3cad46a45ededb3c5be83d7a6236cc1dfa9bd78fb3c687e6f` |
| Вывод бинарника | hiddify-core `v4.1.0`, hiddify-sing-box `1.13.1`, Go `1.25.7`, linux/amd64, CGO disabled |

Не использовать `latest` при установке целевого ядра. Go module заявляет sing-box
`v1.13.0`, но заменяет его локальным submodule; версия из `go.mod` не описывает
весь фактический runtime. Зафиксировать оба submodule и параметры сборки.
ARM64, musl и другие платформы ещё не проверены; amd64 checksum нельзя применять
к их архивам. `libcronet.so` входит в проверенный архив: будущий установщик должен
учитывать сопровождающие runtime-файлы, а не копировать только исполняемый файл.

Первичные источники, все на зафиксированной ревизии:

- [Релиз v4.1.0](https://github.com/hiddify/hiddify-core/releases/tag/v4.1.0).
- [go.mod](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/go.mod), [.gitmodules](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/.gitmodules).
- [Registry inbounds/outbounds/endpoints](https://github.com/hiddify/hiddify-sing-box/blob/0a02b7729f6a211436bb8bdcd8696c283eb27767/include/registry.go), [QUIC](https://github.com/hiddify/hiddify-sing-box/blob/0a02b7729f6a211436bb8bdcd8696c283eb27767/include/quic.go).
- [CLI srun](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/cmd/cmd_sing_run.go), [неэкспортированный check](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/cmd/cmd_check.go).
- [Core gRPC](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/v2/hcore/hcore_service.proto), [raw config](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/v2/hcore/buildconfighelper.go).
- [Транспорты client/server](https://github.com/hiddify/hiddify-sing-box/blob/0a02b7729f6a211436bb8bdcd8696c283eb27767/transport/v2ray/transport.go), [их JSON-схема](https://github.com/hiddify/hiddify-sing-box/blob/0a02b7729f6a211436bb8bdcd8696c283eb27767/option/v2ray_transport.go).
- [Сборка release CLI](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/.github/workflows/build.yml), [Makefile для библиотек](https://github.com/hiddify/hiddify-core/blob/c9d6f0f00b2eda34e4fb71863e4e0a62b3e931a0/Makefile).

## Как читать матрицу

«Перенос» означает, что серверная реализация найдена в исходниках; это не
подтверждение установленных соединений. «Адаптация» требует изменения модели
конфига/учёта. «Внешний runtime» означает сохранение функции через существующий
специализированный сервис с новым маршрутом в hiddify-core. «Блокер» означает,
что зафиксированный upstream не обеспечивает эквивалентное поведение.
Само наличие outbound, зависимости или заглушки не доказывает поддержку inbound.

## Серверные протоколы: все 25 значений модели и alias Hysteria2

Источник инвентаря: `internal/database/model/model.go` и
`internal/database/model/vk_turn_compat.go`. Нельзя пользоваться только
`coreSupportsInboundProtocol`: эта функция включает внешние сервисы и не
проверяет транспорт или безопасность.

| Значение панели | Способ переноса / конкретное ограничение |
| --- | --- |
| `vmess` | Перенос в `vmess` inbound; UUID, имена пользователей, параметры VMess и транспорт. Отдельно проверить legacy alterId и multi-user. |
| `vless` | Перенос в `vless` inbound; UUID и `flow`. VLESS encryption/decryption ML-KEM, Vision seed и расширения REALITY — блокеры ниже. |
| `trojan` | Перенос в `trojan` inbound; password, TLS. Fallback только default и по ALPN; Xray name/path/xver не переносимы текущим адаптером. |
| `shadowsocks` | Перенос в `shadowsocks` inbound; сохранить метод, master PSK, user PSK и ограничения TCP/UDP. Multi-user и каждый используемый cipher требуют handshake-теста. |
| `http` | Перенос в `http` inbound; accounts → users, TLS и авторизация без потери выключенных клиентов. |
| `mixed` | Перенос в `mixed` inbound. SOCKS `udp=false` отдельно не сохраняется текущим переводчиком; при таком ограничении остановить миграцию строки. |
| `hysteria` | Выбрать `hysteria` или `hysteria2` по streamSettings.hysteriaSettings.version; сохранить auth, bandwidth, masquerade и поддерживаемый obfs. Нужен `with_quic`. |
| `hysteria2` | Совместимый alias из vk_turn_compat; нормализовать к версии 2, не создавать ещё один независимый тип в БД. |
| `tuic` | Перенос в native `tuic` inbound; переиспользовать сборку users из `singbox.go`. Не запустить одновременно native listener и tuic-server на одном порту. Нужен `with_quic`. |
| `naive` | Native `naive` inbound зарегистрирован; сохранить username/password, сертификат и TCP/QUIC-вариант. Наличие `with_naive_outbound` относится к outbound. |
| `anytls` | Native `anytls` inbound; users/password, padding_scheme, сертификаты. Параметры и статистика требуют проверки на реальном трафике. |
| `shadowtls` | Native `shadowtls` inbound + внутренний Shadowsocks inbound через detour. Сохранить derived credentials, handshake/SNI и учёт обоих уровней без двойного начисления. |
| `mieru` | Native inbound найден в `protocol/mieru/inbound.go`; требуется адаптация users и portBindings. В панели Mieru v3.38.0, в целевом ядре v3.27.0: не удалять текущий runtime до проверки совместимости. |
| `wireguard` | Адаптация к endpoint `wireguard` с listen_port, address, private_key, peers. Это endpoint, а не `wireguard` inbound. Сохранить peer/allowed_ips и отдельный учёт клиентов. |
| `amneziawg` | Исходник endpoint `awg` есть, но release CLI без `with_awg` отказывает в создании. Панель использует amneziawg-go/v3; ядро — v0.2.16. Сохранить embedded amneziawgnet + SOCKS bridge; native перенос блокируется AWG 3.1 parity и peer metering. |
| `tun` | Native `tun` inbound; нужно пересобрать конфиг интерфейса и policy routing. Существующий TranslateXrayInbound его отклоняет. Привилегии/маршруты принадлежат gateway runtime панели. |
| `tunnel` | Блокер прямого переноса Xray tunnel/dokodemo-door. Hiddify `tunnel_server`/`tunnel_client` — другой протокол с UUID/key и вложенным listener. Для forwarding проектировать `direct` inbound + route; для transparent proxy — redirect/tproxy, не переименовывать тип механически. |
| `mtproto` | Внешний Telemt: сохранить reload, quota/expiry, webproxy, секреты и subscription reconciliation; его egress/bridge перевести на hiddify-core. Native MTProto inbound не найден. |
| `pingtunnel` | Внешний runtime `internal/externalvpn`; сохранить TCP/UDP bridge, supervisor, настройки и трафик. Native inbound не найден. |
| `trusttunnel` | Внешний runtime `internal/externalvpn`; сохранить auth/TLS и bridge. Native inbound не найден. |
| `fptn` | Внешний runtime `internal/externalvpn`; сохранить token/MTU/SNI и bridge. Native inbound/outbound не найдены. |
| `openflux` | Внешний runtime `internal/externalvpn`; сохранить secret/context/transports и bridge. Native inbound/outbound не найдены. |
| `vk-turn-proxy` | Сохранить существующий внешний runtime и WireGuard reconciliation/relay; заменить только зависимость от старого ядра. Native inbound не найден. |
| `sudoku` | Сохранить `internal/sudoku` и его credentials/traffic bridge. Native inbound не найден. |
| `snell` | Блокер: нет native Snell inbound/outbound. Панель хранит версии вплоть до 6 и userkey, которые upstream не понимает. Для сохранения нужен port реализации в целевое ядро; не отключать клиентов автоматически. |
| `masque` | Блокер: панель генерирует endpoint `masque-server`, отсутствующий в целевой registry. Нужен port серверной реализации и схемы/users/маршрутов/учёта. Клиентская поддержка HTTP3 не заменяет сервер MASQUE. |

Специализированные сервисы не заменяют целевой hiddify-core на другое общее
proxy-ядро: они уже обеспечивают отдельные протоколы. Если требуется удалить и
их, это дополнительные порты реализаций, а не возможность upstream v4.1.0.

## Транспорты, TLS, REALITY и маски

Источники панели: `internal/singbox/config.go`, `inbound_compat.go`,
`finalmask_compat.go`, `frontend/src/lib/xray/protocol-capabilities.ts`.

| Функция | Перенос / ограничение |
| --- | --- |
| TCP/raw | Без transport-блока; plain TCP допустим только для протоколов, допускающих его. TCP header camouflage не считать эквивалентом TLS. |
| WS | transport `ws`: path, headers и early-data; перенос есть в текущем переводчике. |
| HTTP/H2 | transport `http`: host, path, method, headers, таймауты. Проверить поведение без TLS и с ALPN. |
| gRPC | transport `grpc`: service_name, idle/ping timeout, permit_without_stream. Проверить Xray multiMode/нестандартные поля, не отбрасывать их. |
| HTTPUpgrade | transport `httpupgrade`: host/path/headers. |
| XHTTP | Реальные client/server реализации есть, включая packet-up/stream-up/stream-one. Текущий переводчик панели отклоняет xhttp. Добавить отдельный mapping XHTTPOptions, xmux/downloadSettings и проверить режимы; это адаптация, не отсутствие сервера. |
| mKCP | В transport registry отсутствует. Блокер для existing KCP configs; QUIC или XHTTP не эквивалентны. |
| Xray QUIC transport | Native QUIC transport есть с `with_quic`, но его options пусты. Xray security/key/header не переносить как native QUIC: конфиг с такими полями блокировать. |
| TLS | Native inbound TLS, PEM/path, SNI/ALPN, min/max version, ciphers и ECH. Проверить сертификаты на целевом JSON schema; Xray rejectUnknownSni, pinning и несколько certificate identities текущий переводчик отклоняет. |
| REALITY | Native reality handshake/private_key/short_id, клиентские utls/public_key/short_id. Проверять допустимую пару protocol/transport, не переносить UI eligibility как runtime capability. |
| REALITY ML-DSA / mldsa65Seed | Поля в целевой TLS схеме нет. Текущий sing-box адаптер пропускает seed; миграционный адаптер должен сообщать об изменении защиты и блокировать автоматический перенос такого конфига. |
| VLESS Vision | Базовый flow `xtls-rprx-vision` есть; проверить TCP/TLS и TCP/REALITY на реальном клиенте. Xray Vision seed и XHTTP+VLESS encryption не подтверждены. |
| VLESS encryption / ML-KEM | VLESSInboundOptions не содержит decryption; блокер. Outbound `xray` — заглушка `Xray is not implemented yet`, не обход ограничения. |
| VLESS fallback | В native VLESS inbound поля fallbacks нет. Блокер для existing fallback цепочек. |
| Trojan fallback | `fallback`/`fallback_for_alpn` есть; Xray name/path/xver, UNIX destination и неоднозначные дубликаты — блокеры. |
| FinalMask TCP, XMC/XOR и прочие маски | Нет эквивалентной схемы. Не удалять эти опции молча. |
| Hysteria2 salamander | Перенести в native `obfs` с password; handshake-тест обязателен. |
| Hysteria2 gecko | В целевой Hysteria2Obfs только type/password; min_packet_size/max_packet_size отклоняются бинарником. Блокер для уже генерируемого panel gecko. |
| UDP hopping / QUIC tuning | Текущий адаптер генерирует server_ports/hop_interval и дополнительные окна; проверить каждое поле на закреплённой схеме (она отличается от текущего sing-box панели), неподдерживаемые поля блокировать. |
| Mux | Проверить конкретные VMess/VLESS multiplex options; Xray mux/xmux/UDP443/XUDP нельзя копировать одним блоком. |
| Sockopt и sniffing | Сохранить mark/interface/TFO/MPTCP при наличии эквивалента. PROXY protocol/customSockopt/TCP congestion/tproxy в sockopt текущий адаптер отклоняет. Sniffing переносить в route action sniff; FakeDNS destOverride, metadataOnly и domainsExcluded — отдельные ограничения. |

## Outbounds, DNS, маршрутизация и подписки

| Функция панели | Рабочий способ / конкретное ограничение |
| --- | --- |
| freedom/blackhole | direct / route reject (при необходимости native block). Проверить domainStrategy, bind/mark, fragment и шум отдельно. |
| VMess/VLESS/Trojan/SS/HTTP/SOCKS/Hysteria/TUIC | Переиспользовать TranslateXrayOutbound с дополнительной валидацией целевой схемы; один credential/server на native outbound, дополнительные серверы — группа. Ограничения inbound не означают одинаковую поддержку outbound. |
| WireGuard/WARP/PIA | endpoint wireguard или целевой WARP endpoint после mapping; сохранить reserved, keys, addresses, PIA auth/lease/keepalive и WARP настройки панели. |
| Native `singbox:<type>` | Запретить слепое pass-through: registry отличается. AnyTLS, ShadowTLS, Mieru, SSH, Tor, Naive, selector/urltest имеют реализации; проверить options и build tags. |
| Native bridge/Snell/MASQUE | Целевая registry их не имеет; явный блокер. FPTN/OpenFlux и прочие external outbounds — через существующий SOCKS bridge. |
| selector/urltest и balancers | Native группы есть; mapping Xray balancers/observatory/fallbackTag и health events отдельно, сохранить стабильность тегов. |
| DNS UDP/TCP/TLS/HTTPS/QUIC/HTTP3/local/hosts/FakeIP | Typed native servers; QUIC/HTTP3 требуют with_quic. Использовать mapping из routing_dns.go, сверить final/strategy/detour/domain_resolver и cache. FakeIP не эквивалентен Xray FakeDNS автоматически. |
| DNS SDNS/DNSCrypt | В fork есть SDNS transport; отдельный mapping stamp. Не считать любой Xray DNS address готовым native server. |
| DNS rules, qtype, split DNS и adblock | Native dns.rules / route actions, сохранить порядок, fallback и qtype. Проверить query types и исключения на целевых fixtures. |
| domain/IP/port/source/inbound/user rules | Перевод в native route.rules; route user matcher и metadata.User должны соответствовать именам DB clients. Поля, не имеющие mapping, возвращают ошибку с tag/index. |
| geoip/geosite/ext:*.dat | Конвертация категорий в native rule_set с теми же значениями. Переиспользовать geodata reader/conversion; нельзя передавать Xray ext token как IP matcher. Удалять Xray dependency только после выделения geodata формата. |
| sniff/resolve/hijack-dns/reject | Native route actions; сохранить порядок относительно route rules, default outbound и DNS resolver. |
| Gateway | Native tun/redirect/tproxy плюс существующие nft/ip forwarding/ip rules в internal/gateway. Новый binary lifecycle не заменяет настройку ОС. Проверить rollback/boot recovery и переключение gateway mode. |
| Remote nodes / mTLS | Runtime capabilities/version handshake; старые узлы не получать новый config. Сохранить API совместимость и core-aware restart. |
| Подписки raw/Clash/sing-box/Hiddify | Клиентские форматы остаются отдельными экспортами; генерировать из канонической модели БД, не из client-oriented hiddify build. Проверить credentials, profile sorting, detached/shared clients, domain/port и server-only поля. |
| Импорт внешних подписок и routing presets | Сохранить проверку URL/SSRF, atomic update, tags, detour и rule references; проверять imported native type по новой registry. |
| Native JSON editor | Целевая схема 1.13.1 fork, а не универсальная sing-box latest. Проверка endpoints/services/experimental плюс согласование DB-owned listener tags обязательны. |

## Статистика и управление runtime

| Контракт | Рабочий способ / конкретное ограничение |
| --- | --- |
| Start/stop/restart/version | `hiddify-core srun -c <absolute-config>` для panel-owned raw server config. `run` использует клиентский builder/Setup, это другой entrypoint. Адаптировать process identity (сейчас ожидает run), сигнал SIGTERM/SIGHUP и applied config snapshot. |
| Проверка перед применением | CLI `check` не зарегистрирован: непосредственное копирование `sing-box check` сломает restart/rollback. На следующем этапе нужен validation entrypoint на pinned source: decode + box.New + Close без Start, до замены работающего конфига. |
| Hiddify Core gRPC | Start/Stop/Restart, logs/outbounds/system info есть. Не предоставляет эквивалент Xray HandlerService/StatsService для add/remove inbound/client. Raw config доступен через EnableRawConfig; client builder нельзя использовать для сервера. |
| User/inbound/outbound bytes | Release CLI включает `with_v2ray_api`. experimental.v2ray_api.stats enabled + lists inbounds/outbounds/users, QueryStats через `/experimental.v2rayapi.StatsService/QueryStats`; существующий ручной codec панели можно выделить без Xray imports. |
| Идентичность пользователя | VMess/VLESS/Trojan/SS и другие inbound должны отдавать стабильный metadata.User. У Naive/Mieru username вместо name, у WireGuard peer identity: это требует mapping, не гарантировано наличием счётчика. |
| Лимиты, expiry, autoreset и долговечный учёт | Сохранить job/DB transaction path, crash/restart baseline и disabled client filtering; снять последние counters перед остановкой. QueryStats reset должен учитывать потерю ответа и повторы. |
| Online/IP/session kill | Clash API /connections с secret и loopback; текущие panel client/session helpers адаптировать к фактическим metadata и endpoint-сессиям. Hiddify CoreInfo общая статистика не заменяет user quota. |
| Hot user/inbound mutations | Xray HandlerService отсутствует. Atomic config replacement + validated reload/restart; удаление клиента должно также закрывать активные сессии. Оценить restart downtime до удаления old hot-apply paths. |
| Внешние протоколы и endpoints | Сохранить Telemt/AWG/TUIC/relay счётчики до доказанного native per-user metering. Не складывать native и bridge bytes дважды. |
| Makefile против release workflow | В Makefile есть with_awg/with_grpc, но нет with_v2ray_api; release workflow наоборот включает with_v2ray_api и не включает with_awg. Для собственной panel-сборки явно фиксировать tags, нельзя выводить возможности из названия продукта. |

## Что реально проверено

Скачан официальный amd64 архив, checksum совпал с опубликованным. Бинарник
запущен для `version`, `--help`, `srun --help`; CLI `check --help` вернул ошибку
unknown command. Изолированные raw config probes на loopback дали:

| Probe | Наблюдение |
| --- | --- |
| VLESS TCP | JSON декодирован и service создан; Start остановлен sandbox networkmanager: operation not permitted. |
| VLESS XHTTP | JSON декодирован и service создан; то же ограничение Start. Не подтверждает успешный XHTTP handshake. |
| V2Ray stats | V2Ray server инициализирован; Start остановлен networkmanager. gRPC QueryStats с трафиком ещё не проверен. |
| AWG endpoint | `Awg is not included in this build, rebuild with -tags with_awg`. |
| Snell inbound | `unknown inbound type: snell`. |
| MASQUE endpoint | `unknown endpoint type: masque-server`. |
| Bridge outbound | `unknown outbound type: bridge`. |
| Hysteria2 gecko sizes | `obfs.min_packet_size: json: unknown field "min_packet_size"`. |

Не выполнялись полноценные TCP/UDP handshakes, nft/ip rules, ARM64/musl и
проверка учёта на сетевом трафике. Поэтому матрица разделяет source support,
подтверждённые отказы официального бинарника и будущие integration gates.

## Результат этапа и вход в следующий

Этап 1 завершён как аудит: целевая версия и snapshot зафиксированы; каждому
протоколу и функциональному контракту указан путь переноса либо ограничение.
Это не разрешение на переключение работающего сервера: stock v4.1.0 не сохраняет
полный функционал этой панели.

Следующий этап: реализовать `internal/hiddify` runtime и validation entrypoint
поверх pinned source, выделить config/stats interfaces из Xray, затем выполнять
mapping и integration fixtures. Для текущего набора функций потребуется
panel-specific hiddify-core build/patches (как минимум Snell/MASQUE и несовместимые
защитные опции) либо сохранение конкретных внешних protocol runtimes. Все
нераспознаваемые meaningful options должны останавливать preflight с указанием
inbound/outbound tag и поля.

Перед финальным удалением старых ядер требуются: все 25 протоколов/alias и
активные config combinations проходят preflight; handshakes TCP/UDP/TLS/REALITY;
per-user quota/reset/restart без потерь и дублей; gateway boot/rollback; remote
nodes/mTLS; подписки и expiry; сборки всех поддерживаемых платформ. После этого
удалить old binaries/installers/imports, сделать новый core единственным default
и убрать UI переключения.

Для проверки полноты инвентаря и pinned source:

```bash
python3 tools/audit-hiddify-migration.py
python3 tools/audit-hiddify-migration.py --upstream /path/to/hiddify-core
```

Скрипт не устанавливает и не запускает ядро; не изменяет БД, firewall или настройки.
