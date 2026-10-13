# MASQUE (sing-box 1.15+)

MASQUE inbounds use the native `masque-server` CONNECT-IP endpoint. MASQUE is
available in the inbound picker only when sing-box is selected and its installed
version is 1.15+; prereleases must be 1.15.0-alpha.7 or newer.

The form automatically selects a free port, enables HTTP/3 with HTTP/2 and
HTTP/1.1 compatibility, supplies the standard URI template, IPv4/IPv6 tunnel
prefixes and MTU 1280, and uses the internal network stack. It requires no system
TUN device or additional process. Client usernames are their panel emails;
passwords use the ordinary client credential lifecycle. Empty certificate/key
paths select the panel HTTPS certificate, then the subscription HTTPS certificate.
If neither exists, configure a certificate pair explicitly.

The automatic settings button resets tunnel parameters without changing clients
or TLS credentials. Advertised routes may be left empty to allow all destinations.
Custom prefixes, routes and URI templates are validated on the server.
Path templates support `target` and `ipproto` in path or query expressions, for
example `/tunnel{?target,ipproto}`. Basic Auth usernames cannot contain a colon
or control characters and must be unique within the panel identity namespace. HTTP/3 binds UDP; HTTP/1.1
and HTTP/2 bind TCP on the same port. Port conflicts respect selected versions.

Use a **sing-box JSON subscription** with a client based on a MASQUE-capable
sing-box core. The profile contains `masque-client` under `endpoints` and routes
to its tag. The highest enabled HTTP version is preferred regardless of form
selection order, with automatic fallback to lower versions. Host TLS overrides
(SNI, explicit blank SNI, insecure and ALPN) are retained. Standalone profiles
bootstrap the endpoint server through a local DNS resolver so tunneled DNS does
not create a startup dependency loop. No proprietary `masque://` share URI is invented. Xray and Clash
subscription formats cannot represent this endpoint. Import support in other
applications depends on their own core version and endpoint importer.

Native sing-box editor settings cannot overwrite generated MASQUE endpoints.
Disabling clients or deleting the inbound updates the native runtime through the
same restart path used by the other sing-box protocols.

Validation:

```sh
go test ./internal/masque ./internal/singbox ./internal/web/service ./internal/sub
SINGBOX_TEST_BINARY=/path/to/sing-box go test ./internal/singbox -run MASQUE
cd frontend
npm run typecheck
npm run test -- src/test/masque.test.ts
npm run build:ci
```

The optional binary tests validate both client and server JSON for HTTP/1,
HTTP/2 and HTTP/3. They also relay TCP and UDP over CONNECT-IP on loopback when
the host permits the sing-box network monitor; restricted netlink environments
report those connection checks as skipped.

An outgoing MASQUE connection can also be configured as a `masque-client` in
`endpoints` through the native sing-box editor, and targeted by its tag in routing.
It must not be placed in the Xray outbound editor or native `outbounds` array.

References:

- https://sing-box.sagernet.org/configuration/endpoint/masque-server/
- https://sing-box.sagernet.org/configuration/endpoint/masque-client/

## Панель: автоматическая настройка

В форме inbound доступны стандартный пресет, только QUIC/HTTP/3 и TCP/HTTP/2
с совместимостью HTTP/1.1. Смена пресета сохраняет пользователей, пароли и TLS.
Форма объясняет, какие TCP/UDP-порты открыть. Собственные сертификаты и сетевые
параметры сгруппированы в раскрывающихся секциях. Кнопка автоматического выбора
сертификата очищает оба пути одновременно: сервер затем выбирает сертификат
панели или подписок. Проверка наличия сертификата выполняется на стороне сервера.

В `advanced` сохраняются дополнительные параметры ядра: таймауты UDP/HTTP,
keepalive, окна приёма, количество потоков, начальный размер QUIC-пакета,
определение MTU пути, сетевой интерфейс и параметры сокетов. Неизвестные ключи
и попытки заменить TLS, пользователей, порт или tag отклоняются. Эти настройки
применяются к серверу; клиентская подписка сохраняет собственную конфигурацию.

`masque-client` доступен в списке типов редактора исходящих и в Endpoints.
Форма содержит имя пользователя, пароль, CONNECT-IP URI, HTTP-версию,
автоматический переход к меньшей версии HTTP, MTU, SNI, маршруты и подключение
по требованию. Прочие поддерживаемые параметры доступны через редактор JSON.

## WARP через MASQUE

Кнопка **Добавить WARP через MASQUE** в исходящих добавляет нативный outbound
`type: masque` со свободным tag и включает `experimental.cache_file.enabled`
и `store_masque_config`. При запуске ядро само создаёт отдельное устройство
Cloudflare, регистрирует ключ и сохраняет профиль. При следующих запусках профиль
повторно используется. Сохранение настроек также включает кэш для вручную
добавленных WARP-MASQUE подключений. Уже настроенный WireGuard WARP сохраняется.

В форме выбираются HTTP/3 или HTTP/2 и IPv4/IPv6, прокси для регистрации,
UDP keepalive, таймаут и задержка переподключения. Удаление outbound из панели
не удаляет устройство Cloudflare. Регистрация требует доступности Cloudflare API;
её ошибки отображаются в журнале ядра. Новое подключение выбирается в маршрутизации
после сохранения конфигурации. Панель не перенаправляет существующие маршруты
автоматически.

Конфигурации стандартного MASQUE проверены командой `check` опубликованного
ядра для HTTP/1, HTTP/2 и HTTP/3. WARP-MASQUE проверен для HTTP/2 и HTTP/3.
Сама регистрация Cloudflare не выполняется в тестах.
