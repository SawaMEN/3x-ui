# Интеграция hiddify-core: этапы 2–4

Ветка `update`. Ядро собирается только из `SawaMEN/hiddify-core`:
`b5749002266b9782803669138494ef6937eb4e95`, зависимый `SawaMEN/hiddify-sing-box`:
`8a82b70836022a1b56eca15df12f56f903cfb27d`.
Точный build lock: [hiddify-core-target.json](hiddify-core-target.json).
Предыдущий v5 lock сохранён отдельно в [hiddify-core-v5-baseline.json](hiddify-core-v5-baseline.json).

## Общие модели панели

Этап 3 вынес трафик и структуру inbound в `internal/coremodel`. БД,
подписки, лимиты и синхронизация узлов используют общие модели напрямую.
Таблицы, идентификаторы и контракты JSON сохранены. Xray wire-сериализация
остаётся в адаптере; общие DTO не подменяют native-конфиг Hiddify.
Проверки: [hiddify-core-stage-3-verification.md](hiddify-core-stage-3-verification.md).

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
Подробности: [hiddify-core-stage-4-verification.md](hiddify-core-stage-4-verification.md).

## Runtime и управление процессом

`internal/hiddify.Runtime` реализует `internal/core.Runtime` и используется
service lifecycle панели. Ядро запускается отдельным процессом через `srun -c`
с абсолютными путями. Процесс переживает отмену запроса, проверяется по binary,
config и команде `srun`; клиентский `run` не принимается как серверный runtime.
Stop выполняет ограниченное по времени завершение и при уже отменённом контексте
панели. Watchdog восстанавливает процесс после сбоя; ручная остановка сохраняется.

`version` обязан сообщить `sawamen-b5749002266b-panel4`, полный source revision и точные
обязательные tags. Проверка больше не принимает совпадение tag по подстроке.
Неподходящая сборка отклоняется с ошибкой hiddify-core. Выбор Hiddify возвращает
capabilities интеграции, включая Clash, rulesets и статистику, без обещания HotReload.

Новый конфиг проверяется через `check -c` до замены файла и остановки процесса.
Проверка без candidate config запрещена. AppliedConfig описывает запущенный
конфиг, а не отредактированный файл. Журнал пишется в `hiddify-core.log`.
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

Пользовательские счётчики связываются с metadata.User и stats.users. Существующий
сборщик трафика сохраняет неудачную DB batch для повтора; это ещё не доказательство
crash-safe учёта при потерянном ответе QueryStats — полный этап 6 остаётся впереди.
Clash используется для онлайн-пользователей, IP и отключения нужной сессии.

## Границы этапа

Автоматическая миграция БД, смена ядра по умолчанию и удаление Xray/sing-box
не включены: это последующие этапы. Config translation, nodes, subscriptions,
Gateway и sidecar bridges продолжают использовать имеющиеся адаптеры панели.
Ограничения ML-DSA/seed, mKCP, Xray session/fallback/sockopt/TLS, peer metering и
полный паритет дополнительных протоколов сохраняются до проверки каждого переноса.

Проверки второго этапа и их фактические результаты записаны в
[hiddify-core-stage-2-verification.md](hiddify-core-stage-2-verification.md).
