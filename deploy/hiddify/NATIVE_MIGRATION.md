# Перенос Hiddify overlay-патчей в исходники ядер

**Текущий порядок действий:** [CORE_RELEASE_PIPELINE.md](CORE_RELEASE_PIPELINE.md) — sing-box/main сливается и исправляется первым; после полного зелёного CI закрепляется проверенный SHA в core; только после успешной сборки и релиза 3x-ui забирает готовые бинарники без пересборки.

**Срез состояния: 2026-11-09.** Репозитории: [hiddify-sing-box](https://github.com/SawaMEN/hiddify-sing-box), [hiddify-core](https://github.com/SawaMEN/hiddify-core), [3x-ui](https://github.com/SawaMEN/3x-ui).

> **Не переключать production на незавершённые ветки.** Сейчас `3x-ui/main` собирает Hiddify с 16 overlay-патчами из `deploy/hiddify`. Ядра в рабочих PR ещё не совместимы с этим сборщиком: повторное применение `clash-user.patch` вызывает ошибку `patch does not apply`. Перенос выполняется атомарно, без потери функций.

## Счётчик работ

- **Всего:** 16 групп patch-файлов.
- **Сделано на уровне исходников в черновой ветке:** 9/16 реализовано либо заменено нативно; это **не** означает успешный релиз.
- **Осталось:** 7/16 групп, включая частично реализованные, плюс интеграционные проверки.
- **Слияния и консолидация:** sing-box PR #23 уже слит в `main` (`411bf2c`), пересекающийся PR #22 закрыт как включённый в #23, core PR #20 пока в работе; production 3x-ui не переключён.

## Выполнено в текущем продолжении (без переключения production)

- [x] Исправлена межтестовая гонка staged monitoring в sing-box PR #23: ожидание завершения предыдущей пробы перед новой. Коммит [b09bfaf](https://github.com/SawaMEN/hiddify-sing-box/commit/b09bfaf80ae161f1545dd9af65190dad8945edea).
- [x] Отформатированы изменения VLESS для исправления CI PR #22. Коммиты [7d56509](https://github.com/SawaMEN/hiddify-sing-box/commit/7d56509257a3ca497716e3b8b7ebd6711434aec8) и [791b026](https://github.com/SawaMEN/hiddify-sing-box/commit/791b0267fb41b223ab54119d71cb566864b7e0fc).
- [x] Перенесена защита от повторного закрытия VLESS-сокета с TCP/UDP/packet тестами из PR #22 в рабочую ветку PR #23: [c496406](https://github.com/SawaMEN/hiddify-sing-box/commit/c4964069d8816d84d865cf20ea34c238ba7c9e74), [bcb876d](https://github.com/SawaMEN/hiddify-sing-box/commit/bcb876d82236498c439e1e2c358931d34eeae519).
- [ ] **Требуется подтверждение CI** актуального `hiddify-sing-box/main` перед обновлением gitlink в `hiddify-core`.
- [ ] Рабочие ветки и core/main по-прежнему требуют всех проверок и переноса 7 остальных патчей.

## Дополнительные исправления от 2026-11-09

- [x] Исправлен нестабильный интеграционный тест FinalMask: TCP может объединять фрагменты при чтении; точный размер записываемых фрагментов проверяется отдельно в `finalmask.TestFragmentSplitsFirstPacket`. Применено к обоим sing-box PR: [#22](https://github.com/SawaMEN/hiddify-sing-box/pull/22), [#23](https://github.com/SawaMEN/hiddify-sing-box/pull/23).
- [x] Для XHTTP исправлен безопасный приём nil-ответа `WaitReadCloser.Set(nil)`, добавлены тесты одновременного `Set/Close` и освобождения сокета; [sing-box PR #23](https://github.com/SawaMEN/hiddify-sing-box/pull/23).
- [x] В **изолированном** [core PR #20](https://github.com/SawaMEN/hiddify-core/pull/20) добавлен проверочный режим для восьми полностью нативных overlay-групп: они не применяются повторно, вместо этого проверяется наличие нативного исходника/регрессии. Семь незавершённых групп, security verifier, release/CI gates оставлены обязательными.
- [ ] Дождаться итогов новых CI на последних HEAD (а не переносить результаты старых коммитов).
- [ ] Подтвердить оставшиеся семь групп и зависимости, затем довести `hiddify-sing-box/main` до зелёного CI, только после этого обновить pin в `hiddify-core`.

## Итог текущего прогона интеграции

- **Core parser/integration: PASS** на кандидате PR #20 (`dc0129e`). Нативный синтаксис и парсер проходят, но это не означает работоспособный релиз.
- **Полная сборка: FAIL (fail-closed)** на `rebase-panel-protection.py`. После устранения повторного применения восьми завершённых патчей обнаружено несовместимое наложение protection-кода в шести файлах: `common/tls/reality_client.go`, `common/tls/reality_server.go`, `option/tls.go`, `option/vless.go`, `protocol/vless/outbound.go`, `protocol/vless/xui_preconnect.go`.
- **Требуется исправить содержимое, а не обходить проверку:** завершить серверную ML-DSA, Vision testseed и нативные зависимости, затем заменить `protection-options.patch` целиком и подтвердить сравнительными тестами. Текущий production builder 3x-ui/main не изменён.
- **Внешнее условие:** создать управляемые форки `SawaMEN/sing-vmess`, `SawaMEN/utls-lx`, `SawaMEN/xray-core` или принять поддерживаемый vendoring. Подробности в [sing-box issue #24](https://github.com/SawaMEN/hiddify-sing-box/issues/24).

## Реестр 16 патчей

| Патч | Статус | Что нужно |
| --- | --- | --- |
| `clash-user.patch` | Нативно в кандидате | Проверить данные пользователя в Clash API при запуске панели |
| `sniff-override.patch` | Нативно в кандидате | Проверить поведение с исходными правилами |
| `http1-options.patch` | Нативно в кандидате | Проверить кодирование HTTP/1 options |
| `hysteria-safety.patch` | Нативно в кандидате | Регрессии Hysteria/Hysteria2 |
| `inbound-finalmask.patch` | Нативно в кандидате | Проверить inbound TCP FinalMask |
| `stream-handshake.patch` | Нативно в кандидате | Проверить Trojan/HTTPUpgrade framing |
| `explicit-cipher-suites.patch` | Нативно/эквивалент в кандидате | Проверить поддержку legacy cipher suites |
| `vless-handshake.patch` | Нативно в кандидате | Проверить закрытие соединения ровно один раз, TCP/UDP/packet и отмену |
| `runtime-dependencies.patch` | Зависимости уже представлены в графе core-кандидата | Удалять только при обновлении сборщика и сохранении security pins |
| `transport-balancer.patch` | Частично | Полный Xray-совместимый balancer/selector, XHTTP, observatory, настройки leastLoad и E2E |
| `vless-reverse.patch` | Частично | Настоящий VLESS Reverse portal/bridge, lifecycle и E2E |
| `protection-options.patch` | Частично | Полная схема и поведение защитных опций REALITY/Vision; fail-closed для неподдержанного |
| `sing-vmess-protection.patch` | Не перенесён полностью | Vision testseed, padding, безопасное unwrap и тесты |
| `sing-vmess-reverse.patch` | Не перенесён | Reverse wire framing и callbacks в sing-vmess |
| `utls-protection.patch` | Не перенесён полностью | Серверная ML-DSA-65 подпись REALITY; клиентская проверка есть в кандидате |
| `xray-runtime-compat.patch` | Частично | Нативный Xray/observer parity, остатки mKCP и race/lifetime тестов |

Источник статуса — [sing-box PR #23](https://github.com/SawaMEN/hiddify-sing-box/pull/23); некоторые части в черновой ветке пока не прошли сквозные тесты.

## Открытые ветки/PR и зависимости

1. [sing-box #23](https://github.com/SawaMEN/hiddify-sing-box/pull/23), `refactor/native-panel-overlays-20261108`: **слит** в `sing-box/main`, merge commit `411bf2c`; до полной готовности 16/16 и зелёного CI это только рабочая база, не production.
2. [sing-box #22](https://github.com/SawaMEN/hiddify-sing-box/pull/22), `fix/vless-handshake-close-once-20261105`: **закрыт как поглощённый PR #23**, сохранены `handshakeOnceConn` и тест `h_handshake_close_once_test.go`. Повторный merge не нужен.
3. [core #20](https://github.com/SawaMEN/hiddify-core/pull/20), `integration/native-panel-overlays-20261109`: 7 коммитов, тестовое закрепление sing-box ревизии `9ae53f5` (старее последнего кандидата `340d941`). **Не сливать в core/main, пока release builder применяет конфликтующие патчи.**

## Зафиксированные проверки и проблемы

- sing-box #23, commit `340d941`: **Test PASS, Lint PASS**, **Upstream compatibility FAIL**. В race-регрессиях `TestH_MonitoringStagedTestShowsTheMissThenTheAnswer` провалился из-за временного порога (`5.286761ms` вместо ожидаемых `>400ms`); установить причину, исправить тест/реализацию, повторить race-прогон.
- sing-box #22, commit `1d3c76d`: **Test PASS, Upstream compatibility PASS, Lint FAIL** — форматирование `gci` в `protocol/vless/outbound.go` и `protocol/vless/h_handshake_close_once_test.go`.
- core #20, commit `ba2a557`: **Core and parser integration PASS**, **olcRTC compatibility PASS**, **CI FAIL** — arm64/amd64 release сборка останавливается на `clash-user.patch` (нативное изменение уже присутствует).

## План завершения (очерёдность обязательна)

- [ ] **1. Зависимости.** Создать доступные для записи форки нужных `sing-vmess`, `uTLS`, `Xray-core` (или отдельное решение по vendoring), сохранить лицензии, закрепить точные ревизии. [Блокер sing-box #24](https://github.com/SawaMEN/hiddify-sing-box/issues/24).
- [ ] **2. Перенос 7 оставшихся групп.** Довести Reverse portal/bridge, Vision testseed, серверную ML-DSA-65, Xray balancer/observer, XHTTP/mKCP и protection options до полного соответствия старым overlay.
- [ ] **3. CI sing-box.** Исправить race-регрессию, перепроверить VLESS socket ownership, выполнить linux/macOS/windows проверки по нужным архитектурам, тесты отказов, race и E2E.
- [ ] **4. Подготовить core.** Обновить submodule pin на *проверенный* HEAD sing-box (не на старый `9ae53f5`); проверить парсер, profile import, ABI и security/toolchain pins.
- [ ] **5. Атомарно переключить 3x-ui builder.** В `tools/build-hiddify-core.sh` одновременно обновить lock `deploy/hiddify/hiddify-core-target.json`, убрать только те `deploy/hiddify/*.patch`, функционал которых перенесён, и переместить нужные `.go.in` проверки в исходники. Не удалять самостоятельный `deploy/speed/masque.patch` без отдельной миграции лимитера скорости.
- [ ] **6. Полный release gate.** Реальная сборка amd64/arm64, `check`, запуск/остановка/перезапуск, обновление и откат, TCP/UDP, VLESS Reverse, REALITY+ML-DSA, Vision, XHTTP, mKCP, TUIC, balancer, регрессии счётчиков пользователей, интеграция 3x-ui, кросс-платформенные тесты.
- [ ] **7. Последовательный выпуск.** Sing-box/main уже консолидирован и проходит исправления. После 16/16 и зелёного CI зафиксировать SHA в core; после зелёного core CI и готовых бинарников зафиксировать release tag в 3x-ui. Использовать существующий загрузчик и не пересобирать ядро в CI панели. Ветку `backup/pre-native-main-20261109` сохранять до проверки отката.

### Критерий готовности

**16/16 функциональных эквивалентов + полностью зелёные CI/E2E/сборка панели + проверенный откат.** До этого production в `3x-ui/main` продолжает использовать зафиксированные версии и patch overlays.
