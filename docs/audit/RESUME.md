# Продолжение полного аудита f3baf84

Исходный коммит не менять: `f3baf84ab124c017812924481ccc80d6ce0be306`.
Рабочая ветка: `main`. Первая группа A01–A10 опубликована в `820e391`.
Вторая группа A11–A14 опубликована в `04f2179`.
Группа A15–A20 исправлена и проверена; коммит с этим состоянием — законченная контрольная точка продолжения.
Группа A21–A27 исправлена и проверена; этот коммит сохраняет очередную контрольную точку.
Группа A28–A29 также исправлена и проверена; A21–A27 опубликована в `f96b27f`.
Группа A30–A38 подготовлена как очередная контрольная точка: подписки, Hosts, HWID, кэш и CI.
Группа A30–A38 опубликована в `4ecc2e7`; A39 поднимает сборочный Go панели до 1.27.4.
Всего реализовано 91 подтверждённое исправление панели; A91 — периодический учёт по pinned runtime owner через WAL до advance counters.

Полностью завершён 1 из 4 общих этапов; осталось 3.
Реестр учитывает **все 2364 файла**, включая удаления и переименования.
Проверено вручную 260 файлов, осталось 2104; общий проход не завершён.
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

## Контрольная точка A50–A52

- A47–A49 опубликованы в `ada035e`. Все задания CI 43043068255, кроме
  telemt-web-frontends, прошли. Исходный job 130599790708 упал на apt DNS
  (Ubuntu resolute / ubuntu-latest), до Go frontend tests. Повторён только
  этот job; повтор 130608607846 также упал на DNS apt до тестов. Не считать
  ошибкой приложения или успешным frontend-прогоном.
- Для воспроизводимости этого CI-задания закреплены ubuntu-24.04 и timeout 20m.
  Это изменение тестовой среды, не отдельный исправленный дефект приложения;
  результат следующего CI проверить после публикации. Тесты не отключались.
- A50: genTemplatedRemark всегда запрашивал stats и декодировал TLS settings,
  даже когда display/repeated-link template их не использовал. На 8 Hosts без
  traffic row воспроизведено 10 запросов вместо preload+aggregate (2).
  Теперь контекст вычисляется только для реально оставшихся traffic/SECURITY
  tokens; имя и все 8 ссылок сохранены. Полный пакет покрывает используемые
  statistics/security tokens и first-link-only поведение.
- BenchmarkAuditNameOnlyRemark (100 повторов, Go 1.27.4, один AMD EPYC 9V45):
  17706 -> 1931 ns/op; 20090 -> 1238 B/op; 30 -> 28 allocs/op.
  Около 9x на этой фикстуре, примерно на 94% меньше allocation bytes;
  это микрозамер helper с 8KiB stream settings, не пиковый RSS/скорость панели.
- A51: ошибка SQL при чтении persisted routing cache считалась окончательной
  загрузкой. После краткого сбоя last-good cache не восстанавливался до restart.
  Loaded-флаг теперь ставится только после завершённых чтений; отсутствующая
  строка и отвергнутый invalid payload не равны transient DB error. Тест
  вводит ошибку первого SELECT и проверяет успешную повторную загрузку.
- A52: persisted Clash cache обходил 2MiB network cap и попадал в YAML parser.
  Теперь content до нормализации проверяется по cap своего формата; тесты
  покрывают ровно 2MiB (принят) и +1 byte (отвергнут). Старые oversized rows
  не удаляются из БД, но не загружаются в routing memory cache.
- До исправлений все три новые regression проверки падали с описанными
  симптомами. После исправлений целевые tests/benchmark PASS (0,387 с).
- Полный internal/sub PASS (28,993 с), целевые routing/template tests с -race
  PASS (8,008 с), полный golangci-lint 2.15.0: 0 issues. Финальные TUIC aggregation/shutdown journal
race tests PASS 3.748s.
- Для race использован GOCACHE=/home/agent/.cache/3x-ui-audit-race, для normal
  GOCACHE=/tmp/3x-ui-go1274-cache: это отдельные временные кэши, чтобы не заполнить /tmp.
- Полностью прочитано ещё 9 исходных файлов: remote routing и его tests,
  Clash external export + SS/quota tests, YAML serialization + Sudoku test,
  INCY detector, metadata placeholder tests. remark_vars.go повторно просмотрен
  после изменения и review hash обновлён. Старый неверный комментарий SS test исправлен.
- Охват 146/2364, осталось 2218; подтверждённых исправлений 52. В группе
  A50–A52 нужно 3, реализовано 3, осталось 0 реализации. Общий аудит продолжается.
- R04/R05 внешнего ядра остаются открытыми; ci.yml, controller.go, service.go
  по-прежнему не отмечать reviewed: у них просмотрены только отдельные части.

Финально: сборка всего проекта go build -p 3 ./... прошла; 39 tests tools/ci
прошли после закрепления runner, YAML ci.yml разобран. Рабочие исходники прошли
golangci-lint (0 issues), diff --check и проверку актуальности review hashes.

Следующий приоритет: api-gateway — middleware, sessions, разграничение доступа
и публичные маршруты; затем фоновые jobs/nodes. Проверка подписок не закрыта:
там остаётся 67 pending файлов, controller.go/service.go ещё требуют полного просмотра.


## Контрольная точка A53–A57: API и браузерная авторизация

Предыдущая группа A50–A52 опубликована в ca012f735364a9db8a0542a23e328fef63fcca11.
Новая группа: нужно 5 исправлений, реализовано 5, осталось 0 реализации.
Общий аудит: 57 подтверждённых исправлений; полный просмотр 156/2364, осталось 2208.
Число ещё не найденных ошибок неизвестно.

- A53: Authorization scheme теперь сравнивается без учёта регистра, разделитель
  допускает несколько SP; сам credential сохраняет регистр. Неизвестный/пустой
  Bearer возвращает 401, анонимный запрос по-прежнему 404. RFC 9110 §11.1.
- A54: valid bearer / verified mTLS не выставляет api_authed и не вызывает handler,
  если GetFirstUser не вернул principal (SQL error / no user): 503. Все scope
  allowlists, mTLS node-sync default и explicit node-admin сохранены.
- A55: Create/RecreateByName ограничивают имя 64 Unicode code points, а не байтами.
  Проверены ASCII, кириллица, emoji, границы 64/65; ротация сохраняет scope,
  отзывает старый credential и выдаёт работающий новый.
- A56: login при SQL failure регистрации сессии возвращает 503 + success=false
  вместо пустого 200. Успешные log/event и сброс limiter происходят после Save.
  Приватный текст SQL error остаётся только в серверном журнале.
- A57: /csrf-token всегда устанавливает Cache-Control: no-store, включая повтор
  с существующей cookie и ветку ошибки. Прочие публичные маршруты ещё в очереди.

Все 5 дефектов воспроизведены новыми тестами до правок. После исправлений:
- scoped offline controller auth/login/CSRF + scope matrix: PASS, 1.174s;
- повтор этих сценариев -race: PASS, 14.724s;
- весь service/panel: PASS, 1.469s; session: PASS, 0.217s;
  middleware: PASS, 0.151s;
- API token/auth предыдущая группа -race: controller 11.869s, panel 5.645s;
- go build -p 3 ./...: PASS; полный golangci-lint 2.15.0: 0 issues;
- git diff --check, inventory check: PASS, 0 stale.

Тесты controller намеренно ограничены полностью просмотренными sentinel handlers:
они используют временную SQLite и loopback, не GetStatus и не внешние IP services.
Запрет на повтор полного go test ./... до устранения внешней зависимости остаётся.

CI ca012f7 run 43047897501: Go tests, все race shards, vet, lint, frontend,
PostgreSQL, native core/client-speed и official integrations прошли. Исключение:
telemt-web-frontends job 130616792770 снова упал до тестов на apt DNS
azure.archive.ubuntu.com уже на Ubuntu 24.04. Закрепление runner не решило DNS.
Release ada035e оба build job тоже упали на apt DNS (amd64 130600990247),
это не дефект компиляции. Не объявлять CI/release зелёным и не засчитывать
пропущенные Telemt frontend tests. R04/R05 ядра остаются открытыми.

Далее: оставшиеся api-gateway 101 файла, особенно WS, публичные HTTP gateways,
мультиузловая авторизация, затем services/jobs. Сохранить исходную точку f3baf84.


## Контрольная точка A58–A59: WebSocket shutdown и HTML страницы

A53–A57 опубликованы в e01e5bc952d2aa7196e8cf9993a98f805a313ee6.
Новая группа: нужно 2 подтверждённых исправления, реализовано 2, осталось 0.
Дополнительно завершён A57 для второго маршрута /panel/csrf-token: это та же
проблема, её не считать ещё одним новым исправлением. Всего 59.

- A58: shutdown закрывал только h.clients, но не регистрации в h.ops; при
  отмене Register просто возвращал управление с незакрытым Send. HandleConnection
  всё равно запускал write/read pumps, поэтому поздний или ещё не обработанный
  клиент мог продолжать жить после остановки hub. Теперь shutdown забирает и
  закрывает pending registrations, а Register закрывает канал при отмене даже
  если enqueue совпал с завершением drain. Закрытие защищено тем же h.mu, что
  и fanout, с closeOnce. Старую FIFO-семантику Register/Unregister не менять.
- A59: Normalize допускает кавычки и HTML delimiters в URI prefix, но dist.go
  вставлял prefix в asset attributes без HTML escaping, а ручной jsEscape
  оставлял </script> нетронутым. DOM терял asset URLs и получал лишние script
  элементы. Теперь asset prefix HTML-escaped, значения inline JS encoded через
  json.Marshal (включая версию и DB kind). Новый тест проверяет реальный HTML
  parse tree: ровно три script nodes, оба asset URLs и decoded base path.
  Не заявлять удалённый обход CSP: доказан дефект обработки настроенного URI.

До правок: 3 shutdown regression tests падали с незакрытыми каналами; HTML test
показывал 4 scripts вместо 3, 0 intact assets вместо 2 и оборванную JS string.
После правок:
- полный websocket package: PASS, 0.596s;
- полный panel: PASS, 1.555s; session 0.223s; middleware 0.106s;
  util/uripath 0.002s;
- scoped controller auth/login/CSRF/DOM/SPA/origin: PASS, 0.510s;
- полный websocket + panel -race до добавления final DOM check: 1.686s / 18.399s;
- итоговый scoped -race: controller 15.684s, panel 7.227s, websocket 1.045s;
- реальный loopback WebSocket при остановленном hub немедленно получает close
  frame (вместо Read timeout), PASS 0.019s;
- go build -p 3 ./...: PASS; golangci-lint: 0 issues;
- inventory check и git diff --check: PASS, 0 stale.

Полностью дочитаны ещё 20 исходных файлов: hub/tests/pumps/origin, dist/SPA,
recovery/root redirect/deadlines и uripath. Охват 176/2364, осталось 2188.
Вне original diff также прочитаны notifier.go и spa_test.go — в счётчик не вошли.

CI e01e5bc run 43050203515: CodeQL, lint, vet, frontend, race/sub, PostgreSQL,
оба native integrations и official service integrations уже прошли; go-test и
service race ещё выполнялись при проверке. telemt-web-frontends 130625013622
снова остановлен apt DNS azure.archive.ubuntu.com до тестов. Это ограничение
не закрыто сменой runner и не считается успешным тестом. Внешний core R04/R05
и локальный запрет полного go test ./... по-прежнему открыты.

Следующий приоритет: api-gateway 85 pending, nodes/background services;
subscriptions 67 pending остаются отдельной очередью, не объявлять закрытыми.
Рабочая ветка main, исходный diff f3baf84 неизменен.


## Контрольная точка A60–A63: RPC и согласование узлов

Предыдущая группа A58–A59 опубликована в dd6e13848271d951e55bf5cda2f4608a24d67253.
Новая группа: требуется 4 подтверждённых исправления, реализовано 4, осталось 0.
Всего 63; просмотрено 197/2364 исходных файлов, осталось 2167.

- A60: resolveRemoteID отличает подтверждённое отсутствие inbound от ошибки
  списка (HTTP, success=false, некорректный JSON). Update не делает Add при
  ошибке чтения, Delete её больше не скрывает. Пустой корректный список сохраняет
  upsert и идемпотентность удаления. Добавлен errors.Is sentinel not-found.
- A61: RestartXray выбирал legacy fallback по словам HTTP 404/405 в Error(),
  включая ответ 500 или success=false, провоцируя второй restart. Добавлен
  remoteHTTPError с реальным status и IsRemoteHTTPStatus с errors.As. Только
  настоящие 404/405 вызывают fallback. Два таких же текстовых сравнения в
  traffic job переведены на этот predicate, чтобы не скрывать реальные failures.
  Ограничение snippet 8KiB и escaping недоверенного диагностического текста сохранены.
- A62: удаление/переименование инвалидировало только точный central tag, оставляя
  bare/prefixed/adopted mappings и fingerprint. Теперь все aliases одного ID
  очищаются под Remote.mu; новые/посторонние mappings сохраняются. Подтверждённое
  отсутствие при повторном Delete тоже удаляет fingerprint/adopted association,
  не задевая другие отсутствующие теги (id=0 не является wildcard).
- A63: Remote запоминал resolver, установленный при создании, поэтому не видел
  позднюю установку/замену/отключение egress. Managed Remote получает Manager как
  динамический resolver; callback выполняется без manager lock. Проверены смена
  URL, отключение (refuse direct), восстановление и reentrant callback.

Новые tests воспроизвели все 4 дефекта до правок; дополнительная ветка absent
Delete также воспроизведена до расширения A62. Два старых reconcile fixtures
возвращали success=true без obj для /inbounds/list и полагались на дефект A60:
теперь они возвращают корректный пустой список. Тесты реального lookup failure
сохранены отдельно и требуют error без мутации.

Проверки:
- полный internal/web/runtime после A60–A63: PASS, 2.355s;
- финальный полный runtime -race после всех правок и reentrant test: PASS, 8.143s;
- scoped job heartbeat/traffic/atomic/adoption: PASS, 13.083s;
- сборка всего проекта go build -p 3 ./...: PASS;
- финальный полный golangci-lint 2.15.0: PASS, 0 issues;
- diff --check и inventory check: PASS, 0 stale.

Среда /tmp и /home/agent сбросилась между запросами; восстановлены Go 1.27.4
из official dl.google.com с проверкой SHA-256 и golangci-lint 2.15.0.
Обычный GOCACHE /tmp/3x-ui-go1274-cache, race /home/agent/.cache/3x-ui-audit-race.
Полный go test ./... не повторяли; offline runtime не импортирует service,
выбранные job tests используют только временную DB и loopback nodes.

CI dd6e138 run 43051558265: все jobs, кроме telemt-web-frontends (130630042201),
прошли; CodeQL success, release failure. Предыдущий DNS blocker пока не закрыт,
лог этого job прочитан: подтверждён тот же apt DNS azure.archive.ubuntu.com до тестов.
R04/R05 внешнего core остаются открытыми.

Следующий участок: SNI/certificate discovery nodes. Есть два подозрительных
контракта для воспроизведения: discoverNodeSNI убирает file paths, а merge
пока требует их; node-sync allowlist не включает discovery domains routes.
Не расширять mutation scope: domain writes требуют interactive session.
В API остаётся 82 pending, services/nodes/jobs 640, subscriptions 67.


## Контрольная точка A64–A66: межузловое обнаружение и выбор SNI

A60–A63 опубликованы в 563a1bd417dc58fffa9d24b7df4aca301f6f5062.
Группа A64–A66: требуется 3 новых исправления, реализовано 3, осталось 0.
Всего 66. Охват 211/2364 исходных файлов, осталось 2153.

- A64: discoverNodeSNI намеренно удалял certificate/key paths, но merge требовал
  оба пути и отбрасывал все remote summaries. Теперь merge принимает metadata
  без путей, нормализует hostname общим валидатором, отвергает IP/invalid labels,
  сохраняет (nodeID, hostname) и очищает пути, присланные устаревшим peer.
  До правки реальный loopback discovery выдавал sites=1, merged=0.
- A65: node-sync/node-admin/mTLS получали 403 на SNI/certificate discovery и
  HTTP-01 observation. Добавлены ровно GET /domains/sni, GET /domains/certificates,
  POST /domains/probe/http01. Для node scopes SNI/certificates всегда local-only
  и metadata-only независимо от query flags. Отдельный local certificate summary
  не обращается к firewall и не пересылает warnings, file paths или child nodes.
  Generic panel/subscription labels заменяются DNS SAN для совместимости с
  parent validation. Remote cert RPC явно просит summary=1. Monitor scope,
  /domains/status и domain mutations не расширены; записи требуют interactive
  admin session, включая запрос с полным admin bearer.
- A66: SniSearchInput использовал hostname и как Select.value, и как find key.
  При одинаковом SNI у нескольких владельцев клик по второму узлу возвращал
  первую локальную запись. Идентификатор option теперь включает source, nodeID,
  hostname; наружу по-прежнему передаётся чистый SNI и выбранная metadata record.
  Реальный AntD component test воспроизвёл неправильного владельца до правки.

Дополнен A62 без увеличения счётчика: full push adopted inbound меняет deployed
wire tag и при неизменном central tag. Такой successful update тоже сбрасывает
obsolete alias; новая регрессия сначала упала и после правки проверяет сохранение
canonical mapping и reconcile skip. Финальный полный runtime -race PASS 8.518s.

Проверки A64–A66:
- targeted service discovery/metadata and related summaries: PASS 0.448s;
- controller auth/scope + actual Remote-to-Gin RPC: PASS 2.791s;
- scoped controller/service -race: PASS 14.597s / 6.032s;
- финальный actual RPC controller -race: PASS 4.914s;
- полный runtime после summary query: PASS 1.775s;
- AntD ownership regression + managed-SNI ownership: 6/6 PASS;
- SNI protocol forms + ownership + new selection regression: 43/43 PASS,
  3 files, 50.43s. Использован временный component-only Vitest config с теми же
  React/jsdom/setupFiles; Storybook browser project локально не загружался.
  Это не полный frontend suite. Временный config удалён, тесты сохранены.
- frontend tsc --noEmit: PASS; oxfmt/oxlint для changed files: PASS;
- сборка всей панели: PASS;
- финальный полный Go lint после дополнения A62: PASS, 0 issues;
- diff --check/inventory check: PASS, 0 stale.

14 новых исходных файлов отмечены reviewed; 3 изменённых review hashes обновлены.
Большой frontend sni-search.test.tsx прочитан фрагментами и не засчитан.
DomainFrontService целиком прочитан, но domain_front.go оставлен pending:
R06 — проверить rollback при ошибке записи domainFrontConfig: oldBackend собирается
без Certificates/ChallengeRoots и ошибка обратного Apply сейчас игнорируется.
Нужен regression с локальным TLS existing-certificate и SQL failure; не считать
это исправленным до воспроизведения. Отдельно не расширять remote domain writes
ради Manager ProvisionCoverSite: они намеренно требуют interactive session.

CI 563a1bd run 43062443040: все 26 jobs success, включая telemt-web-frontends
130670344809, frontend, все race shards, PostgreSQL, native core/client-speed.
Release 3X-UI и CodeQL тоже success. DNS blocker прежних запусков больше
не воспроизводится на этой контрольной точке; новые изменения A64–A66 ещё
должны пройти свой CI после публикации.
R04/R05 внешнего ядра и запрет полного локального go test ./... сохраняются.


## Контрольная точка A67–A68: rollback и граница полномочий узла

A64–A66 опубликованы в 501c657776f2fd56bb5baaf92d8bb6e61fbb351f.
Пока шла работа, main получил независимый 0468d8650de2da8bab60cf2084a041ac09e4c8f6
(cover-site templates, local TLS, domain activity). Наши 4 незавершённых файла
сохранены stash, main fast-forward, stash восстановлен без конфликтов. Новые
возможности этого коммита не перезаписаны. Результаты старого запуска, загружавшего
packages во время смены дерева, не засчитывались: выполнены свежие проверки.
Среда /tmp и /home/agent снова сбросилась; Go 1.27.4 и golangci 2.15.0 восстановлены
с проверкой официальных archive SHA-256, normal/race cache разнесены.

- A67 закрывает R06. При SQL failure domainFrontConfig прежний existing-TLS
  gateway оставался остановленным: service строил неполный oldBackend, а
  обратный Apply с отменённым request context мог тоже отказать. Новые tests
  запустили реальный localhost TLS со своим сертификатом, затем запретили UPDATE
  настройки (в вариантах с отменой запроса и без). До правки оба потеряли listener.
  Manager.ApplyAndCommit сохраняет actual old cfg/backend до успешного commit;
  при ошибке закрывает новый runtime и восстанавливает полный старый backend
  отдельным context. Callback не должен reenter Manager. Ошибки commit и rollback
  обёрнуты %w, оба доступны через errors.Is. Service больше не собирает усечённый
  rollback и не скрывает ошибку обратного старта. Добавлены проверки освобождения
  обоих портов при неудачном первом commit, запрета commit invalid config и
  сохранения старого runtime при таком invalid draft.
- A68: normalizeSingBoxSessionNodeTarget ограничивал только node-sync, но
  node-admin тоже является transport credential с update authority, не общим
  администратором. Его можно было использовать для relay чужого domain activity
  и session commands. Real authenticated controller regression воспроизвёл
  success=true и вызов чужого runtime. Теперь обе node scopes local-only;
  полноценный admin по-прежнему может обращаться к дочернему runtime. Проверены
  GET domainActivity и POST disconnect-users, query и body nodeId.

Группа: требуется 2 новых исправления, реализовано 2, осталось 0 реализации.
Всего 68; reviewed 214/2364, pending 2150, stale 0. 6 hashes, изменённых 0468d86,
повторно просмотрены/обновлены; ещё 3 исходных файла закрыты. Новый runtime
DomainActivity и UI certificate helpers прочитаны как зависимости вне original diff.

Проверки на объединённом main:
- A67 targeted normal domainfront/service: PASS 0.008s / 0.194s;
- A68 scoped controller normal: PASS 0.132s;
- полный internal/domainfront на объединённом main: PASS 0.062s;
- targeted race domainfront/service/controller: PASS 1.022s / 2.580s / 2.385s;
- полный golangci-lint 2.15.0: 0 issues;
- go build -p 3 ./...: PASS;
- diff --check и inventory check: PASS.

Первая проверка до объединения дерева не прошла compile из-за изменения исходников
во время package load (новые SiteCatalog/SiteTemplate с прежним cache dependency).
После завершения merge повторный запуск прошёл; это не считается дефектом main.
Целевые TLS/RPC тесты используют временную SQLite, self-signed fixture и loopback,
не выпускают ACME сертификаты и не применяют host firewall rules.

DomainFrontService теперь reviewed и R06 закрыт. domainfront/manager.go и shared.go
ещё оставить pending до проверки двух новых подозрительных lifecycle контрактов:
RefreshRoutes заменяет CertificateEvent/DNS01Token вместе с routing; transport pools
старых reverse proxies не закрываются при RefreshRoutes/Stop. Сначала воспроизвести
callback loss и реальные localhost idle connections, затем исправлять. API/jobs/
subscriptions общий аудит не закрыт. Source diff всё ещё фиксирован f3baf84.


## Контрольная точка A69–A70: обновление и остановка шлюза

A67–A68 опубликованы в cb2ba6762934202252126919a78b304fa0298f3d.
Новая группа: требуется 2 исправления, реализовано 2, осталось 0 реализации.
Всего исправлений 70. Reviewed 216/2364, pending 2148, stale 0.

- A69: RefreshRoutes терял CertificateEvent и DNS01Token при замене backend.
  До правки три последовательных refresh оставляли events=0 и пустой credential.
  Теперь сохраняется полный сертификатный lifecycle backend вместе с путями,
  roots и shared flags. Это также сохраняет данные для отката A67.
- A70: reverse proxies создавали собственные HTTP/h2c transports, но не закрывали
  idle pools при замене маршрутов, ошибке построения handler и остановке gateway.
  Реальные localhost regression tests до правки удерживали upstream connections
  после refresh и stop. Runtime теперь владеет pools и освобождает их.
  Дополнительно в рамках A70 воспроизведён зависший upgraded tunnel: Shutdown
  не закрывает hijacked connections. Upgrade request привязан к runtime context,
  остановка закрывает оба конца. Refresh сохраняет действующий туннель; это
  проверено echo до/после refresh, затем EOF клиента и закрытием upstream на stop.

До независимых TLS изменений main: весь domainfront -race PASS 1.186s;
финальные три lifecycle scenarios -race PASS 1.094s; service regressions -race
PASS 3.939s; build PASS; golangci-lint 0 issues.

Main параллельно получил 75b6f4d..d74ba2381c7d95ff7370c2224d45985b290a0eb0:
распределение сертификатов с verified HTTPS, dedicated node-sync install endpoint,
session-only distribution и локальное обнаружение установленных копий. Выполнен
fast-forward; новые возможности сохранены. Прочитаны новые RPC/service целиком
и изменения пяти прежних reviewed files, их hashes обновлены. Эти новые файлы
не увеличивают фиксированный исходный scope f3baf84. Форматирование четырёх
входящих Go файлов нормализовано golangci --fix, поведение не менялось.

Свежие проверки объединённого дерева:
- targeted race domainfront/service/controller: PASS 1.129s / 4.112s / 2.573s;
- go build -p 3 ./...: PASS;
- golangci-lint 2.15.0 после форматирования: 0 issues;
- inventory и diff --check: PASS.

Общий go test ./... не запускался: остаётся ранее описанное ограничение внешнего
public-IP запроса. Host firewall и ACME issuance не запускались. R04/R05 остаются
открыты; новая передача сертификатов не считается полностью покрытой этими
lifecycle tests. Следующий приоритет: pending API/jobs и управление подписками,
а также отдельные регрессии нового distribution feature. Аудит в целом не закрыт.

Перед публикацией также включён fast-forward до 808421175ae5c779c27e610a89e5001544e540d2:
независимые TLS editor UI, отрицательные distribution tests и документация ручного
renewal fan-out. Их содержимое просмотрено, UI не изменялся в этой группе.


### A71: строгий hostname при передаче сертификата

Новый входящий TestCertificateDistributionHostnameValidation падал до правки:
https://example.com принимался как example.com. managerDomain предназначен также
для импорта URL, но distribution API обещает bare DNS hostname. Теперь результат
managerDomain должен совпасть с нормализованным вводом; случай регистра, пробелы
по краям и завершающая точка по-прежнему нормализуются. Добавлены network-path
URL, порт, путь, credentials/query/fragment. Это исправление нового параллельного
кода main, а не расширение первоначального списка 2364 файлов.
Текущая группа A69–A71: требуется 3, реализовано 3, осталось 0 реализации.
Всего 71 исправление; reviewed 216/2364, осталось 2148, stale 0.

A71: оба TestCertificateDistribution* прошли с -race (1.132s).

A69–A71 опубликованы в 4864d8c29b3f45d359a5061cdd601929f72b2947.
Последние независимые frontend commits 55ea87a/9741214 сохранены как родители.
После синхронизации обнаружен изменившийся reviewed hash useManagedSniDomains.ts;
все 48 строк повторно прочитаны, расширение union source на distributed учтено.
Этот служебный checkpoint обновляет реестр: 216 reviewed, 2148 pending, 0 stale.
CI 4864d8c запущен, на момент сохранения ещё не завершён.


## Контрольная точка A72–A73: отмена каталога и права SNI jobs

Найдена актуальная точка A71, а не устаревшая A43 из истории диалога.
Рабочая база этого запроса — main 1292f402bbbe527778e06418672e1f814351e505;
независимые TLS/cover-site изменения сохранены.

- A72: Save/Refresh ждали общий mutex даже после отмены запроса. Перед правкой
  оба blocked-waiter regression теста падали; отменённый Refresh также затирал
  LastAttempt/LastError, откладывая нормальный повтор, а RefreshDue возвращал nil.
  Теперь записи сериализуются context-aware semaphore; запросы БД получают ctx,
  отмена сохраняет errors.Is и не записывается как отказ источника. DeleteContext
  передаёт отмену HTTP удаления, прежний Delete сохранён для совместимости.
  Кэш и расписание отменённой операции остаются прежними. Ошибки реального
  источника по-прежнему фиксируются; прежний SSRF/privacy regression сохранён.
- A73: node-admin обходил ограничение local-only у SNI jobs, хотя это транспортный
  credential узла, а не полный admin. До правки реальный bearer-auth controller
  regression получил success=true и вызвал чужой runtime. Теперь node-sync и
  node-admin не пересылают start/status/cancel/history на другой узел; полный admin
  сохраняет remote operations. Проверены оба локальных представления: nil и nodeId=0.

Группа: нужно 2, реализовано 2, осталось 0 реализации. Всего 73.
Реестр: 227/2364 reviewed, 2137 pending, 0 stale. Полностью прочитаны 11 новых
исходных файлов; четыре изменённых входящими TLS commits reviewed файла повторно
прочитаны целиком. domains.go нормализован gofmt без изменения поведения.

Следующая очередь после этого checkpoint:
- Lifecycle RoutingCatalogJob: Run использует context.Background; до 100 источников
  с бюджетом 15s каждый. Проверить привязку к остановке панели и startup scheduling,
  воспроизвести до правки. Отмена HTTP каталога A72 не закрывает этот вопрос.
- OutboundAutoBalancerJob / RemoteRoutingJob: проследить отмену фонового I/O и
  интеграцию scheduler; полное чтение коротких job wrappers не закрывает service calls.
- Продолжить pending services/nodes/jobs/API и оставшуюся SQL fixture по coverage.md.
- Прежние R04/R05 внешнего ядра и ограничение общего go test ./... остаются открыты.

Новые тесты используют temporary SQLite, canceled contexts и подменённый node runtime;
внешний public-IP discovery, ACME и firewall не вызываются.

Перед публикацией выполнен fast-forward до 8de49f7113860915aef4cf2546dad07540daf9f3:
сохранено ограничение verified HTTPS для TLS lookup и новые тесты cover-site/UI.
Нормализовано форматирование входящих TLS Go-файлов и api allowlist; семантика
этих независимых изменений сохранена, отдельными исправлениями A72/A73 не считалась.

Проверки текущей группы:
- До правки: A72 regressions FAIL с воспроизведением блокировки и порчи schedule;
  A73 authenticated node-admin FAIL с remote calls=1.
- Все TestRoutingCatalog normal: PASS 0.622s.
- Controller SNI jobs/auth/scopes normal: PASS 0.832s.
- После финальных A72/DeleteContext и A73: targeted race service/controller PASS
  10.830s / 12.351s; с CertificateDistribution PASS 8.912s / 10.731s.
- Frontend build: PASS на объединённом main.
- go build -p 3 ./... и go vet -p 3 ./...: PASS на объединённом main.
- inventory check и diff --check: PASS.

Первый полный build свежей копии требовал отсутствующий dist; после настоящей
frontend сборки прошёл. Один запуск lint пересёкся с удалением dist новой Vite
сборкой и остановился на embed typecheck; он не считается дефектом исходников.

Итоговый golangci-lint 2.15.0 на объединённом дереве: 0 issues.
Сборка frontend, go build ./... и go vet ./... прошли; targeted race
service/controller с A72/A73 и CertificateDistribution прошёл.


## Контрольная точка A74–A76: lifecycle фоновых заданий

Предыдущая группа A72–A73 опубликована в 334ec7eda3519dbb2b2de11bfe0f780286c31ac1.
Работа начата с этой main; отдельный исходный scope f3baf84 остаётся фиксированным.
Нужно 3 исправления; реализовано 3, осталось 0 реализации. Всего 76.
Ручной охват 231/2364, осталось 2133, stale 0.

- A74: RoutingCatalogJob использовал context.Background вместо жизненного цикла
  панели. GORM callback regression до правки подтвердил, что запрос продолжал
  ждать после cancel. Теперь constructor принимает необязательный parent,
  Server передаёт s.ctx; RefreshDue и его DB/HTTP операции получают этот context.
  Уже отменённый tick не трогает БД; отмена не логируется как ошибка источника,
  остановленное задание не запрашивает новый restart ядра.
- A75: удалённые Happ/JSON/Clash routing refresh создавали HTTP request с
  Background, а coalesced wait и persisted load mutex не принимали cancel.
  До правки новые HTTP и waiter tests зависали до ручного освобождения.
  Добавлен context-aware entry point при сохранении прежних обёрток; cron job
  получает s.ctx. HTTP, DB Ping/query/persistence и ожидание загрузки передают
  context. Отмена снимает inflight без негативного retry delay и без замены
  last-good; отменённый waiter не останавливает чужую загрузку. После отмены
  не начинается следующий настроенный источник. Проверяется реальный localhost
  HTTPS с закрытием upstream request и отсутствие URL tokens в cancellation error.
- A76: cron.Stop не учитывает вручную запускаемые initial WrappedJob.Run.
  До правки StopPanelOnly возвращал nil до освобождения ресурсов такого задания.
  Server теперь владеет initial worker wait group, join включён в существующий
  bounded reload wait/retired-worker механизм; полный stop ждёт завершения.
  Все initial sidecar jobs и remote routing warm зарегистрированы до запуска.
  Warm и periodic remote routing используют один cron wrapper/overlap guard.
  Отложенный первый memory-release вместо Sleep использует cancelable timer,
  учитывается тем же join и не выполняется после отмены панели. Проверяются
  panic/recovery, overlap, ожидание освобождения и отмена отложенного задания.

Воспроизведение до правки:
- TestRoutingCatalogJobCancelsDatabaseWork: FAIL 1.16s, ignored cancellation.
- TestRemoteRoutingRefreshCancellationDoesNotDelayRetry: FAIL 1.00s.
- TestRemoteRoutingCanceledWaiterDoesNotCancelSharedFetch: FAIL 1.00s.
- TestPanelStopJoinsInitialScheduledJob: FAIL 0.17s, преждевременный nil от stop.

После правки первый targeted normal: job PASS 0.187s, sub PASS 2.450s,
web PASS 0.309s. go build -p 3 ./... и go vet -p 3 ./...: PASS.
Последующие результаты расширенных проверок — ниже.

Следующая очередь (не считать закрытой):
- OutboundAutoBalancerJob, ProbeAutomaticSubscriptions/ApplyAutomaticSelections:
  проверить context и bounded stop, HTTP/core probes и сохранение выбора после cancel.
- resolveEntry/triggerPersistedLoad в subscription resolver всё ещё могут запускать
  собственные background goroutines вне job. A75 закрывает scheduled/warm path,
  а не lifetime всех request-triggered background loads; проследить владельца.
- Проверить порядок остановки periodic sidecar jobs относительно StopAll: initial
  workers теперь joined, но periodic workers и финальный сбор трафика требуют
  отдельного прохода по всем managers, включая externalvpn/mieru/sudoku.
- При недоступном TLS cert listener сейчас может деградировать в HTTP: отдельно
  воспроизвести и определить безопасное поведение startup/reload.
- SQL fixture, pending services/API/UI/install и прежние R04/R05 не закрыты.

Тесты используют temporary SQLite, canceled contexts, controllable HTTP transport
и локальный self-signed HTTPS server. Общий go test ./... не запускается из-за
ранее описанного public-IP discovery; ACME и host firewall не вызываются.

Проверки A74–A76:
- Полный internal/sub: PASS 72.466s.
- Расширенный targeted -race: job PASS 3.084s; sub PASS 16.208s; web PASS 5.007s.
- golangci-lint 2.15.0: 0 issues после нормализации импортов новых файлов.
- Полные go build -p 3 ./... и go vet -p 3 ./...: PASS.
- Inventory и diff --check: PASS, 231 reviewed, 2133 pending, 0 stale.

Финальный targeted -race после добавления всех regressions: job PASS 5.280s,
sub PASS 3.170s, web PASS 4.355s. Delayed timer проверяется с synctest:
worker сначала входит в ожидание таймера, затем отменяется; полный час не выжидается.


## Контрольная точка A77–A80: автобалансировщик и процессы протоколов

A74–A76 опубликованы в a9bd1c81204cad4624cae9b2005db4265ffca393; с этой main
продолжена работа. Нужно 4 исправления, реализовано 4, осталось 0 реализации.
Всего 80; reviewed 243/2364, pending 2121, stale 0.

- A77: отменённый probe продолжал HTTP/замеры и запуск следующих пакетов,
  временное ядро оставалось живым до обычного timeout; поздний успешный результат
  менял SelectedTag после cancel. До правки regression получил count=1, selected=b,
  calls=2 после отмены. Теперь scheduler и ручная проверка передают context через
  DB/settings snapshot, health/ranking/failover, временное ядро/ожидание портов,
  очереди workers, HTTP/speed/TCP, warm GET и egress trace. Отмена возвращается
  как context error; late results не публикуются в БД и не начинают следующий
  пакет. Временное ядро всегда очищается, включая частично неудачный старт.
  Старые API сохранены как Background wrappers. Existing probe hook тестов
  получает context, но прежние offline egress stubs сохранены.
- A78: отменённый apply tick стирал AppliedTag и заменял ApplyError сообщением
  остановленного ядра. Regression до правки получил count=1 и потерю прежнего
  acknowledgement. Context теперь проходит в subscriptions/settings queries,
  selector HTTP calls, Xray apply deadline и DB writes; после отмены сохранённое
  подтверждение не затирается. Проверяются отменённый tick, зависшее чтение БД,
  прекращение job до apply и сохранность прежнего выбора. Реальный уже принятый
  ядром выбор не откатывается отменой HTTP: следующий tick заново сверяет его.
- A79: StopAll вызывался до cron join. Последний periodic reconcile мог вновь
  поднять external sidecar, после чего полный Stop возвращал nil с живым child.
  Регрессия воспроизвела это настоящим owned shell child и cron tick. Теперь
  initial/current cron/retired workers завершаются до остановки managers.
  Убраны повторные StopAll, которые больше не закрывают гонку; сохранён финальный
  TUIC traffic flush после остановки. Полный stop и panel-only bounded wait
  сохраняют свои прежние роли.
- A80: NewSudokuJob создавал отдельный cancel watcher с безусловным StopAll.
  StopPanelOnly обрывал рабочий Sudoku, а незавершённый watcher мог затронуть
  процессы новой панели. Regression до правки получил not running после reload.
  Watcher удалён; процессами владеет Server: reload их сохраняет, полный stop
  синхронно вызывает Sudoku StopAll после join заданий. Проверено настоящим
  временным child; job пропускает уже отменённый запуск.

Первый объединённый normal: web PASS 2.521s, job PASS 1.268s, service PASS 10.470s,
outbound PASS 0.120s. Расширенный -race: web PASS 8.383s, job PASS 4.336s,
service PASS 87.276s, outbound PASS 1.220s. Дополнительно cancellation/speed queue
-race outbound PASS 1.125s. Полный outbound пакет PASS 0.078s; затем полные
build -p 3 ./... и vet -p 3 ./...: PASS. Итог lint/последних lifecycle tests — ниже.

Один промежуточный lint загрузил тестовый файл одновременно с добавлением нового
импорта sync/atomic и вернул typecheck error; это не успешный прогон. Последний
запуск выполняется на неизменяемом коде. Реальные протоколы тестовыми children
не проверяются: эти regressions проверяют lifecycle, локальные HTTP/SOCKS и БД.

Следующая очередь:
- EnsureProbeOutbound и managed sidecar bootstrap внутри externalvpn ещё имеют
  собственные Background contexts/locks. A77 прекращает следующие setup шаги и
  очищает выделенные ресурсы, но уже выполняющийся managed bootstrap остаётся
  отдельным вопросом; не считать все FPTN/OpenFlux/Vetroff start paths мгновенно
  отменяемыми. Проверить эти пути и cleanup на реальных timeout scenarios.
- Финальные счётчики Mieru/externalvpn/MTProto/AWG: отдельная проверка tail traffic
  и pending batches при полном Stop; A79 регрессия доказывает порядок завершения,
  не гарантирует сохранность всех последних дельт каждого протокола.
- Request-triggered remote routing background loads, TLS downgrade при невалидном
  cert, SQL fixture и остальные pending подсистемы остаются открыты.
- setting.go и xray_setting.go изменены только в прочитанных getter/auto-apply
  участках и остаются pending до полного чтения. Новые setting_context helpers
  прочитаны целиком. Рефакторинг getters сохраняет прежние defaults/legacy core
  normalization, добавляя cancellable query без изменения write APIs.
- Прежние R04/R05 и ограничение общего go test ./... сохраняются.

Новые тесты не обращаются к public-IP API, не выпускают ACME certificates и не
применяют host firewall: используются temporary SQLite, loopback HTTP/SOCKS,
контролируемые probe results и принадлежащие тестам процессы exec sleep 60,
которые обязательно останавливаются cleanup/Server.Stop.

Итог A77–A80: golangci-lint 2.15.0 — 0 issues на неподвижном дереве.
Финальный lifecycle/TUIC tail -race web PASS 5.203s; отмена очереди speed
и временного ядра -race outbound PASS 1.125s. Полные build/vet прошли.
Inventory и diff --check прошли: 243 reviewed, 2121 pending, stale 0.

Перед публикацией выполнен fast-forward до 039f6f9 (независимый frontend DNS-01
setup/issuance для remote inbounds). Backend этой входящей группой не менялся;
прочитан новый общий setup component и diff точек встраивания. Входящие изменения
сохранены как база checkpoint, исходный scope f3baf84 не расширен.
Дополнительно прежние getter defaults/persistence/secret regressions PASS 0.742s.

После включения frontend main: новый DNS-01 component regression PASS 4/4
(12.07s), Vite production build PASS, повторные полные Go build/vet PASS.
Стандартный vitest config не загрузился из-за uv_interface_addresses/EPERM
в Storybook import. Для этих четырёх jsdom tests использован временный config
с теми же React plugin, alias и component setup, без unrelated Storybook project;
он удалён после запуска. Полный frontend test suite этим не засчитывается.

TypeScript --noEmit на объединённом frontend: PASS.


## Контрольная точка A81–A83: подготовка probe-туннелей, WEB worker, TUIC

A77–A80 опубликованы в 4e0c1b34f6c475bcfe2bda1aa8f9a590ee3db290.
Нужно 3 исправления; реализовано 3, осталось 0 реализации. Всего 83.
Reviewed 250/2364, осталось 2114; stale 0. Исходный scope f3baf84 не расширяется.

- A81: EnsureProbeOutbound не принимал context. До правки canceled waiter ждал
  чужой outbound lock; OpenFlux probe возвращал nil вместо cancellation; Docker
  image inspect продолжал работать. Все три regression воспроизведены до правки.
  Новый context-aware API используется bridgeManagedProbeOutbounds. Общий lock
  сохраняет прежние production Lock/Unlock, но probe может покинуть очередь до
  владения какими-либо ресурсами. Setup image inspect/pre-cleanup, grace period,
  SOCKS readiness, FPTN exec readiness получают context; probe child привязан к
  нему. Реальный production child не привязан к краткому caller context.
  Cleanup остаётся независимым/ограниченным, чтобы cancel не оставлял контейнер.
  Проверены удаление приватных файлов, завершение child, отмена после успешного
  старта и сохранность production tunnel с тем же исходным tag. Readiness всех
  четырёх протоколов проверена с synctest без минутного ожидания.
- A82: MtprotoJob.Run запускал WEB Proxy reconciliation в detached goroutine
  с Background/4m. Real GORM callback regression подтвердил, что Run уже вернулся,
  а worker ещё использовал БД. Теперь это отдельный синхронный MtprotoWebJob,
  зарегистрированный в cron и initial worker join сервера. Его длительная работа
  не блокирует traffic job; parent cancellation доходит до worker, overlap guard
  не запускает второй worker. Тест подтверждает, что Run ждёт освобождения
  ресурсов даже после получения cancel. MTProto traffic poll тоже получает ctx
  для WEB HTTP stats и пропускает уже отменённый tick.
- A83: aggregateTuicClientTraffic складывал raw int64 без границ и принимал
  отрицательные дельты. До правки все три identity regressions нарушали bounds.
  Теперь используется saturating/nonnegative accumulateTrafficDelta, общий для
  traffic jobs. Проверены traffic ID, legacy email, inbound+UUID и настоящий
  SQLite journal replay: сумма двух больших значений ограничена TrafficMax,
  отрицательная дельта не вычитает байты; replay той же записи не начисляет дважды.

Normal targeted externalvpn PASS 2.069s, job PASS 0.488s. Расширенные TUIC bounds
и SQLite shutdown/replay tests PASS 0.537s. Расширенные race tests: externalvpn PASS 3.387s, job PASS 4.779s,
outbound PASS 1.222s, web PASS 8.514s. go build ./... и go vet ./... PASS;
golangci-lint 2.15.0: 0 issues. Финальные TUIC aggregation/shutdown journal
race tests PASS 3.748s. git diff --check и inventory check PASS.
Воспроизведение A81: three failures 1.02s/0.50s/1.03s. A82: Run вернулся при
активном WEB DB callback (0.37s); A83: все три identity cases FAIL до правки.

Вспомогательная переносимость тестов: Unix-only sidecar regression из A79/A80
и новые shell/kill fixtures ограничены build tag !windows, чтобы syscall.Kill
не ломал Windows test compilation. Это не считается новым исправлением исходного
коммита. Удалён больше не вызываемый private vetroffSocksReady Background wrapper.

Тесты используют fake Docker executable в temporary PATH: host Docker daemon,
NET_ADMIN и реальные tunnel containers не запускаются. OpenFlux fixture — owned
exec sleep 60, который убивается и reap-ится. No real ACME or host firewall apply.
WEB traffic regression использует пустую временную БД и stub systemctl; detached
query контролируется callback, реальные listener/service mutations не требуются.

Следующая очередь:
- pendingTrafficBatch у Mieru/MTProto/externalvpn хранится внутри job и не имеет
  durable replay. При создании новой панели pending batch может быть утрачен;
  full stop пока flush-ит только TUIC journal. Нужен journal/receipt lifecycle
  с устойчивой client/inbound identity, проверками DB failure, replay once,
  rename/delete и исключением начисления новому пользователю с прежним email.
  Не заменять это простым финальным Run, который может снова запустить sidecars.
- У этих managers остаётся проверка последних runtime deltas до StopAll.
  A83 закрывает TUIC aggregation bounds, а не финальную статистику всех протоколов.
- A81 относится к isolated probes. Production apply/supervisor APIs и независимый
  bounded teardown сохраняют прежние контексты; не объявлять весь manager отменяемым.
- Telemt WEB service global operation locks/внутренние команды ещё требуют
  проверки отмены и deadlines. A82 гарантирует владельца/join worker, а не мгновенную
  отмену каждой внутренней операции ACME/systemctl.
- Request-triggered routing workers, TLS startup downgrade, SQL fixture и остальные
  pending подсистемы, прежние R04/R05 остаются открыты. Общий go test ./... не запускать
  до исправления ранее описанного public-IP discovery в тестах.


## Контрольная точка A84: durable traffic retry для трёх sidecar jobs

A81–A83 опубликованы в 7edf41ede245ecd42a5f80e555397c9dcecbd019.
Нужно закрыть 3 пути (Mieru, MTProto, external VPN); реализация подключена к 3/3.
Это один общий дефект lifecycle pending batch, A84; общий итог 84 исправления.
Reviewed 250/2364, осталось 2114, scope не расширялся. На этом этапе обновлены
доказательства ранее просмотренных файлов, новые исходные файлы не добавлялись.

До исправления pendingTrafficBatch принадлежал экземпляру job и сохранял только
срезы traffic с email/tag. Panel replacement создаёт новый job и теряет batch,
который runtime collector уже вычел из своего cursor. Между повторами email/tag
могли перейти к другому владельцу. В коде не было journal/receipt для этих jobs.

A84:
- Каждый batch перед счётчиками записывается через temporary file + fsync + rename
  + directory fsync; используется общий с TUIC примитив без изменения его формата.
- Stable traffic-row и inbound IDs захватываются до потребления runtime counters,
  включая disabled owners, legacy inbound_id и normalized shared-client links.
  Replay следует за переименованными владельцами и пропускает удалённые ID.
  Названия не используются для поиска нового владельца при восстановлении.
- SidecarTrafficReceipt записывается в той же DB-транзакции, что bytes. Повтор
  после неоднозначного commit/unlink не начисляет batch второй раз. Модель включена
  в обычную и cross-dialect migration. Post-commit maintenance использует прежний
  quota/runtime path и не превращает committed bytes в failed accounting.
- Все три job сначала восстанавливают journal, затем получают desired/runtime
  configuration; повреждение или недоступность journal останавливает потребление
  новых samples. Per-source process locks объединяют retired/replacement jobs.
- Если WAL нельзя записать, batch остаётся у процесса, а не у старого panel job.
  Новая panel task повторяет его запись до сбора следующего runtime sample.

Проверки используют temporary SQLite DB и synthetic counters/filesystem faults.
Для каждого из 3 sources проверены: accounting failure и rollback receipt,
потеря всех first-job state, replay, rename client/inbound, передача старых names
новым владельцам и ambiguous repeated WAL. Отдельно проверены delete/recreate,
ошибка файловой записи с восстановлением новой job и реальные Run entry points
всех трёх jobs до configuration queries. Query failure guard исключает запуск
реальных sidecars/Docker/systemctl/firewall в entry-point tests.
Первый normal targeted job run PASS 1.004s (до добавления entry-point tests).
go build ./... и go vet ./... PASS. golangci-lint 2.15.0: 0 issues.
Итоговый normal TestSidecar* (включая Run entry points): PASS 0.243s.
Расширенный -race: job PASS 8.367s, service PASS 21.860s, database PASS 2.845s.
Inventory check и git diff --check PASS: reviewed 250, pending 2114, stale 0.

Ограничения и следующая очередь:
- Это persistence уже снятых periodic batches, не финальная статистика перед
  StopAll. Нужен отдельный drain-before-stop для Mieru/MTProto/external VPN,
  включая routed totals/WEB. Не заменять drain вызовом Run: он может запустить
  sidecars заново. Полный останов пока имеет специальный tail journal только TUIC.
- При process crash между runtime sampling и успешным fsync, а также при отказе
  диска с одновременной потерей процесса, нельзя гарантировать сохранность sample.
  Нужен runtime cursor ACK или восстановимый источник для устранения этого окна.
- Receipts сохраняются, как TUIC receipts; безопасную bounded retention/согласованное
  восстановление DB backup вместе с filesystem WAL проверить отдельно. Восстановление
  старой DB с повторным использованием ID не входит в доказанную identity гарантию.
- PostgreSQL live replay в этом этапе не запускался; SQLite migration/receipt path
  проверяется целевыми тестами. Предыдущие результаты PostgreSQL не выдавать за новый прогон.
- Remaining production cancellation/Telemt operations, routing workers, TLS
  downgrade, SQL fixture и R04/R05 остаются в общей очереди.


## Контрольная точка A85: финальный опрос sidecars перед полным остановом

A84 опубликован в 8188e8fd36f99ecee6b2574e0a6c76bc7cf3ddb9.
Нужно 3 пути остановки, подключены 3/3: Mieru, raw/WEB MTProto, external VPN.
Это один дефект порядка shutdown/accounting, A85; всего 85 исправлений.
Reviewed 253/2364, осталось 2111. Полностью прочитаны Mieru manager/traffic,
MTProto manager/consistent collector; три новых original-scope файла отмечены.
Mieru traffic.go и новые journal/shutdown helper files вне original scope,
поэтому не увеличивают число просмотренных исходных файлов.

Доказательство до правки: TestShutdownPersistsMieruTrafficBeforeProcessRemoval
запускает принадлежащий тесту exec sleep, устанавливает cumulative baseline 10/20,
потом увеличивает sample до 110/220. Panel-only reload сохраняет runtime; полный
Stop до правки оставил client bytes 0/0 вместо 100/200 (FAIL 0.11s).
Первоначальный OpenFlux Unix-socket fixture был недоступен в sandbox (operation
not permitted) и не является product regression. Заменён loopback HTTP metrics
fixture через TrustTunnel config; production Unix socket код не менялся.

A85:
- После join всех startup/cron/retired/API producers Server вызывает отдельный
  StopSidecarsWithTraffic до остановки основного core/traffic writer.
  Каждый source под process-wide traffic lock снимает статистику, пишет WAL и
  немедленно вызывает свой StopAll (defer выполняется и при ошибке). Все 3 источника
  обрабатываются независимо, ошибки объединяются и возвращаются из Server.Stop.
- Mieru сохраняет копию фактически применённых users при start/noop/reload.
  Shutdown CollectRunningTraffic не фильтрует недавно disabled rows через desired
  DB-конфигурацию и не запускает/переконфигурирует процессы.
- MTProto collector возвращает actual running RouteThroughXray metadata. На shutdown
  client bytes всегда сохраняются, routed inbound total повторно не начисляется.
  WEB использует независимый от уже canceled panel context ограниченный HTTP poll;
  успешный raw sample сохраняется даже при WEB error. System-managed WEB backend
  не получает новых команд stop/start в этом изменении.
- PersistSidecarTrafficBatch сохраняет receipt/counters без post-commit quota/runtime
  maintenance. Обычные periodic jobs продолжают прежнее применение квот.
- При уже упавшем DB write старый WAL не мешает сохранить второй, финальный WAL.
  Если старый batch не удалось записать даже в filesystem, он не перезаписывается
  новым sample. Без доступных stable owner IDs курсоры не потребляются, StopAll
  всё равно выполняется, а ошибка возвращается вызывающему коду.

Проверки после исправления:
- Настоящие owned child processes + synthetic metrics: Mieru, TrustTunnel,
  raw/direct MTProto и routed MTProto. Проверяются client/inbound totals, отсутствие
  двойного routed total, отсутствие запущенного процесса после Stop, panel-only
  reload и сохранение TUIC final flush. Shell fixtures только exec sleep; HTTP
  endpoints только loopback, config/files/SQLite только temporary directories.
- Все 3 WAL sources: предыдущая periodic DB failure + новая shutdown DB failure,
  два durable файла, восстановление один раз, direct/routed counters, отсутствие
  maintenance даже при unrelated expired inbound. Owner query failure не запускает
  sampling и не мешает StopAll. Частичный WEB failure не теряет raw sample.
- Расширенный -race: web PASS 5.878s, job PASS 6.829s, mtproto PASS 1.121s,
  service PASS 1.616s, domainfront PASS 1.029s. Mieru standalone selected regex не
  содержит тестов; фактический manager/collector проверен web integration tests.
- Нормальный combined targeted прогон: web PASS 0.493s, job PASS 0.321s,
  mtproto PASS 0.044s. Полные go build ./... и go vet ./... PASS.
- Итоговый golangci-lint 2.15.0: 0 issues. Финальные shutdown job tests
  (включая partial WEB failure): -race PASS 3.157s. Inventory/diff check PASS, stale 0.

Во время работы main продвинулся до d57cb69 (DNS-01 и custom cover sites,
включая follow-up frontend controls и JSON escaping body cap).
Изменения fast-forward включены без пересечения с локальным shutdown patch.
Обновлены прежние review hashes domains.go/domain_front.go после просмотра diff,
endpoint guards и новых helper/HTML rendering integration; custom HTML validation
и rendering tests включены в race checks. Чужие feature commits не считаются
новыми исправлениями этого аудита. Frontend в этом этапе не изменялся локально.

Оставшиеся ограничения и следующий приоритет:
- Это последний доступный pre-stop snapshot, а не транзакционная остановка сети.
  Между ответом metrics API и завершением активных соединений могут прийти байты.
  Для строгой гарантии нужен upstream quiesce/drain + финальные счётчики после
  закрытия соединений либо durable runtime cursor ACK. Не заявлять нулевую потерю
  при crash, недоступном API, отказе диска и DB owner-query failure.
- Existing raw/Mieru/external scrapers часть ошибок возвращают как empty sample;
  отдельный явный статус/ACK API и last-tick handling для CRUD removal/restart,
  а также финальный Xray/native-core bridge accounting остаются в очереди.
- WEB HTTP poll имеет ограничение, но внутренний systemctl и service operation
  locks ещё не стали полностью context-aware; реальный host service/ACME здесь
  не запускался. No host firewall/NET_ADMIN/Docker daemon modifications.
- Остальные production cancellation, routing workers, TLS downgrade, SQL fixture,
  receipt retention и предыдущие R04/R05 по-прежнему открыты.


## Контрольная точка A86: ошибка опроса больше не считается пустой статистикой

A85 опубликован в 3a10904ea64723fd43f457a66acca3a362972d0c.
Нужно 3 checked paths, подключены 3/3: Mieru, MTProto, external VPN.
Один общий дефект failure reporting/partial accounting, A86; всего 86 исправлений.
Reviewed 255/2364, осталось 2109; полностью дочитаны external manager и additional
traffic collectors, остальные ранее полные reviews обновлены после изменений.
Mieru traffic.go и новые helper/test files не расширяют original-scope inventory.

До правки TestShutdownReportsUnavailableSidecarMetrics воспроизвёл три FAIL:
Mieru get users exit 1, Telemt HTTP 503, TrustTunnel HTTP 503. Во всех случаях
Server.Stop вернул nil, как будто последний опрос успешно не нашёл трафика.
Normal pre-fix web suite FAIL 0.640s, все три subtests провалены.

A86:
- Публичные checked collectors возвращают successful deltas и отдельную error;
  старые экспортированные сигнатуры сохранены для совместимости. Все три periodic
  jobs используют checked path и логируют incomplete poll. Shutdown сохраняет
  доступные samples, обязательно останавливает менеджеры и возвращает ошибки.
- MTProto проверяет HTTP/JSON envelope OK и data, trailing/malformed/oversize
  response и отрицательные total_octets. Неуспешный poll не двигает baseline.
- TrustTunnel проверяет HTTP, JSON целиком и размер ответа. FPTN больше не считает
  префикс ответа за полный snapshot на границе 4 MiB. OpenFlux возвращает явный
  отказ IPC/frame/status; protocols без per-client API не дают ложную ошибку API.
- В checked diagnostics нет URL/metrics key, response body, stdout/stderr или
  auth token. Используется protocol/inbound ID и безопасная категория отказа.
- Mieru проверяет exit status и фактический формат таблицы. Pinned upstream
  pkg/cli/server.go serverGetUsersFunc и common.ByteCountIEC просмотрены локально:
  User/LastActive/1DayDown/1DayUp header, compact IEC, '-' для отсутствующих counters.
  Empty legitimate table отличается от диагностики на stdout. Проверяются timestamp,
  unit и переполнение числового значения; старый permissive parsing не используется.
- Mieru serialized collector проверяет owner после scrape: ответ старого процесса
  не засеивает cursor нового. У уже умерших Mieru/MTProto owners нет ложного
  successful empty result. Это не восстановление counter source после crash.

Тесты:
- Owned sleep children + temporary SQLite/files и loopback HTTP: shutdown всех
  трёх managers явно сообщает отказ. Старый dummy OpenFlux lifecycle test теперь
  ожидает диагностический metrics error, затем по-прежнему проверяет child exit.
- Healthy external source не теряется рядом с HTTP 503; failed baseline 7 сохраняется,
  recovery 35 даёт 28, healthy 10 -> 15 даёт 5. Проверены invalid/null/trailing/oversize
  JSON, отказ FPTN/OpenFlux и отсутствие private response/URL/key в errors.
- Mieru command failure/recovery, controlled process replacement во время scrape,
  неизменность нового cursor, пустая корректная таблица/невалидный stdout/overflow.
  Старый permissive parser первоначально принял произвольный stdout; обнаружено
  тестом и исправлено до публикации. MTProto failed/missing-data/negative/oversize
  envelope отклоняется, valid empty array принимается.
- Расширенный race: web PASS 9.357s, job PASS 7.886s, mtproto PASS 1.256s,
  externalvpn PASS 2.838s. Первоначальная Mieru проверка выявила parser failure;
  после исправления финальный targeted -race: mieru PASS 1.065s, web PASS 3.329s.
- Normal targeted после checked integration: web PASS 2.404s, mieru PASS 0.023s,
  mtproto PASS 0.019s, externalvpn PASS 0.050s (до финального strict parser).
- Финальные go build ./... и go vet ./... PASS при -p 1 / GOMAXPROCS=1 /
  GOGC=20 / GOMEMLIMIT=512MiB. Полный golangci-lint 2.15.0 PASS: 0 issues
  (concurrency 1 / GOGC=50 / GOMEMLIMIT=1GiB / timeout 10m). Предыдущий
  memory-limited lint остановлен по timeout 5m и не считается PASS. Один lint запуск остановлен
  с exit 137, второй поймал временный typecheck во время изменения файла. Затем
  cache filesystem заполнился; проверки с no-space errors не считать успешными.
  Воспроизводимый compiler cache перенесён частично в свободный tmpfs без удаления
  исходников/пользовательских файлов; финальные проверки выполняются заново.

Остаётся:
- Предостановочный drain для индивидуального CRUD Remove/Pause/restart и reconcile
  replacement: это изменение диагностирует неуспешный poll, но не добавляет durable
  tail queue на каждой замене процесса. Не объявлять A85/A86 нулевой потерей всех bytes.
- Crash/metrics API outage/source reset нельзя восстановить одной error-сигнатурой;
  нужны upstream quiesce/drain и durable cursor ACK. Existing online presence на
  failed poll требует отдельной проверки, как и лишние warnings при долгом outage.
- Checked collectors сохраняют прежние bounded internal timeouts, а не обещают
  caller-context cancellation во всех API. Полный active WEB host-service и реальный
  OpenFlux Unix IPC не запускались; no host firewall/ACME/Docker daemon mutations.
- SQL fixture, production background lifecycle, receipt retention, TLS downgrade,
  остальные original-scope pending и R04/R05 остаются открыты.


## Контрольная точка A87: передача счётчиков при замене отдельного процесса

A86 опубликован в 449e6398f2cc56ad786f18e0c6a4d55e5bedcb81.
Нужно 3 managers, подключены 3/3: Mieru, MTProto, external VPN.
Один общий lifecycle/accounting дефект A87; всего 87 исправлений.
Reviewed 256/2364, осталось 2108. Дополнительно полностью прочитан Mieru process.go;
ранее полные manager/collector reviews обновлены. Новые helper/test/shared package
не увеличивают original-scope inventory.

Доказательство до правки: в отдельном checkout 449e639 с одним новым regression
TestMieruLifecycleJournalsConsumedTail все 4 subtests (Remove, Pause, restart,
Reconcile removal) FAIL: после cumulative 10/20 -> 110/220 журнал пуст. Нормальный
baseline web FAIL 0.169s. Этот checkout изолирован от реализации; build error
промежуточного меняющегося рабочего дерева не считается доказательством дефекта.

A87:
- Shared sidecartraffic.Owner закрепляет local inbound ID и traffic-row IDs
  обслуживаемых имён при старте или перед применением нового user configuration.
  Capture ограничен context 5s; unchanged running instances не делают новый lookup,
  если owner уже привязан. Standalone managers без panel DB/unowned inbound остаются
  совместимыми, но не получают panel-accounting identity задним числом.
- До Stop/Remove/Pause/Reconcile removal/restart и перед hot reload менеджер снимает
  текущие counters и передаёт delta в WAL. Файл/DB owner во время stop не ищется по
  email или tag, поэтому переименование и повторное использование имён не меняют
  получателя старого пакета. Inbound metadata pinned по ID; deleted rows при replay
  по-прежнему пропускаются существующим receipt/accounting path.
- Никакого AddTraffic/quota/runtime maintenance под manager locks: handoff пишет
  только shared WAL. Periodic job сначала восстанавливает накопленные packets через
  прежние atomic receipt/counters. Формат batch совместим с A84; типы сервиса — aliases.
- Shared atomic-file helper используется и TUIC: write/fsync/rename/directory sync.
  Имена journal ID валидируются. При ошибке filesystem batch принадлежит process-wide
  immutable retry queue до повторной записи; manager может безопасно отключить
  удалённого/disabled пользователя, не сохраняя ему сетевой доступ ради accounting.
  Очередь переживает смену panel job, но не process crash до успешного fsync.
- Mieru/MTProto handoff повышает traffic epoch. Запоздавший ответ periodic scrape
  от прежнего состояния того же процесса после reload не перезаписывает новый cursor.
  Это дополняет owner-pointer guard A86. External counters защищены statsMu.
- Failed final scrape логируется, затем lifecycle action продолжается; crash/dead
  process и недоступный API не выдаются за восстановленные байты. Full Stop сохраняет
  A85 explicit diagnostic и теперь дополнительно пытается снять маленький tail в StopAll.

Проверки:
- Baseline 4 FAIL описаны выше. После подключения Mieru same regressions PASS 0.189s.
- Все 3 managers, включая direct/routed MTProto: Remove/restart/reconcile removal,
  Mieru Pause и hot reload, MTProto hot reload. Owned exec sleep children, fake CLI,
  loopback metrics, temporary SQLite/config/WAL. Replay follows renamed original
  traffic ID и не начисляет bytes новому owner старого email. Повторный replay once.
- Mieru blocked journal directory: Remove всё равно останавливает runtime, затем
  новая job восстанавливает process-owned packet после исправления filesystem.
- Controlled overlapping old scrape + reload проверены для Mieru и MTProto:
  handoff 200/400 (MTProto 200 aggregate), старый ответ отклонён, следующий sample
  даёт только 10/20 (MTProto 10), WAL не начисляется дважды.
- Normal combined regression: web PASS 5.093s, job PASS 0.309s, mieru PASS 0.030s,
  mtproto PASS 0.020s, externalvpn PASS 0.015s. Финальные lifecycle + оба concurrent
  handoff tests normal web PASS 2.616s.
- Расширенный -race: web PASS 11.410s, job PASS 4.676s, mieru PASS 1.058s,
  mtproto PASS 1.375s, externalvpn PASS 1.124s. Shared package не имеет отдельного
  test binary и проверяется через эти integrations, не считать [no test files] тестом.
- Финальные оба concurrent reload handoff + Mieru disk retry: web -race PASS 3.510s.
  Incoming certificate scope/session guards: controller normal PASS 0.033s.
- Финальный external preflight/rollback: web -race PASS 4.996s, externalvpn
  regression -race PASS 3.067s. Новый отрицательный startup fixture подтверждает,
  что rollback возвращает прежнего owner, а не владельца переиспользованного email.
  Handoff external выполняется до записи replacement credential/certificate files;
  teardown повторно не опрашивает уже перезаписанный источник trust material.
- Итоговые go build ./..., go vet ./... PASS (-p 1 / GOMAXPROCS=1 / GOGC=50 /
  GOMEMLIMIT=1GiB). golangci-lint 2.15.0 полный проход: 0 issues. Inventory/diff
  checks PASS: reviewed 256, pending 2108, stale 0.

Incoming main fast-forward 710e8b3 (certificate fleet/preparation/registry workflows)
сохранён без пересечения с lifecycle patch. Changed contexts в ранее полностью
проверенных файлах перечитаны: managed-vs-inherited issuer, master-only cover
certificate export, session-bound prepare/fleet, allowlist и безопасные remote
certificate metadata. Incoming feature commits не считаются fixes этого аудита;
их новые крупные исходники не отмечаются автоматически как original reviewed.

Ограничения/следующая очередь:
- Это pre-mutation sample, не upstream quiesce/drain активных connections. Между
  metrics response и завершением старого процесса ещё могут прийти bytes; нужны
  upstream финальные counters/ACK для строгой гарантии. Reset/crash/API outage
  и отсутствие исходной baseline не восстанавливаются локальным WAL.
- Identity binding здесь относится к runtime-owned handoff. Periodic A84 batches
  по-прежнему захватывают current DB owners отдельно: concurrent rename/reuse и
  standalone-to-panel adoption требуют отдельного прохождения end-to-end.
- Ошибки записи сохраняются в памяти и логируются, CRUD signature/availability
  остаются прежними. При долгом disk outage retry queue может расти; при crash до
  fsync память будет потеряна. Receipt retention/backup consistency остаются открыты.
- WAL handoff не выполняет обслуживание квот синхронно; bytes становятся видны в
  БД на следующем successful replay. Строгие immediate UI counters не обещаются.
- Приоритет следующего прохода: stale desired/config apply после DB delete/disable.
  Capture не является авторизационным generation guard: standalone/unowned runtime
  сохраняет прежнюю совместимость, и менеджер может принять устаревший Ensure.
  Проверить, что удалённая/disabled row не запускается последним stale reconcile;
  это отдельный lifecycle дефект, не закрытый сохранением tail packets A87.
- Scope протоколов ограничен их существующими metrics APIs. Full real upstream,
  host WEB/systemctl/ACME, Docker/NET_ADMIN и OpenFlux Unix IPC не запускались.
- Новые manager-to-DB bindings проверены на temporary SQLite/post-commit paths;
  live PostgreSQL handoff/DB lock stress и remaining request/background lifecycle,
  TLS downgrade, SQL fixture и прежние R04/R05 остаются в общей очереди.


## Контрольная точка A88: проверка актуальности sidecar inbound перед запуском

Нужно 3 managers; исправлены 3: Mieru, MTProto, external VPN.
Один подтверждённый дефект A88; всего 88 исправлений.
Охват исходного коммита: reviewed 257/2364, осталось 2107.
Полностью дочитан internal/mtproto/settings_test.go; остальные новые helper/tests
не входят в исходный inventory и не увеличивают исходный охват.

Причина: старый desired snapshot или отложенный Ensure мог заново запустить
подключение после delete/disable/move-to-node/protocol-change. Accounting Capture
не является авторизацией, а cached/no-op Ensure вообще обходил повторное чтение БД.

`sidecartraffic.Current` читает только id/enable/node_id/protocol пакетами по 900
с общим timeout 5 секунд. Разрешены только существующие enabled local rows с точно
совпадающим protocol, включая смену протокола внутри external VPN. Каждому desired
элементу соответствует отдельное решение, включая повторяющиеся IDs с разными
протоколами. DB=nil сохраняет standalone contract; закрытая/недоступная panel DB
возвращает ошибку и не переводится в разрешение запуска.

Все три Ensure проверяют актуальность под manager mutex до no-op/sweep/config
writes. Reconcile проверяет весь desired до любых stop/start/sweep и фильтрует
отозванные элементы в новую slice. DB read failure оставляет текущий runtime
нетронутым. Explicit empty desired/Remove/StopAll не требуют DB availability.
Блокировка связывает check/start с последующим CRUD Remove: если изменение БД
произошло после проверки, Remove дождётся запущенного процесса и остановит его.

Доказательства:
- На прежней логике aff1af2 все 24 regression subtests упали, web FAIL 5.171s:
  3 managers × 4 причины отзыва × Ensure/Reconcile. Только тесты/неподключённый
  helper были добавлены; production manager paths оставались исходными.
- После исправления общий selected run: sidecartraffic PASS 0.453s,
  web PASS 13.178s, job PASS 1.649s, externalvpn PASS 0.423s.
  Mieru/MTProto в этом фильтре не имели matching tests — не считать их отдельными
  unit-test прогонами. Интеграционные web fixtures исполняли все 3 managers.
- Финальные 24 сценария с дополнительным cached/no-op Ensure guard и DB-outage
  runtime-preservation fixture: web PASS 4.362s.
- Unit coverage: 1802 rows/3 DB batches, disabled/moved/exact-protocol/missing rows,
  duplicate ID с другим протоколом, sanitized DB error и empty-desired без query.
- Старые lifecycle/shutdown fixtures теперь создают enabled rows перед Ensure;
  перед accounting-only replay явно выключают inbound. Их прежние assertions
  о handoff/rename/routed/replay/concurrent-poll не ослаблены.

Ограничения и следующая очередь:
- Это guard существования/enable/local/protocol, не revision guard содержимого
  settings/clients. Same-protocol credential edits и stale omission новых rows
  остаются следующей очередью; предпочтителен единый generation contract для
  формирования desired и применения runtime updates.
- Прежние ограничения pre-stop samples/volatile metrics APIs A87 сохраняются.
- Real upstream, host firewall/systemctl/ACME, Docker/NET_ADMIN, Unix IPC и live
  PostgreSQL не запускались. Fixtures используют temporary SQLite, owned fake
  children и loopback HTTP. Полный go test ./... не запускать: есть внешние probes.

Дополнительная проверка A88:
- Selected manager/service regressions PASS: Mieru 0.039s, MTProto 0.193s,
  external VPN 0.658s, service 6.316s. Среди них service post-commit no-op edit,
  credential replacement и reset depleted quota; standalone managers сохранили
  прежнее поведение без panel DB.
- Полностью прочитанный исходный settings_test.go: native settings/username
  round-trip PASS 0.014s. Reviewed 257, pending 2107, stale 0.

Итог A88: go build -p 1 ./... PASS, go vet -p 1 ./... PASS;
golangci-lint 2.15.0 full ./... PASS, 0 issues. Inventory check и staged diff
check PASS: reviewed 257, pending 2107, stale 0. Для A88 -race отдельно не
запускался; перечисленные временные SQLite/process/loopback regressions — normal
runs. Предыдущие race результаты A87 не считать новым прогоном A88.


## Контрольная точка A89: версии настроек и доступа клиентов перед применением

Нужно 3 managers; сделано 3: Mieru, MTProto, external VPN.
Всего 89 подтверждённых исправлений. Reviewed 258/2364, осталось 2106, stale 0.
Полностью прочитан исходный inbound_mtproto_desired_test.go, добавлена проверка
равенства версий интерактивного и периодического filtered payload.

A88 проверял только существование/enable/local/protocol. Задержанное задание всё
ещё могло восстановить старые credentials, manual client enable или quota block
state, оставаясь в том же inbound/protocol. External startup rollback также мог
восстановить уже отозванные credentials после неудачного запуска replacement.

A89:
- Shared RevisionOf: SHA-256 saved id/protocol/tag/listen/port/settings. Counters и
  panel-only remark не инвалидируют payload. Изменение формата settings JSON
  считается новой saved representation, но не меняет runtime fingerprint.
- DisabledRevision: детерминированный digest полного множества depletion-disabled
  email для inbound; отслеживает как блокировку, так и последующий quota reset.
- Все три constructors переносят provenance в Instance; periodic Desired builders
  штампуют eligibility из того же disabled snapshot, которым фильтруют clients.
  buildInboundForLocalRuntime сохраняет raw source revision до фильтрации и
  eligibility revision из прочитанных ClientTraffic, включая settings без clients.
- Два временных поля model.Inbound исключены из gorm/json/form. Миграции/передача
  API provenance не нужны; unit fixture подтверждает отсутствие persistence/JSON.
- Current читает saved fields и disabled rows в одной read-only по действию GORM
  transaction, batch 900, общий timeout 5s. Все managers проверяют перед любым
  mutation под mutex. Missing provenance либо mismatch возвращает ErrStaleDesired;
  Reconcile полностью пропускает stale batch, сохраняя новый live runtime.
- Дополнительная проверка active client names запрещает передавать в runtime
  известные blocked emails. DB=nil сохраняет standalone contract. Explicit empty
  desired возвращается без DB transaction и по-прежнему может остановить всё.
- External rollback повторно проверяет provenance старого runtime. Если он уже
  отозван или состояние БД недоступно, старый процесс удаляется без восстановления
  старых credentials. Разрешённый rollback остаётся рабочим. No-op external apply
  обновляет только provenance, чтобы benign saved-format change не делал живую
  конфигурацию необоснованно устаревшей при последующем rollback.

Доказательства A89:
- До подключения проверки версий все 18 subtests упали: web FAIL 4.763s,
  3 managers × credentials/manual-disable/quota-disable × Ensure/Reconcile.
  Nine stale Ensure accepted; nine stale Reconcile overwrote newer config files.
  Provenance fields на этом шаге уже добавлены, но A88 guard их ещё не проверял.
- Отдельный external rollback regression до rollback guard FAIL 0.773s:
  failed replacement восстановил old-password после commit new-password.
- Первый selected run после защиты: sidecartraffic PASS 0.189s, web 11.622s,
  job 0.356s, service 6.319s, Mieru 0.025s, MTProto 0.006s, external VPN 0.425s.
- После проверки provenance parity и пути settings без clients: final selected
  web PASS 8.157s, sidecartraffic 0.335s, desired parity service 0.296s.
- Отдельный финальный service run desired/quota-reset/no-op update/fallback payload:
  PASS 1.812s. Существующие assertion об effective clients не ослаблены.
- После rollback guard: selected web PASS 10.936s, external VPN 0.433s,
  sidecartraffic 0.197s. Включены оба stale desired набора A88/A89, новый rollback,
  старые lifecycle/handoff/replay/разрешённый rollback tests и helper regressions.
- Unit tests: old/missing revision, saved credentials, quota block и reset в обоих
  направлениях, counters/remark stability, local filtering provenance,
  deterministic disabled digest, JSON/DB exclusion и batch/read-failure A88.

Ограничения / следующая очередь:
- Это provenance контракта внутреннего builder, а не публичный authentication
  token. Direct panel Instance обязан нести provenance; production paths получают
  его через constructors и local builder. Synthetic accounting fixtures явно
  связываются со своей saved fixture row. Standalone DB=nil не получает DB guard.
- Полный desired-set generation ещё не реализован. Stale empty/omitted desired
  либо отдельный delayed Remove/DelInbound может остановить новый runtime; в том
  числе остаётся старый случай protocol-change внутри external manager. Следующий
  приоритет — согласованная версия всего reconcile set и операций удаления.
- Проверка не заменяет post-commit apply: если DB изменена после guard, её более
  новый apply/Remove сериализуется тем же mutex. При отказе caller runtime позже
  восстанавливает периодическая сверка, как и ранее.
- Повторное создание тех же client identities с прежними credentials и сохранение
  accounting ownership при semantic no-op требуют отдельного generation анализа.
- Ограничения pre-stop metrics samples и A87 WAL/retry scope сохраняются.
- Normal temporary SQLite/owned child/loopback tests; real upstream, PostgreSQL,
  host firewall/systemctl/ACME, Docker/NET_ADMIN и Unix IPC не запускались.
  Для A89 отдельный -race не запускался; старые race результаты не считать новыми.

Итог A89: go build -p 1 ./... PASS, go vet -p 1 ./... PASS;
golangci-lint 2.15.0 full ./... PASS, 0 issues. Inventory/diff checks PASS:
reviewed 258, pending 2106, stale 0. Source стабилен на этих финальных checks;
первый pre-rollback full-check run был остановлен до новых production edits и
не считается финальным результатом. Все результаты A89 выше — normal test runs.


## Контрольная точка A90: stale empty desired и запоздавшие удаления

Нужно 3 managers; сделано 3: Mieru, MTProto, external VPN.
Всего 90 подтверждённых исправлений. Reviewed 259/2364, осталось 2105, stale 0.
Полностью прочитан исходный inbound_mieru.go, в том числе validation,
quota/egress/speed-alias filters и post-commit immediate apply.

После A89 оставались две destructive части того же desired-state lifecycle:
старый пустой/неполный job snapshot мог остановить новый процесс, а delayed
DelInbound/empty-client UpdateInbound удалял актуальный runtime, не проверяя БД.

A90:
- ReconcileCurrent принимает внутренний read-only resolver. Захватывает manager
  mutex до построения полного desired set и удерживает до sweep/remove/start.
  Context-based source read получает timeout 5s; отмена проверяется до и после
  resolver. A88/A89 Current сохраняет свою последующую проверку и свой timeout.
- Все 3 periodic jobs, повторные quota refresh Mieru/external и reset-all MTProto
  используют новый API. Возвращённый filtered desired используется для tags и
  Mieru poll; prefetched desired arrays больше не применяются production callers.
- Desired*InstancesContext использует один context-bound DB handle для всех
  запросов builder; Mieru ListForInbounds получает тот же handle, а не DB global.
  Старые getter wrappers остаются совместимыми для прочих read callers.
- RemoveIfInactive под тем же manager mutex читает актуальный saved inbound и
  disabled clients через shared readState. Удалять можно missing/disabled/remote/
  вне protocol ownership либо без активных effective clients. MTProto WEB mode
  освобождает обычный native manager; Pingtunnel сохраняет свою key-only модель.
- External проверяет весь managed protocol set, поэтому старое удаление TrustTunnel
  не удалит уже действующий OpenFlux того же ID. Проверка callback использует
  текущий protocol, не старый payload. DB read failure возвращает sanitized error
  и сохраняет runtime; обычный local runtime переводит все соответствующие Remove
  branches на guarded API, включая empty clients и protocol transitions.
- Force Remove/StopAll и low-level Reconcile сохраняют прежний явный контракт:
  аварийная остановка/компонентные операции не зависят от DB availability. Новые
  production resolver paths не должны заменяться вызовами Reconcile(prefetched).

Доказательства:
- На прежней логике 42ea00b web FAIL 3.742s: 9 failures delayed deletion/empty
  clients/read outage (3 managers × 3 destructive причины), плюс 3 failures empty
  job snapshot vs newly enabled startup. 9 positive disabled/deleted/depleted
  сценариев уже проходили. Production code на baseline не менялся.
- Barrier ставится после фактического пустого Desired SELECT. Между snapshot и
  продолжением job БД включает inbound, Ensure запускается параллельно. После
  правки Ensure присоединяется к manager lock и запускается после старой сверки;
  callback не подменяет desired и не вызывает host/network tooling.
- Selected regressions: web PASS 20.098s, sidecartraffic 0.407s, job 0.439s,
  service 1.849s, Mieru 0.008s, MTProto 0.014s, external VPN 0.421s. Runtime
  package в этом фильтре имел no tests to run — не считать отдельным test coverage.
- Final focused regressions: web PASS 5.840s (18 removal cases + 3 job races +
  3 resolver error/cancellation cases), sidecartraffic PASS 0.135s.
- Дополнительные post-commit/defer-to-commit/enable/core-bridge accounting tests:
  service PASS 0.597s. Existing lifecycle/WAL/replay/shutdown/A88/A89 assertions
  не ослаблены.
- Unit CanRemove: manager protocol ownership, переход между двумя протоколами
  одного manager, inactive callback, DB error sanitization/fail-preserve,
  remote/missing row. Resolver failure/cancel tests подтверждают сохранение child
  и наличие deadline; cancelled resolver не вызывается.

Ограничения / следующая очередь:
- Не введён persisted global generation counter. Вместо него production full-set
  builder выполняется под тем же mutex, что runtime mutation; DB commit после
  snapshot опирается на следующий post-commit Ensure/Remove, который присоединяется
  к этому mutex. SQL resolver timeout и последующий Current timeout — разные фазы;
  ожидание sync.Mutex не является context-cancellable до захвата.
- Force manager API предназначен для lifecycle/component/shutdown и тестов;
  он намеренно может остановить enabled DB row. Native speed handover Pause/
  core apply и прочие managers не переводились на новый eligibility contract.
- Readiness egress/limit aliases и startup retry остаются прежними. Большие IN
  запросы в source builders отдельно не stress-tested; Current/readState batch900.
- Следующий приоритет: согласованность periodic traffic owner snapshot с новыми
  inbounds/client identities, созданными между CaptureSidecarTrafficOwners и poll;
  проверить replay/сохранение accounting при semantic no-op и identity recreation.
- Ограничения pre-stop metrics A87, WEB backend lifecycle и остальных managers
  сохраняются. Normal temporary SQLite/fake owned children/loopback tests;
  отдельный -race A90, real upstream, live PostgreSQL, host firewall/systemctl/
  ACME, Docker/NET_ADMIN и real Unix IPC не запускались.

Итог A90: go build -p 1 ./... PASS, go vet -p 1 ./... PASS;
golangci-lint 2.15.0 full ./... PASS, 0 issues. Первая lint iteration нашла QF1001
только в новом test condition; после упрощения focused 18-case removal suite снова
PASS 4.380s и полные build/vet/lint повторно PASS. Inventory/diff check PASS:
reviewed 259, pending 2105, stale 0. Изменения не затрагивают чужие incoming commits.


## Контрольная точка A91: периодические bytes принадлежат runtime owner

Нужно 3 managers; сделано 3: Mieru, MTProto, external VPN.
Всего 91 подтверждённое исправление. Reviewed 260/2364, осталось 2104, stale 0.
Полностью прочитан исходный manager_mutation_test.go: stats envelope, bearer,
user counters и unreachable endpoint; targeted loopback run PASS 0.022s.

Причина: periodic jobs заново разрешали tag/email в IDs через
CaptureSidecarTrafficOwners. Если traffic row переименовали, а email присвоили
новой строке при неизменившемся runtime, его delta начислялась новому клиенту.
При появлении runtime/row после owner snapshot новая identity отсутствовала в
карте. A87 pinned Owner уже защищал lifecycle handoff, но не обычный periodic poll.

A91:
- Отдельные CollectTrafficPersisted используют тот же parser и epoch/process
  checks, но сохраняют samples через Owner работающего процесса. Байты принимаются
  в shared immutable retry queue/WAL до commit нового cursor/baseline. Возвращённые
  deltas уже приняты accounting и не должны повторно передаваться в commit(bytes).
- Mieru клонирует tentative cursor map; MTProto использует existing next snapshot;
  external клонирует counters и откатывает при отказе принять batch. Missing pinned
  owner сообщает явную ошибку, не продвигая periodic owned counters. Legacy checked
  collectors остаются самостоятельными read/consume API без этого side effect.
- External копирует owner pointers под manager mutex, затем опрашивает процессы
  под statsMu. Mutable manager accounting field не читается без manager lock.
- Periodic Mieru/external jobs больше не создают name-based owner map и не
  пересобирают returned deltas для повторного начисления. После owned poll идёт
  receipt replay с quota maintenance. Source locks и prefix recovery сохраняются.
- Native MTProto аналогично пишет pinned batches. Owner.routed исключает двойной
  inbound total через Xray bridge. WEB backend сохраняет отдельный старый путь;
  name-based owner map в MtprotoJob относится только к его WEB deltas.
- Sample.Active + WithActivity сохраняют zero-byte connection activity по pinned
  ClientTraffic ID, включая existing zero sample без дубликатов. Это позволяет
  delayed-start maintenance работать без переназначения события по свежему email.
- Fsync failure после process acceptance возвращает явную ошибку, но sample остаётся
  в shared pending queue; следующий job сначала восстанавливает его, прежде чем
  читать новые counters. Partial successful samples сохраняются, ошибки остальных
  sources остаются видимыми.

Доказательства:
- До правки все 3 настоящих periodic job fixtures FAIL, web 0.590s: old owner 0/0,
  replacement 100/200 для Mieru/TrustTunnel и replacement 0/100 для MTProto.
  Initial owner был захвачен runtime до переименования row; job запускался после
  повторного назначения email новой row. Production code на baseline не менялся.
- После правки selected run PASS: web 17.818s, sidecartraffic 0.343s, job 0.376s,
  service 1.654s, Mieru 0.008s, MTProto 0.008s, external VPN 0.416s. Включены A88–A90,
  lifecycle/epoch/shutdown/replay/rollback и обычные service regressions.
- Расширенный periodic run: 7 cases PASS, web 1.134s. Три normal owner reassignment,
  три filesystem fault во время фактического scrape с recovery replacement job,
  плюс routed MTProto. Проверены client и inbound totals, old vs replacement IDs;
  при disk fault DB totals остаются нулевыми до recovery, затем ровно одна delta.
- Full sidecartraffic tests PASS 0.389s, включая новый pinned zero-activity WAL test:
  после row rename/replacement receipt с 0/0 всё ещё содержит original ID.
- go build -p 1 ./... PASS; go vet -p 1 ./... PASS; golangci-lint 2.15.0 full ./...
  PASS, 0 issues. Inventory/diff check PASS: reviewed 260, pending 2104, stale 0.

Ограничения / следующая очередь:
- WEB backend MTProto не переведён на native managed Owner contract; его отдельный
  collector/owner snapshot требует собственного анализа. Name-based last-online
  UI updates также не стали identity-based в A91; byte counters и zero-byte
  accounting maintenance теперь используют pinned IDs.
- Semantic no-op Ensure после удаления/создания ClientTraffic с теми же email и
  credentials может сохранять прежний pinned Owner. Следующий приоритет — граница
  смены identity без restart и сохранение delta до корректного owner rebind.
- Capture должен видеть committed traffic rows при запуске; API не достраивает
  отсутствующие на startup identities по свежему email во время poll.
- Периодический WAL теперь создаётся на source instance с изменёнными samples/
  activity, а не одним общим name-based batch на весь job. Это повышает fsync/receipt
  нагрузку при большом количестве inbounds; отдельный performance benchmark открыт.
  Fsync выполняется под соответствующим manager/stats lock, как lifecycle handoff.
- Process retry queue не переживает crash до fsync. Прежние ограничения pre-stop
  source metrics, ACK/drain и остальных managers остаются. Normal temporary SQLite,
  owned fake children и loopback HTTP; отдельный -race A91, real upstream, host
  firewall/systemctl/ACME, Docker/NET_ADMIN, Unix IPC и PostgreSQL не запускались.
