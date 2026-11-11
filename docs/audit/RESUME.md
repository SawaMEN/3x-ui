# Продолжение полного аудита f3baf84

Исходный коммит не менять: `f3baf84ab124c017812924481ccc80d6ce0be306`.
Рабочая ветка: `main`. Первая группа A01–A10 опубликована в `820e391`.
Вторая группа A11–A14 опубликована в `04f2179`.
Группа A15–A20 исправлена и проверена; коммит с этим состоянием — законченная контрольная точка продолжения.
Группа A21–A27 исправлена и проверена; этот коммит сохраняет очередную контрольную точку.
Группа A28–A29 также исправлена и проверена; A21–A27 опубликована в `f96b27f`.
Группа A30–A38 подготовлена как очередная контрольная точка: подписки, Hosts, HWID, кэш и CI.
Группа A30–A38 опубликована в `4ecc2e7`; A39 поднимает сборочный Go панели до 1.27.4.
Всего реализовано 49 подтверждённых исправлений панели; A47–A49 — текущая группа из 3 исправлений.

Полностью завершён 1 из 4 общих этапов; осталось 3.
Реестр учитывает **все 2364 файла**, включая удаления и переименования.
Проверено вручную 137 файлов, осталось 2227; общий проход не завершён.
Число ошибок, которые ещё предстоит найти, неизвестно и не заменяется числом файлов.

## Бюджет одного большого этапа: 300 минут

| Работа | Минуты |
|---|---:|
| Сверка источника и сохранение контрольной точки | 20 |
| База данных, миграции, статистика | 45 |
| Ядро и протоколы, ресурсы и завершение | 70 |
| Подписки и лимиты | 45 |
| API, узлы, фоновые задания и интерфейс | 70 |
| Установка, CI и итоговые проверки | 50 |
| Всего | 300 |

Это бюджет приоритетов, а не заявление, что весь исходный код уже просмотрен
или что один ответ гарантированно может работать пять часов.
Не закрывать аудит по времени: закрывать только по доказанному охвату и проверкам.

## Как восстановить состояние после любого запроса

1. Получить `main` и прочитать этот файл, основной отчёт и `coverage.md`.
2. Выполнить `python tools/audit/commit_inventory.py check`.
3. В `f3baf84-files.json` выбрать `pending` или `stale` в очередной подсистеме.
4. Читать полные файлы и связанные вызовы, воспроизводить дефекты до исправления.
5. После проверки отмечать конкретные пути:
   `python tools/audit/commit_inventory.py mark PATH --evidence 'что проверено и чем подтверждено'`.
6. Обновить `coverage.md` командой `python tools/audit/commit_inventory.py report`.
7. Сохранить код, тесты, отчёт, реестр и этот файл одним законченным коммитом в `main`.

Изменение уже отмеченного файла делает его проверку устаревшей (`stale`).
Ни удаление, ни генерация файла сами по себе не считаются ручной проверкой.
Новый код исправлений проверяется отдельно; исходный список файлов остаётся фиксированным.

## Следующая очередь

- database: исходники и тесты прочитаны полностью, 51 из 52 файлов. Осталась отдельная
  SQL-фикстура `internal/database/testdata/v3.9.8.sql`: её успешное восстановление и
  миграции не заменяют полный просмотр содержимого.
- `db.go`, `model/model.go`, `migrate_data.go` дочитаны; R01–R03 закрыты A21–A26.
- `internal/web/service/node_contract.go` прочитан полностью. У node.go исправлен
  контракт JSON (A27), но файл целиком ещё не проверен — оставить pending.
- Подписки: host_sub.go, host_order.go, external_config.go, external_subscription.go,
  settings_cache.go, links.go, telemt_subscription.go и связанные малые файлы/тесты
  прочитаны целиком. client_hwid.go и stage6_hwid_test.go тоже прочитаны.
  controller.go, service.go, host_sub_test.go читались фрагментами — оставить pending.
- Десять файлов классификатора limits прочитаны; реальный core round-trip пока пропущен
  без HIDDIFY_CONFIG_CHECK_BINARY. Далее проверить expiry/quota/device-limits в services.
- Продолжить остальные подсистемы по coverage.md, включая удалённые файлы и генераторы.

## Тесты и ограничения

A11–A14 воспроизведены локально до правок. После правок:
- `go test -race ./internal/clientlimit/... -count=1 -timeout 2m`: оба пакета прошли.
- `go test ./internal/database -short -count=1 -timeout 4m`: прошёл.

Для тестов используется Go из go.mod (сейчас `/tmp/3x-ui-go-patched/go/bin/go`, Go 1.27.4).
Вызовы синтетического speed broker и SQLite-тесты используют loopback / временные файлы.
Общий `go test ./...` пока не повторять: ранее автоматическая проверка отклонила
его из-за обращения к api4.ipify.org. Сначала определить тест и заменить реальный
внешний запрос локальной тестовой зависимостью либо явно выделить интеграционный тест.
Это не должно менять сетевое поведение продукта или обходить запрет проверки.
Firewall-тестам нужен отдельный network namespace; среда запрещает unshare.
PostgreSQL 16 проверен в отдельной временной локальной среде, см. ниже.
Реальные протоколы всё ещё требуют испытательной среды; пропуски не считать успехом.

## Контрольная точка A15–A20

- До правок воспроизведены потеря индекса, синтаксическая ошибка quoted-индекса,
  сломанная view, потеря NULL-дельты и вывод синтетических credentials в журнал.
- Полный database `-short -count=1`: прошёл после всех шести исправлений (22,444 с).
- Целевые migration / NULL / stream / privacy regression-тесты: прошли.
- database/model: прошёл; coremodel: компилируется, собственных тестов нет.
- TestClientSpeedLimit*: прошёл, live-core сценарий пропущен без бинарника.
- `go build -p 3 ./...`: прошёл для всего проекта.
- `go vet -p 3 ./...`: прошёл для всего проекта.
- Расширенный database `-race` для A15–A19: прошёл (135,129 с).
- A20: normal regression и отдельный -race прошли (3,153 с).
- Детектор устаревших review-mark проверен изменением уже отмеченного dump_sqlite.go:
  check отказался считать его проверенным; после повторного просмотра отметка обновлена.
- A15 уменьшает суммарные B/op примерно на 37,4% на описанной фикстуре, не пиковый RSS.
- При неизвестном расширении старой SQLite-схемы, которое нельзя восстановить поверх
  текущей модели, миграция откатывается целиком; тест проверяет сохранность исходных
  данных и индекса и восстановление PRAGMA после ошибки.

## Контрольная точка A21–A27

- R01 подтверждён на настоящем PostgreSQL 16 и исправлен A25; regression покрывает пустую
  таблицу, удалённый MAX(id), пустую таблицу после удаления, explicit import, is_called=false,
  зарезервированный будущий ID, повторный запуск и rollback внешней транзакции.
- R02 закрыт A21: зависший fail2ban-client не задерживает запуск больше бюджета probe.
- R03 закрыт A22–A24: параметры не теряются; старые нетронутые импорты ремонтируются один раз.
  Сбой UPDATE откатывает весь ремонт и не записывает успешную history-марку.
- A26: null Hysteria воспроизведён как panic до правки, regression после правки проходит.
- A27: 4 JSON payload подтверждённо публиковали неверное имя; контракт восстановлен,
  NodeSummary принимает старое имя на чтении, canonical поле имеет приоритет даже пустое.
- Все результаты и длительности прогонов — в основном отчёте A21–A27.
- Финальные `go build -p 3 ./...` и `go vet -p 3 ./...` прошли.
- PostgreSQL в текущей среде находится в `/tmp/3x-ui-pg16`; data — `/tmp/3x-ui-pg16-data`.
  Сервер остановлен после тестов. Это одноразовая тестовая БД, не сервер пользователя.
  Для повторных испытаний запускать сервер и тесты в одном exec-сеансе, с trap на остановку.
  Только loopback 127.0.0.1:38179; Unix socket среда запрещает. База xui_audit имеет UTF8.
  DSN без секретов: `host=127.0.0.1 port=38179 user=xui_audit dbname=xui_audit sslmode=disable`.
  Для Stage3/native backup нужен PATH с `/tmp/3x-ui-pg16/bin` и XUI_TEST_PG_DSN;
  XUI_DB_TYPE=postgres ставить только для явно отобранных PostgreSQL-тестов.

### Не закрытые вопросы для следующего полного просмотра

- Найти вызов публичного IP discovery в других test packages до общего Go-прогона.
  service TestMain уже подставляет offlinePublicIPTestClient, но он не действует на
  пакеты controller/runtime, импортирующие service. Не повторять запрещённый сетевой прогон.
- Проверить оставшиеся runtime/протоколы, подписки, services/jobs, API и UI по реестру.
- JSON quoting Listen проверен и исправлен A28; raw MTProto marker-like key исправлен A29.
- ConnectionAPIClient в singbox/connections.go прочитан. Текущих вызовов конструктора
  в production internal-коде не найдено; отдельно оценить удаление legacy daemon API
  и перенос общего ActiveSession типа, прежде чем менять этот код.

## Контрольная точка A28–A29

- Фиксация воспроизводимых проблем: invalid listen JSON и неожиданная ротация raw MTProto key.
- Полный model -race, database normal, targeted mtproto и singbox/hiddify -race прошли.
- Временное настоящее ядро `/tmp/3x-ui-audit-core/hiddify-core-linux-amd64` (рядом Cronet
  и target.json), релиз `sawamen-1f4926dd43b9-19f378cb93b5-mask1-ciphers1-net62-sec1-stream1`.
  Загрузка сделана штатным tools/fetch-latest-kernel.py с проверкой контрольных сумм,
  manifest и source identity; unit-тесты fetcher 10/10.
- С HIDDIFY_CONFIG_CHECK_BINARY прошли TestPinnedBinaryChecksGeneratedConfigurations,
  TestPinnedProtocolTransportMatrix, TestTunTranslationOwnerCore, TestStage5XDriveXrayValidator:
  4 теста + 75 subtests. Это config validation, не передача трафика.
- С HIDDIFY_RUNTIME_BINARY реальный TestPinnedBinaryRawRuntimeAndStatistics не смог
  запустить ядро: create netlink socket: operation not permitted. Не повторять без
  изменения возможностей среды и не пытаться обходить sandbox-ограничение.
- Документы и inventory сохраняются в том же коммите, что и код; следующие вопросы
  выбирать из оставшихся pending, а не объявлять весь аудит законченным.

## Контрольная точка A30–A38

- A30: некорректная VMess payload null больше не вызывает panic при переименовании.
- A31: запросы неизвестного subID не создают HWID-строки; отключённые владельцы
  сохраняют существующие строки, новые не регистрируются. Ранее созданные orphan
  rows автоматически не удаляются этим исправлением.
- A32: hosts-first не раскрывает исходный endpoint при полном исключении формата.
- A33: включение cap 0 -> N синхронизировано с регистрацией устройств на PostgreSQL;
  ошибка необязательного tracking изолирована savepoint и не отклоняет unlimited sub.
- A34: запросы после purge не присоединяются к singleflight предыдущего поколения.
- A35: URL внешней подписки не копируется в новые fetch errors и status-write warnings.
  Ранее сохранённые ошибки и журналы автоматически не стираются.
- A36: оба публичных Telegram-маршрута применяют CheckSubscriptionDevice до выдачи secret;
  внутренний авторизованный API узлов не менялся.
- A37: реальные CI logs выявили замечания errcheck/errorlint/goimports/usestdlibvars.
  Исправлены в коде/тестах, полный golangci-lint 2.15.0 возвращает 0 issues.
- A38: исправлен WebSocket mock, который не реализовывал disconnect; 2/2 теста
  проходят и проверяют cleanup. Проверка текущего файла oxfmt/oxlint прошла.
- TypeScript после всех правок прошёл (`tsc --noEmit`).
- Полный normal internal/sub после A36 прошёл (41,154 с), целевые service тесты
  прошли (1,123 с). До сброса среды PostgreSQL -race HWID прошёл (11,196 с),
  SQLite -race HWID прошёл (12,944 с); client_hwid.go после этих проверок не менялся.
- Финальные subscription/cache/Telegram/HWID tests с -race прошли (42,075 с),
  service HWID tests с -race прошли (10,076 с).
- Общие build/vet прошли до косметических CI-правок; после них полный pinned lint прошёл,
  database/domainfront normal прошли (9,713 с / 0,063 с).
- Локальная проверка WebSocket использует временный component-only config с теми же
  React/jsdom/setupFiles: Storybook multi-project import здесь запрещён uv_interface_addresses.
  Не объявлять весь frontend-прогон успешным; проверить новый GitHub CI после публикации.
- Временная среда сбросилась между сообщениями: /tmp с прежними журналами,
  PostgreSQL и настоящим ядром утрачен. Workspace и изменения git сохранились.
  Не считать прерванные поздние прогоны успешными. Go 1.27.3 восстановлен в /tmp/go.
- Следующий участок: оставшиеся подписки и серверная проверка expiry/quota/HWID,
  затем services/jobs и API. Пользовательские настройки, диагностический текст и
  восстановление базы проверять через все реальные входы, а не только helper-тесты.

## Контрольная точка A39

- В CI `4ecc2e7` frontend прошёл целиком: 2535 passed, 1 skipped, 0 failed.
  Также прошли go-test, golangci, race, PostgreSQL и настоящие core/speed TCP/UDP integration.
- govulncheck того CI обнаружил GO-2026-6859 в Go 1.27.3. Подтверждён security release
  Go 1.27.4; обновлён go.mod, все Go setup steps читают его через go-version-file.
- На Go 1.27.4 build и vet прошли. Govulncheck -show=verbose: 0 reachable vulnerabilities,
  0 в импортируемых пакетах; отдельно сообщает неимпортируемый openpgp GO-2026-5932.
- На Go 1.27.4 полный internal/sub прошёл (39,528 с), domainfront прошёл (0,069 с).
- R04: внешний hiddify-core ещё не пересобран этой правкой. Его main go.mod,
  release workflow и frozen source-lock по-прежнему выбирают 1.27.3. Нужны проверка
  реального артефакта и отдельное обновление его сборки/релиза; не считать A39
  исправлением всех внешних бинарников. Другой репозиторий этим этапом не изменялся.

Полный golangci-lint 2.15.0 на Go 1.27.4 также прошёл: 0 issues.

## Контрольная точка A40–A43

- A39 опубликована в `4d04a9d`. GitHub CI подтвердил успешные govulncheck,
  go-vet и golangci на обновлённом Go; полный запуск на момент проверки ещё шёл.
- A40: до первой traffic snapshot configured quota/expiry терялись в userinfo.
  Воспроизведено на HTTP /sub/, /json/, /clash/: total=0; expire=0 при заданных
  total=10000; expire=1893456000. Теперь отсутствующий runtime row представлен
  только в памяти; чтение подписки не создаёт строки статистики в базе.
- A41: cached aggregation дважды учитывала повторные email, SQL IN — один раз.
  Входной список копируется и дедуплицируется; оригинальный slice не меняется.
  Тест сравнивает обе ветви при смеси существующих и отсутствующих traffic rows.
- A42: суммы directional counters/quota переполнялись в отрицательные значения.
  Воспроизведено up=-4, down=-6, total=-2; применяется общий saturating helper.
  Delayed expiry также насыщается при превышении int64. Нулевой total по-прежнему unlimited.
- A43: getInboundsBySubId и его loader оба вызывали indexStatsBySubId.
  Воспроизведено 2 чтения client_traffics вместо 1. Оставлен общий preload после
  загрузки клиентов; тест сохраняет проверку ссылок и точных directional counters.
- Для limits query выбраны только email/total_gb/expiry_time/reset_day,
  без загрузки UUID/password и остальных ненужных полей ClientRecord.
- До A43 полный internal/sub прошёл (43,775 с), golangci-lint: 0 issues.
  Финальные проверки после A43 записать ниже по фактическому завершению.
- Полностью прочитаны ещё 9 исходных файлов: calendar tests, forwarded trust
  и тесты, keepalive, output format и тесты, page data tests, orphan stats tests,
  userinfo tests. endpoint.go/endpoint_test.go тоже прочитаны, но они не входят
  в исходный diff и не увеличивают его охват. service.go остаётся pending.
- Охват: 127/2364, осталось 2237. Подтверждённых исправлений: 43; для группы
  A40–A43 нужно 4, реализовано 4, осталось 0 реализации.

Финальные проверки A40–A43 на Go 1.27.4:
- весь internal/sub: PASS (39,486 с);
- целевые userinfo/query-budget/expiry regression tests с -race: PASS (8,739 с);
- полный golangci-lint 2.15.0: 0 issues;
- go build -p 3 ./...: PASS; diff --check: PASS.

CI предыдущей опубликованной точки `4d04a9d`: govulncheck, frontend, оба
race-sub shard, общий race, native-core-integration и native-client-speed прошли.
Go-test и race-service ещё выполнялись при последней сверке; не объявлять
весь workflow завершённым до получения итогового статуса.

## Контрольная точка A44–A46

- A40–A43 опубликованы в `116f5d7`. CI run 42966839169: все задания завершены
  успешно (включая frontend, govulncheck, PostgreSQL, race shards, native core/speed).
- A44: список links менял порядок/размер при hosts-first, но emails оставался
  в порядке inbounds. Воспроизведены alice вместо bob в первой строке, bob
  вместо alice во второй и пустой email в третьей. MASQUE без raw URI также
  сдвигал подпись следующей VLESS-ссылки. Рендер теперь хранит link и email
  вместе; metadata клиентов без raw URI добавляются после связанных пар.
  Скрытые Telegram-клиенты сохраняют identity/online metadata; общая статистика
  по-прежнему собирается отдельно через seenEmails.
- A45: statsForClient возвращал нулевые limits из runtime row, хотя userinfo
  уже подставлял настроенные лимиты. Raw remark был remark|0|0 при header limits
  1024 байт / 1893456000000 мс. Все три пути (inbound/cache/database) теперь
  используют fallback только при нуле; реальные usage/status и ненулевые
  runtime limits сохраняются. Ни runtime row, ни БД не изменяются рендерингом.
- A46: Jalali конвертер перебирал каждый месяц от 2024 года; максимальная
  int64-millisecond expiry требовала миллиардов шагов. Полные 33-летние циклы
  существующего календаря пропускаются арифметически, JDN intermediates int64.
  Для положительных future expiries остаётся меньше 33 лет помесячных шагов.
  Это оптимизация существующего 33-year календаря, не замена его другим алгоритмом.
- Сняты исходные outputs для 1970, leap boundary 2024/2025, 2500 и 9999;
  после оптимизации совпадают. Maximum int64 expiry regression проходит.
  Benchmark (20 итераций, тот же процессор, Go 1.27.4): 162619 -> 560 ns/op
  для 9999-12-31, около 290x. Это локальная микропроверка, не общий throughput.
- Полностью дочитаны remark_vars.go и placeholders.go; они отмечены в inventory.
  Также прочитаны legacy_request.go, service_sort_test.go и service_dedup_test.go,
  но они отсутствуют в исходном diff и не увеличивают его охват.
- Ручной охват 130/2364, осталось 2234. Для группы нужно 3 исправления,
  реализовано 3, осталось 0 реализации; общий аудит остаётся незавершённым.
- До A46 весь internal/sub прошёл (39,626 с), targeted -race прошёл (5,427 с).
  Замечание staticcheck в новом тесте исправлено (tagged switch).
  Финальные проверки A44–A46 записать ниже после завершения.

Финальные проверки A44–A46: весь internal/sub — PASS (40,210 с), расширенные
Audit/host-order/calendar regression tests с -race — PASS (25,222 с), весь проект
собран go build -p 3 ./...; diff --check прошёл. Первый финальный golangci был
прерван переполнением локального Go build cache (no space left on device),
не считается успешным. Кэш очищен; повторный lint выполняется отдельно
с GOCACHE=/tmp/3x-ui-go1274-cache. Результат записать после завершения.

Полностью прочитан также remark_vars_test.go (765 строк), отмечен в реестре.

Повторный полный golangci-lint 2.15.0 завершился успешно: **0 issues**.
Для дальнейших проверок использовать GOCACHE=/tmp/3x-ui-go1274-cache: старый
/home/agent/.cache/go-build очищен после переполнения, новый кэш уже прогрет.

## Контрольная точка A47–A49

- A44–A46 опубликованы в `60a9ec1`. CI run 42968629282, Release и CodeQL прошли.
  Отдельный Pinned Hiddify core release integrity audit run 43012089228 упал:
  job 130493967750, missing mandatory panel features.
- A47: проверен настоящий старый закреплённый архив 4f1b5e6b9b2b/4fe303071f4f.
  Его SHA256 корректен, но packaged tags не содержат http2legacy, уже обязательный
  в панели. Активный build lock и TargetVersion/TargetRevision обновлены на
  официальный stable 1f4926dd43b9/19f378cb93b5. Historical fork-contract не менялся.
  Проверки версий/тегов не ослаблены.
- Последний stable скачан для AMD64 и ARM64; оба архива, оба внутренних checksum,
  release/source pins, manifest capabilities и ELF-архитектура проверены.
  AMD64 executable version проверен запуском. ARM64 на AMD64 не исполнялся.
  SHA256 AMD64 archive: 1a21f905a472365cded299b99c18012a9dfa50690ae0aaaade70edb93a5693f0.
  SHA256 ARM64 archive: 6140943d3d5d074855370aa787c14e3e096ff1a5fee06dd22aac4cf4de8eb247.
- Аудит закреплённого релиза теперь запускается по связанным push и проверяет
  матрицу AMD64/ARM64. Новый обычный CI также проверяет весь internal/hiddify
  с HIDDIFY_CONFIG_CHECK_BINARY, а не только прежний набор config-тестов.
- A48: изготовленный checksum-valid пакет с правильным manifest и бинарником,
  чья version строка не содержит http2legacy/поддерживаемой linkage, принимался
  и заменял прежний kernel. Воспроизведено unit-тестом до исправления.
  Теперь tags проверяются и в executable version до публикации файлов;
  несовместимость не меняет прежний binary. Ошибка manifest перечисляет missing tags.
  Добавлен контракт равенства обязательных Go/Python tags.
- A49: полный native-core config прогон выявил, что mKCP выдавал устаревшие
  transport.seed/header/legacy/final_mask. Core schema принимает там только
  numeric options. Legacy outbound теперь использует top-level final_mask,
  seed/checksum идёт перед header, wechat-video -> wechat. Explicit UDP chains
  сохраняются; конфликты и повреждённые типы отвергаются, input не меняется.
- R05: реальное ядро отдельно отвергает inbound UDP FinalMask: поддержаны только
  TCP listener masks. Входящий legacy mKCP НЕ объявлять заработавшим: панель
  теперь возвращает понятную ошибку до native serialization; для поддержки нужен
  отдельный этап внешнего ядра. Native mKCP без legacy масок проходит проверку конфигурации.
- R04 остаётся открыт: version скачанного stable подтверждает Go 1.27.3.
  Обновление исходного compiler/runtime внешнего ядра и новый security release
  не выполнены этим этапом. Не выдавать A47 за исправление его стандартной библиотеки.
- Python fetcher: 13/13 tests PASS; tools/ci: 39/39 PASS; YAML обоих workflows
  parsed; static build/fork verifier PASS без optional checkout внешних sources.
- Полный internal/hiddify с настоящим CONFIG_CHECK binary PASS (3,026 с).
  Дополнительная KCP matrix (20 cases) PASS (0,392 с). Это config validation,
  не реальный трафик: R05 и прежний netlink sandbox не обходились.
- go build -p 3 ./... PASS. Первый lint нашёл unused assignment в новом test branch;
  исправлен, финальный lint записать после завершения.
- Среда снова сбросилась: восстановлены Go 1.27.4, modules и golangci 2.15.0.
  GOCACHE=/tmp/3x-ui-go1274-cache; core — /tmp/3x-ui-latest-core-audit/verified.
- Полностью проверено ещё 7 исходных файлов, 2 review marks обновлены после правок.
  Ручной охват: 137/2364, осталось 2227. ci.yml прочитан только в затронутом
  участке и остаётся pending. Подтверждённых исправлений панели: 49.

Финальный golangci-lint 2.15.0: **0 issues**. Аудит release matrix использует
native runners ubuntu-24.04 и ubuntu-24.04-arm, чтобы в CI проверять executable
version и tags на обеих архитектурах. Локальный ARM64 запуск не засчитывается.
Схема mKCP сверена с owner source 19f378cb93b5:
- https://github.com/SawaMEN/hiddify-sing-box/blob/19f378cb93b58a5ac0d0b70183e1b0e92d9553c9/option/h_v2ray_kcp.go
- https://github.com/SawaMEN/hiddify-sing-box/blob/19f378cb93b58a5ac0d0b70183e1b0e92d9553c9/hiddify/finalmask/xray_conf.go
