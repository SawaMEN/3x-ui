# Интеграция собственного sing-box: этапы 2–9

Ветка `update`. Ядро — модифицированный **Sing-box**.
Точный build lock: [hiddify-core-target.json](hiddify-core-target.json).
Предыдущий v5 lock сохранён отдельно в [hiddify-core-v5-baseline.json](hiddify-core-v5-baseline.json).

## Идентификация ядра

Ядро — модифицированный **Sing-box**. В настройках, API, статусе
процесса, сессиях и данных узлов оно определяется как `sing-box`.
Выбор `sing-box` запускает закреплённую модифицированную сборку и
использует её расширенные адаптеры конфигурации, статистики и Gateway.
Старое сохранённое значение `hiddify-core` читается как `sing-box`; при
сохранении записывается каноническое имя. Отдельного варианта Hiddify в UI нет.
Обновление штатным upstream-бинарником через панель запрещено: форк собирается
и поставляется вместе с панелью. Исходники форка можно модифицировать.

Имена репозиториев, внутренних пакетов, build lock, бинарника и файла
конфигурации сохранены для совместимости. Они не задают тип ядра панели.
Исторические отчёты этапов отражают названия на момент выполнения этапа.

Исправление идентификации проверено тестами чтения и сохранения старых
настроек, выбора бинарника Sing-box, HTTP-статуса, сессий Clash, трафика и Gateway.
Прошли регрессионные Go-тесты, race-тесты, `go vet`, frontend typecheck,
9 frontend-тестов, lint и сборки панели и интерфейса. Настоящий закреплённый
бинарник проверяет конфигурации через `check -c`; ограничения сетевой
приёмки, описанные в отчёте этапа 8, сохраняются.

## Общие модели панели

Этап 3 вынес трафик и структуру inbound в `internal/coremodel`. БД,
подписки, лимиты и синхронизация узлов используют общие модели напрямую.
Таблицы, идентификаторы и контракты JSON сохранены. Xray wire-сериализация
остаётся в адаптере; общие DTO не подменяют native-конфиг Hiddify.
Проверки: [hiddify-core-stage-3-verification.md](reports/hiddify-core/hiddify-core-stage-3-verification.md).

## Генерация полного серверного конфига

Этап 4 подключил `HiddifyConfigGenerator`: единый DB snapshot для
inbounds/users/settings/subscriptions и egress узлов, общие native builders, Hiddify-aware template merge, граф ссылок DNS/routing/rule sets,
канонические пользователи и стабильные статистические наборы. Preview,
candidate и restart используют один entrypoint. Полные DB-конфиги 14 вариантов
протоколов и 6 транспортов проверены настоящим `check -c`.
Infrastructure, AdBlock и managed YouTube используют тот же снимок без
дополнительного чтения БД. Preflight проверяет циклы DNS/inbound/outbound
и ограничения DNS-групп; отсутствующий egress target возвращает ошибку.
Настоящее ядро проверено на успешной публикации и отказе candidate с
сохранением старого файла.
Подробности: [hiddify-core-stage-4-verification.md](reports/hiddify-core/hiddify-core-stage-4-verification.md).

## Применение изменений панели

Этап 5 добавил scopes для local native mutations и массовых клиентских
операций. Последний scope запрашивает один apply зафиксированной БД; native
update/enable/user changes используют полный конфиг. Фоновый apply проверяет
очередь раз в секунду, ждёт завершения пачки, пропускает неизменившийся конфиг
и сохраняет ручной Stop. Неудачный запуск восстанавливает предыдущий applied
config и процесс; desired state остаётся для retry.

Raw `srun` не предоставляет client RPC `/hcore.Core/HotReload`, поэтому
применение использует контролируемый restart без объявления HotInboundReload.
Проверки и границы: [hiddify-core-stage-5-verification.md](reports/hiddify-core/hiddify-core-stage-5-verification.md).

## Runtime и управление процессом

`internal/hiddify.Runtime` реализует `internal/core.Runtime` и используется
service lifecycle панели. Ядро запускается отдельным процессом через `srun -c`
с абсолютными путями. Процесс переживает отмену запроса, проверяется по binary,
config и команде `srun`; клиентский `run` не принимается как серверный runtime.
Stop выполняет ограниченное по времени завершение и при уже отменённом контексте
панели. Watchdog восстанавливает процесс после сбоя; ручная остановка сохраняется.

`version` обязан сообщить `sawamen-b5749002266b-panel7`, полный source revision и точные
обязательные tags. Проверка больше не принимает совпадение tag по подстроке.
Неподходящая сборка отклоняется с ошибкой hiddify-core. Выбор sing-box возвращает
capabilities интеграции, включая Clash, rulesets и статистику, без обещания HotReload.

Новый конфиг проверяется через `check -c` до замены файла и остановки процесса.
Проверка без candidate config запрещена. AppliedConfig описывает запущенный
конфиг, а не отредактированный файл. Журнал пишется в `sing-box.log`.
Имена binary/config отделены от sing-box и учитывают `XUI_BIN_FOLDER`.

## Сборка из собственного форка

```bash
bash tools/build-hiddify-core.sh /path/to/panel/bin
```

Сборщик получает **полный commit**, а не main/latest/draft; инициализирует
зафиксированные gitlinks, проверяет источники и вложенные submodules, применяет
panel overlays. Go для ядра закреплён на 1.26.8 независимо от Go панели.
Есть with_v2ray_api, with_awg, with_quic, with_wireguard, with_utls,
with_clash_api, with_masque и остальные tags в lock.

Overlay check добавляет native decode/constructor validation без Start.
Контекстный Clash patch помещает аутентифицированного пользователя именно в
metadata.user. Сборка на host architecture выполняет тест сериализации этого
поля, version/revision и smoke check. Отдельный executable не требует
hiddify-core.so. Для Naive outbound по-прежнему нужна соответствующая libcronet;
эта зависимость не заменяется фиктивным успешным constructor check.
Release workflow и Docker используют тот же сборщик.

Сборка panel4 дополнительно применяет contextual sniff override patch к
собственному форку: `override_destination` в modern action подключён к
существующему механизму перезаписи адреса. Decode/native action тест запускается
в сборщике. Поэтому сборка этапа 2 без этого mapping больше не принимается
за текущий профиль.

## API статистики

Клиент Hiddify закрепляет метод
`/v2ray.core.app.stats.command.StatsService/QueryStats`, соответствующий реально
зарегистрированному ServiceName форка. Zero-value клиент sing-box сохраняет
прежний experimental.v2rayapi контракт. Автоперебора методов нет: повтор запроса
со сбросом счётчиков мог бы потерять данные. Wire payload объявляется обычным
protobuf (`proto`), чтобы стандартный gRPC-сервер принимал запрос.

Пользовательские счётчики связываются с metadata.User и stats.users. Этап 6
использует `Reset=false`, durable контрольные точки и транзакционное начисление
дельт. Перезапуск панели, повторный ответ и DB retry не повторяют начисление.
Квоты, expiry/autorenew и session revocation используют зафиксированное состояние.
Clash adapter общий для service и local runtime; online/IP отражают его снимок.
Проверки и открытая приёмка сохранения последних байтов при остановке ядра:
[hiddify-core-stage-6-verification.md](reports/hiddify-core/hiddify-core-stage-6-verification.md).

## Границы этапа

Автоматический перенос пользователей на новое ядро, смена ядра по умолчанию и удаление Xray/sing-box
не включены: это последующие этапы. Config translation, nodes, subscriptions,
Gateway и sidecar bridges продолжают использовать имеющиеся адаптеры панели.
Ограничения ML-DSA/seed, mKCP, Xray session/fallback/sockopt/TLS, peer metering и
полный паритет дополнительных протоколов сохраняются до проверки каждого переноса.

Проверки второго этапа и их фактические результаты записаны в
[hiddify-core-stage-2-verification.md](reports/hiddify-core/hiddify-core-stage-2-verification.md).

Повторная проверка и исправления этапов 1–5: [отчёт](reports/hiddify-core/hiddify-core-stages-1-5-review.md).

## Этап 7: подписки и удалённые узлы

Hiddify native JSON использует адаптер собственного ядра и сохраняет XHTTP.
Версия, состояние и capabilities выбранного ядра передаются heartbeat и деревом
узлов; Hiddify больше не записывается в статус sing-box. UUID, пароли, SubID,
импортированные URL и прежние клиентские форматы сохраняются.

Реализованные сценарии, проверки и открытая эксплуатационная приёмка:
[отчёт этапа 7](reports/hiddify-core/hiddify-core-stage-7-verification.md).

## Этап 8: Gateway и дополнительные сервисы

Gateway interception следует native Start/Stop/restart и восстановлению процесса;
boot restore использует общий runtime lease. Hiddify генерирует scoped Gateway
DNS rule и переводит настройки TUN с защитой от маршрутизационных петель.
FPTN bridge запускает закреплённый модифицированный Sing-box вместо отдельного sing-box.

Проверки, ограничения совместного TUN/TPROXY и открытая приёмка реального
сетевого трафика: [отчёт этапа 8](reports/hiddify-core/hiddify-core-stage-8-verification.md).

## Этап 9: установленные панели

Добавлен offline CLI `core-migrate` и общий transactional helper установщиков.
Резервирование предшествует изменению схемы и файлов, переключение на
`sing-box` следует успешному trial start, ошибки восстанавливают предыдущую
БД/конфигурацию/установку. Незавершённый journal блокирует запуск панели.
Пользователи, секреты, подписки, узлы и записанная статистика сохраняются.

Инструкции и точные границы проверки:
[отчёт этапа 9](reports/hiddify-core/hiddify-core-stage-9-verification.md).
Полноценный privileged upgrade/traffic/reboot и live PostgreSQL restore
требуют отдельной эксплуатационной приёмки.

## Повторная проверка этапов 1–9

[Общий отчёт](reports/hiddify-core/hiddify-core-stages-1-9-review.md) содержит проверенные контракты,
исправления stale backups, legacy alias, runtime environment и rollback,
результаты регрессий и оставшуюся эксплуатационную приёмку.

## Этап 10: удаление старых управляемых ядер

Панель поставляет только закреплённый модифицированный Sing-box. Удалены standalone
процесс/API Xray и загрузчики Xray/stock sing-box, переключение ядра и старые
runtime страницы. Клиентские форматы подписок и legacy чтение для миграции
сохранены. Старые рабочие бинарники удаляются после успешной проверки запуска;
installation/database snapshot остаётся для восстановления.

[Изменения, миграция контейнеров и границы проверки](reports/hiddify-core/hiddify-core-stage-10-verification.md).

Новые параметры Vision и REALITY в сборке panel5 описаны в
[sing-box-protection-support.md](sing-box-protection-support.md).

VLESS Reverse, транспорты mKCP, расширения XHTTP и динамические балансировщики panel6 описаны в [отчёте](sing-box-transport-balancer-support.md).

## TLS cipher presets and inbound TCP FinalMask

The panel build with the `mask1` runtime suffix applies
`deploy/hiddify/inbound-finalmask.patch`. Update both the panel and its managed
HiddifyCli binary: older binaries do not recognize inbound `final_mask`.

Supported TCP listeners (including VLESS, VMess, Trojan, Shadowsocks, HTTP and
SOCKS) apply Xray's own TCP mask manager before
TLS and the transport handshake, including WebSocket, HTTPUpgrade, HTTP, gRPC
and XHTTP. Mask order and settings are preserved. UDP-based transports and
REALITY remain incompatible with TCP masks and fail validation. Protocol-native
Hysteria UDP masks continue through the existing translator. Invalid mask
settings are rejected by `HiddifyCli check` before opening listeners.

TLS translation uses the cipher names accepted by Go's `tls.CipherSuites()`.
Legacy panel presets containing a mixture of supported and insecure ciphers
retain their supported subset. An unknown name or a list containing only
insecure ciphers is rejected with instructions to replace the list or clear it
explicitly. Saved Xray settings are preserved. The Firefox preset no longer
adds obsolete RSA key-exchange ciphers.

The build runs real VLESS client/server round trips for fragment, header-custom
and sudoku masks, plus rejection tests. The panel's optional binary validation
suite checks TCP FinalMask with TLS and Hysteria2 with an old mixed cipher list.
