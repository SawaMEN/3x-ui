# Этап 3: общие модели и совместимость БД

Работа выполнена в `update`, поверх этапа 2. Зафиксированные исходники ядра
остаются `SawaMEN/hiddify-core` и `SawaMEN/hiddify-sing-box` из build lock.
Изменение самого ядра для этого этапа не потребовалось.

## Результат

`internal/coremodel` владеет `Traffic`, `ClientTraffic` и общей структурой
`InboundConfig`. Пакет не импортирует Xray, sing-box, Hiddify или модели БД.
Трафик API, подписок, лимитов, продления, фоновых заданий, синхронизации узлов
и отдельных протокольных сервисов использует общие типы напрямую.
В коде вне `internal/xray` нет обращений к `xray.Traffic`/`xray.ClientTraffic`.

`ClientTraffic.TableName()` явно закрепляет `client_traffics`. JSON/form/GORM
поля, индексы, уникальность email и неперсистентные UUID/SubId/TUIC metadata
сохранены. `Inbound.ClientStats`, штатная и межбазовая миграции используют
новый тип. Канонические `Client`, `ClientRecord`, `ClientInbound` уже находились
в `internal/database/model`; их поля и таблицы не переносились и не менялись.
Production-зависимости `internal/database`, `internal/database/model` и
`internal/coremodel` больше не включают пакет Xray и библиотеку xray-core.
Тесты исторических миграций outbound всё ещё проверяют Xray-конфиг его валидатором.

`Inbound.GenInboundConfig()` возвращает общий DTO в историческом формате панели.
Существующие нормализации настроек сохранены. `xray.InboundFromPanel()` адаптирует
DTO к Xray wire-типу, где остаётся VLESS-сериализация: client-side encryption
не отправляется серверу, пустые fallbacks при encryption исключаются.
Полный `xray.Config` с API/policy и protobuf-команды остаются частью адаптера,
а не объявляются универсальным native-конфигом Hiddify. Генератор native-конфигов
относится к следующим этапам.

`CommitCoreTraffic()` сохраняет общий набор счётчиков одной транзакцией.
Прежние aliases и имена `GenXrayInboundConfig`/`CommitXrayTraffic` оставлены
как deprecated compatibility entry points. Сам Xray-адаптер ещё не удалён;
получение и подтверждение счётчиков остаются ответственностью адаптера.

## Проверки

- `go test ./internal/coremodel ./internal/database/... ./internal/xray ./internal/singbox ./internal/hiddify` — PASS.
- `go test -race ./internal/database/... ./internal/coremodel ./internal/xray` — PASS.
- Полные тесты `internal/web/service/...`, `internal/web/runtime/...`,
  `internal/web/controller/...`, `internal/web/job/...`, `internal/naiveproxy`,
  `internal/sub/...`, `internal/tuic/...` — PASS.
- `go test ./internal/web -count=1 -timeout=60s` — PASS после исправления
  пропуска `/panel/api/setting/hiddify/status` в реестре API предыдущего этапа.
  Обе OpenAPI-спецификации и ссылка API reference дополнены этим маршрутом.
- Race-тесты Hiddify, rollback/commit baseline Xray, общего учёта, сброса лимитов
  и межузловых счётчиков — PASS. `HIDDIFY_CONFIG_CHECK_BINARY` указывает на
  собранное из собственного форка ядро, поэтому Hiddify database-config также
  проходит настоящий `check -c`.
- `go vet` общих моделей, БД, service/runtime/controller/job, подписок и Xray — PASS.
- `go build -o .../x-ui-stage3 .` — PASS.
- Проверка зависимостей через `go list -deps` и `git diff --check` — PASS.
- Генерация OpenAPI и сравнение Hiddify status contract в обеих спецификациях — PASS.
  Frontend Vitest suite не запускался: локально отсутствует зависимость `vitest`.

Новый regression-тест начинает с независимого SQL snapshot прежней
`client_traffics`, затем выполняет штатный `InitDB` и два повторных запуска.
Он проверяет все счётчики и лимиты, ID `14825`, inbound ID `37`, client ID `91`,
UUID/subId, preload статистики, attachment, индексы, уникальность email,
продолжение autoincrement и отсутствие новых таблиц вместо старой.
Отдельный тест проверяет разделение panel JSON и Xray wire JSON без изменения DTO.

Полный запуск ядра и сетевой трафик по-прежнему не подтверждены в этой среде:
netlink socket запрещён, как зафиксировано в отчёте этапа 2. Этот этап
проверяет слой данных и его совместимость, не crash-safe учёт этапа 6.
