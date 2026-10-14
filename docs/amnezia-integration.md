# Amnezia VPN / AmneziaWG

## Экспорт клиентского подключения

В «Клиенты → QR / конфигурация → AmneziaWG» доступны `.conf` и **Amnezia VPN**.
Второй вариант создаёт `vpn://` с именованным JSON-профилем, сжатием zlib и
заголовком `qCompress`. Доступны копирование, файл `.vpn` и открытие в приложении.
QR использует совместимую с Amnezia серию частей: отсканируйте все части,
переключаясь кнопками. Ключи не отправляются внешнему сервису.

Профиль содержит только доступ клиента. В нём нет SSH-пароля, серверного
private key или учётных данных панели. Старые ссылки подписки с Base64URL
обычного `.conf` сохранены: Amnezia принимает и этот формат.

Формат сверялся с `amnezia-client` 5.0.3.0: `ImportController`,
`AwgProtocolConfig`, `qrCodeUtils` (magic 1984, части по 850 байт).
Amnezia 5.0.3.0 при импорте JSON-профиля переписывает MTU на значение своей
платформы (1280 для Android/iOS/MACOS_NE, 1376 для прочего desktop). Интерфейс
сообщает об этом; для точного сохранения MTU используйте `.conf`.
Проверка формата и обратного декодирования автоматизирована; ручной импорт
в Android/iOS Amnezia в этом изменении не выполнялся.

## Совместимость и настройки

В форме inbound выберите AWG 1.0, 1.5, 2.0, 3.0 или 3.1. Выбор генерирует
новый согласованный набор параметров. Для существующих клиентов показывается
предупреждение: после изменения необходимо заново выдать профили всем клиентам.
Расширенные параметры доступны в раскрывающемся блоке.

Совместимость определяется по используемым параметрам, а не версии приложения:
`I1–I5` требуют 1.5, `S3/S4` и диапазоны `H1–H4` — 2.0,
header protection / content padding / тайминги — 3.0,
`RandomTrailers` и `DisableCookies` — 3.1. Это минимальное поколение формата,
а не гарантия поддержки конкретной сборкой стороннего клиента.

В форме клиента можно переопределить `Jc/Jmin/Jmax`, `I1–I5` и MTU.
Остальные параметры, включая ключ header protection, заголовки и shared padding,
наследуются от inbound. Переопределения применяются ко всем AWG-подключениям
данного клиента. Они изменяют экспорт и подписки, сохраняя конфигурацию сервера.
Пустое числовое поле наследует значение; пустая строка `I1–I5` отключает
соответствующий signature packet. Кнопка сброса возвращает наследование.

API: `client.awgClientOptions` — объект с необязательными полями `jc`, `jmin`,
`jmax`, `i1`…`i5`, `mtu`. Отсутствие объекта сохраняет существующие значения,
`{}` сбрасывает переопределения. Данные сохраняются в БД и резервных копиях.

## Исходящий AWG непосредственно в ядре

В форме AWG outbound доступен переключатель **AWG непосредственно в ядре**.
Он сохраняет `settings.native=true`, переводит профиль в endpoint `type=awg`
и использует nested `awg` options формата SawaMEN/hiddify-sing-box.
Tag, маршруты, detour и peer credentials сохраняются.

Для перехода поле DNS должно быть пустым: DNS задаётся в ядре. Listen Port
должен быть 0, чтобы новый endpoint выбрал свободный порт. Несовместимые поля
отклоняются при сохранении; они не теряются молча. После успешного запуска
ядра прежний мост этого tag останавливается. До успешного запуска ядра прежний мост остаётся доступным; неуспешная
проверка или запуск не передают владение туннелем. Периодический reconciler
не создаёт второй движок для активного нативного outbound.

Существующие outbounds сохраняют SOCKS-мост по умолчанию. Входящие AWG
продолжают работать через встроенный движок панели: текущий network TUN
ядра не заменяет все функции серверного режима, port forwarding и IPv6 alias.
Обновлять или заменять ядро на amnezia-box для этой функции не требуется.

Генерация проверена релизом
`sawamen-22fb84fdd93a-3827d3a871dd-panel8`. Опциональный тест
`TestNativeAWGConnections` проверяет реальные TCP/UDP round-trip через AWG v3:

```bash
HIDDIFY_CONFIG_CHECK_BINARY=/absolute/path/hiddify-core-linux-amd64 \
  go test ./internal/web/service -run 'TestNativeAWG' -count=1
```

## Prometheus

Защищённый GET endpoint: `<basePath>/panel/api/server/amneziawgMetrics`.
В окне логов AWG есть кнопка **Prometheus**. Для внешнего scrape создайте API
token со scope `monitor`; используйте Bearer token и интервал 60 секунд.
Публичный отдельный порт или Redis не нужны.

```yaml
scrape_configs:
  - job_name: 3x-ui-awg
    scheme: https
    metrics_path: /YOUR_BASE_PATH/panel/api/server/amneziawgMetrics
    scrape_interval: 60s
    static_configs:
      - targets: ['panel.example.com']
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/3x-ui-monitor-token
```

Метрики: `awg_inbounds_configured`, `awg_inbound_running`,
`awg_inbound_peers`, `awg_inbound_received_bytes`, `awg_inbound_sent_bytes`,
`awg_current_online`, `awg_active_users{window="24h"|"30d"}`.
Они относятся к локальным inbound, без удалённых узлов. RX/TX — gauges
с начала работы device; после перезапуска они обнуляются. Online определяется
handshake за последние 180 секунд. Пользователь считается один раз независимо
от числа inbound.

`awg_active_users` использует сохранённый общий lastOnline клиентов,
прикреплённых к AWG: активность через другой их протокол тоже учитывается.
Это скользящие окна, не календарные DAU/MAU исключительно AWG.
В labels нет email, endpoint IP, адресов туннеля или ключей.
