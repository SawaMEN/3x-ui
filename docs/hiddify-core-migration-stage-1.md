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
| hiddify-core | `v5.0.0`, commit `7854a77ec228309e2194bdfbec1efb5b7190aa5b` |
| hiddify-sing-box, submodule | `965805340f3f399a48626d3facc355a906c95ebd` |
| ray2sing, submodule | `505d8c645f1dad54544abd024ffd8100a8d771e8` |
| Проверенный архив | `hiddify-lib-linux-amd64.tar.gz` из релиза `v5.0.0` |
| SHA-256 архива | `bb3925263992b43227dc9f6602f18f5f827a73efecbdb601f58ae6596467da33` |
| Вывод бинарника | hiddify-core `v5.0.0`, hiddify-sing-box `1.13.0-rc.2`, Go `1.26.3`, linux/amd64, CGO enabled |

Не использовать `latest` при установке целевого ядра. Go module заменяет sing-box локальным submodule; версия из `go.mod` не описывает
весь фактический runtime. Зафиксировать оба submodule и параметры сборки.
ARM64, musl и другие платформы ещё не проверены; amd64 checksum нельзя применять
к их архивам. Проверенный архив содержит `HiddifyCli` и `lib/hiddify-core.so`: CLI зависит от
библиотеки. Архива standalone `hiddify-core-linux-amd64.tar.gz` у v5.0.0 нет.
Панельная сборка ниже создаёт отдельный executable; Naive outbound требует
соответствующий libcronet runtime.

Первичные источники, все на зафиксированной ревизии:

- [Релиз v5.0.0](https://github.com/hiddify/hiddify-core/releases/tag/v5.0.0).
- [go.mod](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/go.mod), [.gitmodules](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/.gitmodules).
- [Registry inbounds/outbounds/endpoints](https://github.com/hiddify/hiddify-sing-box/blob/965805340f3f399a48626d3facc355a906c95ebd/include/registry.go), [QUIC](https://github.com/hiddify/hiddify-sing-box/blob/965805340f3f399a48626d3facc355a906c95ebd/include/quic.go).
- [CLI srun](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/cmd/cmd_sing_run.go), [неэкспортированный check](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/cmd/cmd_check.go).
- [Core gRPC](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/v2/hcore/hcore_service.proto), [raw config](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/v2/hcore/buildconfighelper.go).
- [Транспорты client/server](https://github.com/hiddify/hiddify-sing-box/blob/965805340f3f399a48626d3facc355a906c95ebd/transport/v2ray/transport.go), [их JSON-схема](https://github.com/hiddify/hiddify-sing-box/blob/965805340f3f399a48626d3facc355a906c95ebd/option/v2ray_transport.go).
- [Сборка release CLI](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/.github/workflows/build.yml), [Makefile для библиотек](https://github.com/hiddify/hiddify-core/blob/7854a77ec228309e2194bdfbec1efb5b7190aa5b/Makefile).

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
| `vless` | Перенос в `vless` inbound; UUID, flow и decryption. В `internal/hiddify` добавлен отдельный mapping encryption/decryption. Vision seed и ML-DSA остаются ограничениями. |
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
| `mieru` | Native inbound; в v5 Mieru v3.38.0 совпадает с панелью. Нужен mapping username/password/portBindings и проверка metering перед заменой sidecar. |
| `wireguard` | Адаптация к endpoint `wireguard` с listen_port, address, private_key, peers. Это endpoint, а не `wireguard` inbound. Сохранить peer/allowed_ips и отдельный учёт клиентов. |
| `amneziawg` | Release library включает with_awg; endpoint awg создаётся, но нужен TUN. Target использует fork amneziawg-go revision 70b344f2f78b, панель — /v3; проверить AWG 3.1/peer metering перед заменой embedded amneziawgnet. |
| `tun` | Native `tun` inbound; нужно пересобрать конфиг интерфейса и policy routing. Существующий TranslateXrayInbound его отклоняет. Привилегии/маршруты принадлежат gateway runtime панели. |
| `tunnel` | Блокер прямого переноса Xray tunnel/dokodemo-door. Hiddify `tunnel_server`/`tunnel_client` — другой протокол с UUID/key и вложенным listener. Для forwarding проектировать `direct` inbound + route; для transparent proxy — redirect/tproxy, не переименовывать тип механически. |
| `mtproto` | Внешний Telemt: сохранить reload, quota/expiry, webproxy, секреты и subscription reconciliation; его egress/bridge перевести на hiddify-core. Native MTProto inbound не найден. |
| `pingtunnel` | Внешний runtime `internal/externalvpn`; сохранить TCP/UDP bridge, supervisor, настройки и трафик. Native inbound не найден. |
| `trusttunnel` | Внешний runtime `internal/externalvpn`; сохранить auth/TLS и bridge. Native inbound не найден. |
| `fptn` | Внешний runtime `internal/externalvpn`; сохранить token/MTU/SNI и bridge. Native inbound/outbound не найдены. |
| `openflux` | Внешний runtime `internal/externalvpn`; сохранить secret/context/transports и bridge. Native inbound/outbound не найдены. |
| `vk-turn-proxy` | Сохранить существующий внешний runtime и WireGuard reconciliation/relay; заменить только зависимость от старого ядра. Native inbound не найден. |
| `sudoku` | Сохранить `internal/sudoku` и его credentials/traffic bridge. Native inbound не найден. |
| `snell` | Native inbound 5/6 и outbound 4/6 зарегистрированы в v5. Существующий panel mapping users/userkey/PSK/obfs_mode/mode применим. Handshake и per-user metering ещё требуют проверки. |
| `masque` | Native masque-server/masque-client endpoints зарегистрированы. Добавлен entrypoint адаптера к текущему panel mapping address/users/TLS/advertise_routes; routing и учёт требуют сетевого теста. |

Специализированные сервисы не заменяют целевой hiddify-core на другое общее
proxy-ядро: они уже обеспечивают отдельные протоколы. Если требуется удалить и
их, это дополнительные порты реализаций, а не возможность upstream v5.0.0.

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
| XHTTP | Реальные client/server реализации есть. Добавлен отдельный mapping camelCase → snake_case, extra, session aliases и XMUX в internal/hiddify. Неизвестные значимые настройки (включая новые Xray sessionIDTable/sessionIDLength/serverMaxHeaderBytes) блокируются; пустые editor defaults не мешают. Добавлен отдельный download mapping для явно заданных address/port, XHTTP и TLS/REALITY; неподдерживаемые download опции блокируются. |
| mKCP | В transport registry отсутствует. Блокер для existing KCP configs; QUIC или XHTTP не эквивалентны. |
| Xray QUIC transport | Native QUIC transport есть с `with_quic`, но его options пусты. Xray security/key/header не переносить как native QUIC: конфиг с такими полями блокировать. |
| TLS | Native inbound TLS, PEM/path, SNI/ALPN, min/max version, ciphers и ECH. Проверить сертификаты на целевом JSON schema; Xray rejectUnknownSni, pinning и несколько certificate identities текущий переводчик отклоняет. |
| REALITY | Native reality handshake/private_key/short_id, клиентские utls/public_key/short_id. Проверять допустимую пару protocol/transport, не переносить UI eligibility как runtime capability. |
| REALITY ML-DSA / mldsa65Seed | Поля в целевой TLS схеме нет. Текущий sing-box адаптер пропускает seed; миграционный адаптер должен сообщать об изменении защиты и блокировать автоматический перенос такого конфига. |
| VLESS Vision | Базовый flow `xtls-rprx-vision` есть; проверить TCP/TLS и TCP/REALITY на реальном клиенте. Xray Vision seed и XHTTP+VLESS encryption не подтверждены. |
| VLESS encryption / ML-KEM | В v5 добавлены реальные VLESS decryption/encryption и CLI vlessenc. Адаптер сохраняет серверное decryption и клиентское encryption (flat/vnext); существующий sing-box переводчик остаётся строгим. Handshake с полученными ключами ещё не проверен. |
| VLESS fallback | В native VLESS inbound поля fallbacks нет. Блокер для existing fallback цепочек. |
| Trojan fallback | `fallback`/`fallback_for_alpn` есть; Xray name/path/xver, UNIX destination и неоднозначные дубликаты — блокеры. |
| FinalMask TCP, XMC/XOR и прочие маски | В v5 FinalMask реализован для outbound DialerOptions final_mask; адаптер сохраняет TCP/UDP список. Native inbound ListenOptions не имеет общего final_mask. Для inbound TCP masks остаётся ограничение. quicParams accepted-but-unused в upstream: meaningful настройки блокируются адаптером, а не теряются. |
| Hysteria2 salamander | Перенести в native `obfs` с password; handshake-тест обязателен. |
| Hysteria2 gecko | В v5 появился native gecko с min_packet_size/max_packet_size. Существующий panel mapping применим; probe дошёл до проверки TLS вместо ошибки JSON. |
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
| Native bridge/Snell/MASQUE | В v5 есть bridge и Snell, MASQUE endpoints и optional outbound. Использовать правильную registry/schema/build tags; FPTN/OpenFlux и другие external outbounds — через существующий bridge. |
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
| Native JSON editor | Целевая схема закреплённого 1.13.0-rc.2 fork, а не универсальная sing-box latest. Проверка endpoints/services/experimental плюс согласование DB-owned listener tags обязательны. |

## Статистика и управление runtime

| Контракт | Рабочий способ / конкретное ограничение |
| --- | --- |
| Start/stop/restart/version | `hiddify-core srun -c <absolute-config>` для panel-owned raw server config. `run` использует клиентский builder/Setup, это другой entrypoint. Адаптировать process identity (сейчас ожидает run), сигнал SIGTERM/SIGHUP и applied config snapshot. |
| Проверка перед применением | CLI check не зарегистрирован в upstream. Добавлен overlay cmd_xui_check.go.in и закреплённая сборка: check -c использует decode + box.New + Close, без Start/listener binding; config flag обязателен. Существующий running runtime нельзя останавливать до успешного check. |
| Hiddify Core gRPC | Start/Stop/Restart/HotReload, logs/outbounds/system info есть. Raw config через EnableRawConfig; Xray HandlerService не предоставляется, client builder не используется для сервера. |
| User/inbound/outbound bytes | Проверенный v5 amd64 release library НЕ включает with_v2ray_api. Панельная source-сборка ниже включает его; stats lists и QueryStats через /experimental.v2rayapi.StatsService/QueryStats. Нельзя заменять per-user quota общим CoreInfo. |
| Идентичность пользователя | VMess/VLESS/Trojan/SS и другие inbound должны отдавать стабильный metadata.User. У Naive/Mieru username вместо name, у WireGuard peer identity: это требует mapping, не гарантировано наличием счётчика. |
| Лимиты, expiry, autoreset и долговечный учёт | Сохранить job/DB transaction path, crash/restart baseline и disabled client filtering; снять последние counters перед остановкой. QueryStats reset должен учитывать потерю ответа и повторы. |
| Online/IP/session kill | Clash API /connections с secret и loopback; текущие panel client/session helpers адаптировать к фактическим metadata и endpoint-сессиям. Hiddify CoreInfo общая статистика не заменяет user quota. |
| Hot user/inbound mutations | В v5 есть Core.HotReload и box.HotReload для inbounds/outbounds/endpoints/DNS/route. Изменение TUN/неподдерживаемых частей требует restart; HOT_RELOAD_FAILED не выключает текущий runtime. Требуется привязка API панели и проверка session disconnect. |
| Внешние протоколы и endpoints | Сохранить Telemt/AWG/TUIC/relay счётчики до доказанного native per-user metering. Не складывать native и bridge bytes дважды. |
| Makefile против release workflow | Makefile библиотек включает with_awg/with_grpc, но не with_v2ray_api; CLI release workflow включает with_v2ray_api, но не with_awg. Реально опубликованный amd64 artifact — library build. Панельная сборка явно фиксирует нужные tags и не выводит capabilities из номера версии. |

## Что реально проверено

Скачан официальный v5 amd64 library archive, checksum совпал с GitHub API digest.
`version`, `--help`, `srun --help` выполнены; check отсутствует в upstream.
Изолированные raw config probes дали:

| Probe | Наблюдение |
| --- | --- |
| VLESS XHTTP | JSON/constructor приняты; Start остановлен create netlink socket: operation not permitted. |
| Snell6 | JSON/constructor приняты; то же ограничение Start. |
| Bridge outbound | JSON/constructor приняты; то же ограничение Start. |
| V2Ray stats | v2ray api is not included in this build, rebuild with -tags with_v2ray_api. |
| AWG endpoint | Реальная попытка создать TUN; /dev/net/tun отсутствует. |
| MASQUE server endpoint | Тип найден, конструктор требует TLS для HTTP/3. |
| Hysteria2 gecko sizes | JSON принят, конструктор требует TLS. |

Эти probes не доказывают handshake, счётчики или работающий gateway. Для нового
адаптера выполняются Go tests на сохранение encryption/XHTTP/FinalMask, неизменность
исходного persisted config, запрет потери защиты и строгий sing-box fallback.

## Доработки для panel build

`tools/build-hiddify-core.sh [output-directory]` получает только закреплённый tag,
проверяет commit/submodules/source anchors и добавляет локальный overlay `check`.
Сборка включает with_v2ray_api + with_awg, QUIC, WireGuard, uTLS, Clash API и Naive.
Toolchain закреплён на Go 1.26.3: попытка использовать Go 1.27.1 собрала executable,
но привела к startup panic Psiphon TLS (ConnectionState struct mismatch).
Скрипт выполняет smoke version/check со stats на host architecture до выдачи результата.
Executable не зависит от hiddify-core.so. Libcronet для Naive outbound поставляется
отдельно и должен соответствовать pinned fork. Скрипт не устанавливает бинарник
на сервер и не меняет coreType/БД/firewall.

`internal/hiddify` содержит target-specific переводчики inbound/outbound и MASQUE
endpoint. Они переиспользуют общий mapping sing-box, но расширения Hiddify включаются
только явно. Обновление persisted DB и runtime dispatch — следующий этап, adapter
ещё не активирован в работающей панели.

Панельный native linux/amd64 executable собран на Go 1.26.3. Семь проверок настоящим
check прошли: encrypted XHTTP inbound + user stats; Snell6; encrypted outbound +
FinalMask; MASQUE server + TLS; XHTTP separate download + TLS; Hysteria2 gecko + TLS;
отказ check без candidate config. Это schema/constructor checks без Start и без
проверки переданного трафика. go test для internal/hiddify, internal/singbox, internal/masque, компиляция
internal/snell и go vet internal/hiddify также выполнены.

```bash
bash tools/build-hiddify-core.sh /path/to/output
HIDDIFY_CONFIG_CHECK_BINARY=/path/to/output/hiddify-core-linux-amd64 go test -v ./internal/hiddify
```

## Результат этапа и вход в следующий

Этап 1 завершён как аудит: целевая версия и snapshot зафиксированы; каждому
протоколу и функциональному контракту указан путь переноса либо ограничение.
Это не разрешение на переключение работающего сервера: stock v5 amd64 library не обеспечивает
per-user stats API этой панели; полная совместимость требует integration gates.

Следующий этап: реализовать `internal/hiddify` runtime и validation entrypoint
поверх pinned source, выделить config/stats interfaces из Xray, затем выполнять
mapping и integration fixtures. Для текущего набора функций потребуется
panel-specific hiddify-core build (check/stats), mapping и решения для оставшихся
несовместимых защитных опций либо сохранение конкретных внешних protocol runtimes. Все
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
