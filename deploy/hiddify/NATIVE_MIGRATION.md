# Перенос Hiddify overlay-патчей в исходники ядер

**Срез состояния: 2026-11-09.** Репозитории: [hiddify-sing-box](https://github.com/SawaMEN/hiddify-sing-box), [hiddify-core](https://github.com/SawaMEN/hiddify-core), [3x-ui](https://github.com/SawaMEN/3x-ui).

> **Не переключать production на незавершённые ветки.** Сейчас `3x-ui/main` собирает Hiddify с 16 overlay-патчами из `deploy/hiddify`. Ядра в рабочих PR ещё не совместимы с этим сборщиком: повторное применение `clash-user.patch` вызывает ошибку `patch does not apply`. Перенос выполняется атомарно, без потери функций.

## Счётчик работ

- **Всего:** 16 групп patch-файлов.
- **Сделано на уровне исходников в черновой ветке:** 9/16 реализовано либо заменено нативно; это **не** означает успешный релиз.
- **Осталось:** 7/16 групп, включая частично реализованные, плюс интеграционные проверки.
- **Слияния рабочих веток в `main`:** 0/3 по состоянию этого среза; все три PR открыты и помечены draft.

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

1. [sing-box #23](https://github.com/SawaMEN/hiddify-sing-box/pull/23), `refactor/native-panel-overlays-20261108`: 46 коммитов, 63 файла, кандидат переноса. **Не сливать до закрытия остатка функциональности и CI.**
2. [sing-box #22](https://github.com/SawaMEN/hiddify-sing-box/pull/22), `fix/vless-handshake-close-once-20261105`: 2 коммита. Пересекается с изменениями `protocol/vless/outbound.go` в #23; перед интеграцией разрешить пересечение, сохранить тест `h_handshake_close_once_test.go` либо заменить его эквивалентным. Не сливать обе версии исправления вслепую.
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
- [ ] **7. Слияние.** При зелёных gate последовательно: sing-box → core → 3x-ui. Перепроверить статусы и SHA перед каждым merge, затем закрыть либо слить #22 без дублирования логики. Удалять устаревшие ветки только после подтверждённого включения их содержимого.

### Критерий готовности

**16/16 функциональных эквивалентов + полностью зелёные CI/E2E/сборка панели + проверенный откат.** До этого production в `3x-ui/main` продолжает использовать зафиксированные версии и patch overlays.
