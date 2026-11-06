# Этап 13 — аудит исторического DEV→main и совместимости

**Статус: COMPLETE — этап 13 закрыт, 5/5 групп.** Специализированная матрица прошла на кодовом SHA `dac0064a5ee20cb659d09723a1895fc9d36423bb`. Подтверждено **13/14 этапов и 65/70 групп**. Остался **1 этап и 5 групп**. Это не является финальным разрешением на выпуск.

## Неизменяемые контрольные точки

- `DEV` на момент исходного аудита: `8b59a974c23ae4c7fb9072d46fa18abb5390761a`
- `main` исходного аудита: `288becb7f31e004bb1f1e37d596aa0922ef38269`
- `main` после PR #279: `50a84c75409e6619de3a3abf14ab1e0eb42f17c8`
- Исходное окно **131 коммит, 57 конечных изменённых путей**.
- Дополнительное окно **283 коммита** до объединённого `main`.
- Полное окно **414 коммитов** от исходного DEV SHA.
- Тег `dev-latest` перемещается и больше не совпадает с исходной контрольной точкой. Нельзя использовать его для воспроизводимого исторического отчёта.

## Исторические области повышенного риска

Исторические 57 путей в исходном окне сгруппированы по реальным изменениям:

- Логи: `internal/logger`, `internal/util/tail`, `internal/web/service/server_diagnostic_logs*`, `server.go`, `server_syslog_test.go`, `server_xray_accesslog_test.go`, фронтенд `LogModal*`. Риски: неверная классификация INFO/DEBUG/ERROR, обрезка stack trace, сортировка по часовым поясам, блокировки на `journalctl`, память при больших логах.
- Перезапуск и live-подписки: `internal/web/job/check_xray_running_job*`, `internal/web/websocket/hub*`, frontend `useWebSocket.ts`, `websocket-handler-freshness.test.tsx`, `chunk-load-recovery*`. Риски: потеря клиентского соединения, переполнение очереди, freeze после отказа загрузки JS chunk.
- Маршрутизация Xray и sing-box: `QuickRoutingPanel*`, `quick-routing.ts`, `inbound-bindings.ts`, `NativeRoutingBasic.tsx`, native bindings. Риски: перезапись управляемых inbound/outbound маршрутов, несовпадение UI и backend.
- TLS/ECH/sidecars: `CipherSuitesSelect`, `tls_cipher_compat`, `native_share_options`, `externalvpn/outbound.go`, `vetroff_outbound_test.go`. Риски: потеря вручную выбранных TLS параметров, ошибочный import ECH, нечестная readiness для Vetroff.
- Установка и данные: `core-migration.sh`, `tools/test-core-migration-shell.py`. Риски: потеря выбранного ядра и rollback snapshot при обновлении.
- Локализация и оформление: `ru-RU.json`, `en-US.json`, `SubUnderground`, `UndergroundBackdrop`, `PanelRouteError` и другое. Риски: некорректная локализация, недоступные recovery действия, мобильная раскладка.

## Исправленные дефекты в рамках этапа

1. `tools/audit-hiddify-migration.py` сравнивал действующие `security_dependencies` с устаревшей **дублированной** тройкой модулей и блокировал сборку при добавлении законных новых security pins. Проверка заменена на сверку **шести** обязательных модулей и минимальных безопасных версий без жёстко зафиксированных полных версий; единственный источник точных версий — `docs/hiddify-core-target.json`. Проверка также подтверждает, что `tools/build-hiddify-core.sh` читает и применяет целиком тот же набор к `go mod edit` и `go mod download`.
2. `tools/ci/test_stage13_security_dependencies.py`: действующий manifest, отсутствие модуля, заниженная версия, допустимое обновление версии, попытка удалить потребление manifest в builder.
3. Приведена к каноническому формату таблица в отчёте этапа 12; Docs CI для SHA `c6ae9b3ad591c07bbf328cd74ac7b55982561ab2` прошёл.
4. `tools/ci/stage13_commit_inventory.py` создаёт полный CSV+JSON по каждому коммиту и изменённому пути (включая изменения, которые GitHub Compare обрезает после 300), классифицирует ответственные области и **кандидатные** тесты. Карта не равнозначна ручной проверке каждого поведения.

## Пять обязательных групп

- **13.1 Immutable history:** независимый `git`-инвентарь исходных 131 коммита / 57 путей, следующих 283 коммитов и полного окна 414, с artifact CSV/JSON.
- **13.2 Kernel/security lock:** целостность runtime dependency manifest, отсутствие старых копий версий, shell syntax, негативные security-тесты.
- **13.3 Исторический backend:** `-race` регрессии logger/tail/core watchdog/TLS/Vetroff.
- **13.4 Сервисный API и WS:** `-race` диагностика / WebSocket / сохранность structured logs.
- **13.5 Frontend + generated contracts:** QuickRouting, bindings, обработка WebSocket, chunk recovery, legacy TLS и точное совпадение генерации OpenAPI/Zod с backend.

Workflow: [stage13-dev-delta-audit.yml](https://github.com/SawaMEN/3x-ui/blob/update/.github/workflows/stage13-dev-delta-audit.yml).

## Подтверждение успешной приёмки

[GitHub Actions run #42282156706](https://github.com/SawaMEN/3x-ui/actions/runs/42282156706): **conclusion success, 5/5 обязательных jobs PASS** на `dac0064a5ee20cb659d09723a1895fc9d36423bb`.

- **13.1 `immutable-history` — PASS:** полный Git-инвентарь 131/283/414 коммитов и 57 изменённых путей исходного окна, результаты CSV и JSON сохранены как artifact.
- **13.2 `kernel-security-pins` — PASS:** защитные уровни всех шести модулей, отрицательные тесты, реальные инструменты сборки.
- **13.3 `historic-backend` — PASS:** повторённые `-race` проверки logging, tail, core recovery, TLS и Vetroff.
- **13.4 `historic-service-websocket` — PASS:** регрессии диагностики, WebSocket и конкурентных событий.
- **13.5 `historical-frontend-contracts` — PASS:** typecheck, routing/websocket/import regression tests, точное совпадение Zod/OpenAPI после генерации.

**Docs CI:** на этом же SHA `42282156691` обнаружено лишь нарушение форматирования этого отчёта (отсутствующий пустой абзац перед списком). Оно исправлено последующим документационным коммитом; успешность Docs CI после исправления требуется проверить отдельно. Общий CI и release-матрица относятся к независимой финальной приёмке этапа 14.

## Ограничения

- Оригинальные 57 изменённых путей и все 131 commit subjects перечислены в проверяемом inventory; это **учёт и соответствие тестам**, не доказательство ручного behavioral review каждой строки.
- Тесты `-race` и unit/Browser smoke не заменяют 24-часовую производственную нагрузку, физические устройства и проверку реального обхода ограничений связи.
- Глобальные CI, Release и Docs CI фиксируются отдельно. Завершение специализированных 5/5 тестов не разрешает объединить ветку, если общие тесты падают.
- `main` не менялся во время этапа 13; изменения удерживаются в `update` и draft PR #296.
