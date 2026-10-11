# Проверка функций, меню, подключений и настроек ядра

Ветка `update`, 2026-10-11. Ядро — модифицированный **sing-box** владельца
`SawaMEN/hiddify-core`, revision `b5749002266b9782803669138494ef6937eb4e95`,
версия `sawamen-b5749002266b-panel4`. Старое имя `hiddify-core` — alias.
Изменять источник ядра для этих исправлений не потребовалось.

## Что исправлено

1. Общие пункты **Исходящие** и **Маршрутизация** открывали Xray даже при выбранном
   sing-box. Теперь редактор выбирается по настройкам, прямые `/outbound` и
   `/routing` открывают соответствующую секцию sing-box. Ошибка чтения настроек
   выводится пользователю вместо бесконечной загрузки.
2. Импорт в редакторе sing-box не принимал HY2. Добавлены `hy2://` и
   `hysteria2://`: auth, IPv6, SNI, insecure, ALPN, SHA-256 pins, ECH,
   salamander/gecko, диапазоны и списки портов, legacy `mport`, hop interval.
   Параметры Gecko принимаются и в формате экспорта панели (`minPacketSize`,
   `maxPacketSize`). Неизвестные параметры и противоречивые/повреждённые значения
   дают явную ошибку. `security=tls` и `fp` из экспорта панели принимаются;
   TCP uTLS fingerprint не применяется к QUIC. Отдельный `vcn`, отличающийся
   от SNI, отклоняется.
3. Проверка реальным бинарником выявила, что каждый элемент `server_ports`
   должен быть диапазоном. Одиночные порты в списке преобразуются в `443:443`;
   конфликтующий `server_port` не добавляется. В форме появилась настройка
   диапазонов с согласованным переключением на одиночный порт.
4. Добавлен импорт `mieru://` (protobuf) и `mierus://` через чистый декодер SDK
   Mieru. Новый authenticated POST `/panel/api/setting/singbox/import-link`
   ничего не сохраняет и не применяет на машине. Сохранение остаётся отдельным
   действием редактора. Добавлены протокол Mieru, credentials, MTU, bindings,
   multiplexing и handshake в outbound-форму и проверку полей.
5. V2Ray API listen выглядел редактируемым, хотя генератор всегда заменял его
   адресом `127.0.0.1:10086`. Поле стало read-only с объяснением автоматического
   управления статистикой. Списки пользователей/тегов формирует панель.
6. Обновлены реестр API и обе копии OpenAPI. Исправлены ошибочные сообщения
   импорта VLESS/XHTTP: ограничение конкретного импортера больше не выдаётся
   за отсутствие поддержки в модифицированном ядре.

## Меню и функциональные области

Проверка сочетает чтение routes/sidebar/API и автоматические тесты. Это не
ручной браузерный прогон каждого действия и не доказательство корректности
каждой функции при всех возможных входных данных.

| Пункт | Что проверено | Граница проверки |
|---|---|---|
| Главная | route, статус ядра, canonical identity, статистика и websocket/backend tests | без работающего production-ядра |
| Входящие | формы/схемы, CRUD, клиенты, TLS, transport, порты и конфликты, генерация конфигурации | без передачи трафика по каждому протоколу |
| Клиенты | CRUD/attach, enable, квоты/traffic, ссылки, QR, настройки отдельных связей | внешние приложения клиентов не запускались |
| Группы | API/controller/service tests и связи пользователей/inbound | без ручного полного UI сценария |
| Узлы | identity/status, credentials, sync, remote config, frontend row/history tests | без соединения с удалённым узлом |
| Hosts | schema, адреса/порты, ссылки и subscription fan-out | без проверки внешних DNS/CDN |
| Firewall | правила/владение, Mieru TCP/UDP bindings, конфликт портов | реальные правила хоста не изменялись |
| Исходящие | выбранное ядро, импорт/редактирование/сохранение, validation, группы и ссылки на теги | latency/handshake удалённых серверов не измерялись |
| Маршрутизация | выбранное ядро, reference cleanup, final/group/detour dependencies, DNS/native overlays | без реальной маршрутизации трафика |
| Telemt | route, общий backend/test suite и настройки sidecar | внешний telemt runtime не запускался |
| Настройки | general/security, Telegram/email/Discord, subscription formats/balancers, settings draft/save tests | сообщения во внешние сервисы не отправлялись |
| Ядро: Основное/DNS/Маршрутизация/Исходящие/Расширенные | чтение/сохранение native template, managed sections, validation-before-publication, API статистики, editor tests | не все возможные expert JSON комбинации |
| Ядро: Adblock/Gateway | отдельные routes, controls, profiles/overlays, network lifecycle/rollback tests | без реального nft/TUN/reboot сценария |
| Выход | существующий route/action, auth/session/backend tests | без отдельного ручного браузерного сценария |

API contract test сопоставляет реальный Gin router с документированными
публичными операциями; внутренние UI-only операции исключаются по правилам
этого теста. Наличие route не доказывает работу внешней зависимости.

## Входящие протоколы: что подтверждено

| Группа | Подтверждение |
|---|---|
| VLESS, VMess, Trojan, Shadowsocks2022, HTTP, Mixed | полный конфиг из БД и `check -c` закреплённого ядра |
| Hysteria1/Hysteria2, TUIC, Naive, AnyTLS, ShadowTLS, Snell6, MASQUE | полный конфиг из БД и `check -c` закреплённого ядра; отдельные regression tests |
| TCP, WS, HTTP, gRPC, HTTPUpgrade, XHTTP | конфиг из БД и проверки адаптера/бинарника |
| TUN/WireGuard/tunnel | адаптеры, validation, existing Go/frontend suites; запуск интерфейсов требует проверки на хосте |
| Mieru | отдельный runtime `mita`; settings/credentials/render, port conflicts/firewall и export tests; native outbound import проверен реальным sing-box |
| MTProto, AmneziaWG, Pingtunnel, TrustTunnel, FPTN, OpenFlux, Sudoku, VK TURN proxy | отдельные локальные runtime; existing unit/service tests; бинарники и интерфейсы должны быть доступны на хосте |

HY2 inbound требует правильного TLS сертификата/ключа и открытого UDP порта;
auth и obfs должны совпадать с клиентом. Mieru inbound не попадает в native
sing-box config: панель запускает `mita` отдельно. Локальные Mieru bindings
валидируются в диапазоне 1025–65535, удалённые outbound-порты — 1–65535.

**Вставка клиентской URI не создаёт входящий сервер.** Для подключения к
чужому HY2/Mieru серверу: **Исходящие → Импорт ссылки → Добавить → Save**.
Для собственного сервера: **Входящие → Добавить → Hysteria (v2) / Mieru**,
задать пользователей, TLS для HY2, TCP/UDP bindings для Mieru.

Mieru importer представляет один профиль с одним сервером. Несколько профилей
или серверов, hash-only password, traffic-pattern, proxy dialer и
`MULTIPLEXING_OFF` отклоняются явно: текущие native options закреплённого ядра
не позволяют перенести их без изменения смысла. Можно экспортировать отдельный
`mierus://` для нужного сервера; OFF/traffic-pattern требуют доработки ядра.
Локальные SOCKS/HTTP/RPC порты из полного client config не импортируются: здесь
создаётся outbound панели, а не отдельный клиент Mieru.

Импортер ссылок VLESS/Trojan остаётся ограниченным для сложных расширений;
XHTTP/VLESS encryption доступны через native JSON и существующие адаптеры,
но не через этот простой URI importer. Ошибка сообщает об этом явно.

## Проверки

- Полный `go test ./... -count=1 -timeout=180s` с owner binary для config checks.
- Отдельные регрессии controller/service/web/hiddify/singbox/mieru/sub:
  импорт обоих Mieru форматов, HTTP handler, реальные HY2/Mieru JSON checks,
  полный inbound/transport matrix, route registry contract.
- Race для новых import handlers/service и Mieru inbound/port regressions.
- Полные frontend unit/components suites: **151 файл, 1548 тестов**;
  дополнительные editor/import/menu проверки: **45 тестов**; фактически импортированный HY2 JSON передан owner binary `check`.
- TypeScript, oxlint, production frontend build, `go vet ./...`, сборка панели,
  форматирование изменённых файлов и `git diff --check`.

**Эксплуатационная приёмка открыта.** `check -c` проверяет JSON и создание
объектов ядра, но не handshake/трафик. `srun` в данном окружении не может создать
netlink socket (`operation not permitted`). Для утверждения «все входящие точно
работают» требуется привилегированный Linux host, сертификаты, реальные клиенты,
TCP/UDP/TUN/Gateway, sidecar binaries и reboot. Storybook browser tests здесь
не выполняются из-за отсутствующего Playwright Chromium; component tests — jsdom.
Live PostgreSQL и реальные внешние сервисы также не проверялись.
