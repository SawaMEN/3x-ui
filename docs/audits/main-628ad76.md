# Аудит коммита `628ad76` (ветка `up`)

Дата: 2026-11-08. Исходная ветка: `main`, HEAD `628ad76e52675c435e8cea0dc22fe4a27079b881`; родитель `416a66a3343d446b9b18a2a8b2b0eb8771ea72fa`.

## Границы проверки

GitHub REST API просмотрен постранично, `page=1..22` (100 файлов на страницу, последняя — 4). Всего в объединённом коммите 2104 изменённых файла, около 253738 добавленных и 47261 удалённой строки. Произведены классификация изменённых путей и просмотр доступных diff. Это обзор охвата, **не доказательство индивидуального построчного ревью каждого файла**.

Детально проверены важные пути из `internal/web/service/`, `internal/web/controller/`, `internal/web/runtime/`, `internal/web/session/`, `internal/web/middleware/`, `internal/database/`, `internal/sub/`, `internal/singbox/`, `internal/sudoku/` и `.github/workflows/`. В отдельных случаях код сопоставлен с родителем коммита для определения новых и существовавших дефектов.

## Подтверждённые дефекты и исправления

1. **HIGH — отсутствовала проверка выбранного случайного порта в менеджере подписок (новая функция).** `internal/web/service/manager_worker.go`, `clone`: первичная проверка выполнялась для порта 0, после чего реально назначался случайный порт без повторной проверки ОС. Занятый другой программой порт мог быть сохранён и вызвать ошибку запуска ядра. Порт теперь проверяется после выбора до создания inbound, локально через `managerPreflightPorts`, на удалённой ноде через `preflightTarget`. Добавлен детерминированный тест `TestManagerRejectsRandomPortAlreadyOwnedBySystem` в `manager_random_port_test.go`.

2. **HIGH — доверие поддельным forwarded-заголовкам подписок (уже существовало до `628ad76`).** `internal/sub/forwarded_trust.go` доверял `X-Forwarded-Host` и `X-Forwarded-Proto` от произвольного источника, если список прокси отсутствовал или совпадал с заводским значением. В результате абсолютные ссылки могли указывать на навязанный клиентом хост. Теперь CIDR источника проверяется всегда, при пустом значении используется заводской список `127.0.0.1/32,::1/128`; ошибка чтения настроек или panic приводит к отказу в доверии. Тест `TestResolveRequest_ForwardedHeaderTrust` включает внешний и loopback-адреса. Для удалённого обратного прокси администратор должен добавить адрес в `trustedProxyCIDRs`.

3. **MEDIUM — неверный адрес сервера при передаче `X-Real-IP` (существовало до `628ad76`).** `internal/sub/service.go`, `ResolveRequest`: IP самого посетителя принимался за адрес узла подписок, когда отсутствовал `X-Forwarded-Host`. Теперь используется `Host` запроса. Добавлен `TestResolveRequest_TrustedProxyRealIPDoesNotReplaceHost`.

4. **MEDIUM — неограниченный рост буфера строк логов Sudoku.** `internal/sudoku/process.go`, `logWriter.Write`: процесс, выдающий строку без `\n`, мог наращивать буфер бесконечно. Добавлено ограничение 64 KiB и линейный поиск разделителя; тест `TestLogWriterBoundsIncompleteLines`.

5. **MEDIUM — гонка интеграционного теста Sudoku.** `internal/sudoku/traffic_integration_test.go`: после запуска клиента попытка подключения иногда опережала открытие SOCKS-порта, давая `connection refused`. Добавлен ограниченный повтор при `ECONNREFUSED`. Проверка Sidecar egress проходила на `617af8ad`, до последующих изменений.

## Миграции БД

Подтверждены определения и порядок вызовов в `internal/database/db.go`, `migrations.go`, `seeders.go`: `repairOverflowedTrafficCounters`, `migrateLegacySocksInboundsToMixed`, `migrateShadowsocksRemovedCiphers`, `migrateVmessRemovedSecurities`, `seedWireguardPeersToClients`, `seedMtprotoSecretsToClients`, `stripMtprotoInboundSecrets`. Проверка присутствия функций **не подтверждает** сохранность всех данных при обновлении/откате конкретной базы.

## CI и статический анализ

- `.github/workflows/ci.yml`: в `up` общий Go unit suite перенесён до загрузки Hiddify-core, чтобы ошибки GitHub Releases (наблюдался HTTP 504) не отменяли тесты кода.
- `.github/workflows/sidecar-egress.yml`: активирован на `up`.
- `.github/workflows/codeql.yml`: активирован на `up` для Go, JS/TS и Actions.
- Ранее наблюдались успешные `golangci`, `govulncheck`, `codegen`, `postgres-durable-first`, `shell-tests` и Sidecar E2E. Результат относится к указанным в GitHub Actions SHA; успешность предыдущего SHA нельзя автоматически переносить на обновлённый HEAD.

## Оставшиеся критерии допуска к слиянию

- Зелёный полный `go test ./...`, включая тесты по исправленным функциям на последнем HEAD `up`.
- Зелёные frontend typecheck, lint, unit tests, build и CodeQL на последнем SHA.
- Повторные native integration runs с загруженными и проверенными официальными бинарниками; HTTP 504 не означает успешную проверку.
- Upgrade/rollback на сохранённых копиях SQLite и PostgreSQL с историческими записями, в том числе MTProto, WireGuard, трафиком и маршрутами.
- Отдельный тест большой нагрузки (тысячи клиентов, многонодовая синхронизация, гонки и длительная работа sidecar). Одних unit-тестов недостаточно для доказательства стабильности всех протоколов.

**Итог:** зафиксированы несколько конкретных дефектов и их исправления; обзор изменённых файлов проведён, но ни все 2104 файла, ни все реальные сетевые и миграционные сценарии нельзя считать исчерпывающе проверенными до выполнения критериев выше. Не сливать `up` в `main`, пока последняя версия не получит подтверждённый результат проверок.
