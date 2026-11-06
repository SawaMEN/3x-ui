# Этап 10: frontend, мобильный браузер и WebSocket

**COMPLETE — этап 10 закрыт: 5/5 групп.** Завершено 10/14 этапов, 50/70 групп; осталось 4 этапа и 20 групп. Финальная обязательная матрица [CI №42254923401](https://github.com/SawaMEN/3x-ui/actions/runs/42254923401) завершилась **5/5 jobs PASS**, общая conclusion **success**, на кодовом SHA `c1ee242053aa61a821d2e86759d3dc1a9efb87c5`. Ветка `update`, draft PR #279; `main` не изменён.

## Пять групп и критерии

| Группа | Область | Обязательный критерий |
| --- | --- | --- |
| 10.1 | Frontend quality | `npm ci`, lint, запрет deprecated API, форматирование, TypeScript typecheck, Vite production build |
| 10.2 | Мобильная адаптация | Реальный Chromium на 320×568, 375×667, 768×1024 и 1366×768: форма входа доступна, отсутствует горизонтальная прокрутка; режим reduced motion, допустима вертикальная прокрутка |
| 10.3 | Серверный WebSocket | Origin проверяет host+port и TLS; межпортовые/междоменные/неверные схемы отвергаются; hub shutdown, backpressure, register/unregister проверяются повторно под `-race` |
| 10.4 | Клиентский WebSocket | Один общий сокет; после возвращения сети/foreground переподключение немедленно; старый retry timer отменён, listeners снимаются после disconnect, обновление React Query после reconnect |
| 10.5 | Browser/component regression | Полный набор Vitest unit + React component + Storybook Chromium; подставной HTTP 401 должен вернуть браузер на вход, не выдавая локальную копию браузера за производственный сервер |

## Найденные дефекты и исправления

1. Zod `passthrough()` в xDNS schema использовал deprecated API: замена на `loose()` без изменения поведения по дополнительным полям; ранее этот lint-барьер валил frontend CI.
2. Серверная проверка WebSocket Origin сравнивала только hostname и игнорировала другой порт. Исправлено сравнение эффективного Origin port и Host port, схема HTTPS проверяется при доступном TLS; добавлены IPv6 и отрицательные тесты.
3. Мобильный WebSocket ждал экспоненциального таймера даже после восстановления подключения. `online` и `visibilitychange` запускают немедленную попытку, отменяют запланированный retry и не создают два сокета.
4. Для экрана входа добавлена поддержка `100dvh`, safe-area inset и компактных карточек; непрерывные дорогие GPU blur animations на мобильных отключены.
5. В WebSocket-хабе раньше терялась последняя мутация в серии, когда сообщение попадало под throttling. Теперь одна trailing REST invalidate на topic обеспечивает обновление без потоковой отправки лишних крупных snapshots; таймеры очищаются при Stop. Регрессионный тест `TestHub_ThrottledFinalMutationTriggersRESTInvalidation` прошёл с `-race`.
6. Восемь frontend файлов отформатированы с обязательным `oxfmt --check`; временный formatter-workflow удалён. Сгенерированные OpenAPI, TS-типы и frontend schemas обновлены из текущего Go backend (Telemt WEB availability/originResolve, speedLimit capability). Специализированный CI повторно генерирует контракты и запрещает устаревшие файлы.

## Подтверждения

**Финальный CI и проверенный код:** [stage10-frontend-audit.yml run 42254923401](https://github.com/SawaMEN/3x-ui/actions/runs/42254923401), `c1ee242053aa61a821d2e86759d3dc1a9efb87c5`. GitHub Actions завершил workflow со статусом **success**, **5/5 jobs PASS**, без пропущенных обязательных групп:

- `frontend-quality`: npm ci, lint, отсутствие deprecated API, форматирование всех frontend-файлов, актуальные Go→OpenAPI/TS контракты, TypeScript и Vite build — PASS.
- `mobile-browser`: Chromium на 320×568, 375×667, 768×1024 и 1366×768; форма/горизонтальная прокрутка, simulated 401 navigation — PASS.
- `websocket-server`: строгий Origin host/port/TLS и IPv6, bounded Hub, trailing invalidate, race-regression tests — PASS.
- `websocket-client`: offline→online, foreground resume, cleanup shared socket, React Query revalidation — PASS.
- `browser-component-suite`: полный npm test (unit, React, Storybook Chromium) — PASS.

Предварительные CI failure в formatter/codegen устранены, обновлённые файлы зафиксированы в ветке. Следующий этап — 11; успешный специализированный workflow не удостоверяет успех остальных CI и не заменяет финальный release gate.

## Границы проверки

Мобильные сценарии проверяются в Chromium с заданными размерами и сенсорным вводом, а не на физических Android/iOS устройствах. HTTP 401 имитируется в браузере; это проверка реакции frontend, а не длительного TTL серверной сессии. Отдельные CI failure вне этапа 10 не следует маскировать специализированным PASS. Full clean-host installations и independent release acceptance относятся к этапам 11 и 14.
