# Этап 10: frontend, мобильный браузер и WebSocket

**IN PROGRESS — специализированный CI ещё не завершён.** До финального подтверждения закрыты 9/14 этапов и 45/70 групп. Все исправления отправляются в ветку `update`, PR #279; `main` не изменяется.

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

## Подтверждения

[Специализированный workflow stage10-frontend-audit.yml](https://github.com/SawaMEN/3x-ui/actions/runs/42252436063) выполняет пять независимых CI jobs. На момент составления предварительного отчёта `mobile-browser`, `websocket-client` и `websocket-server` имеют PASS. `frontend-quality` отклонил форматирование восьми файлов (lint/deprecation уже PASS); корректирующий прогон ожидается. `browser-component-suite` ещё выполняется.

## Границы проверки

Мобильные сценарии проверяются в Chromium с заданными размерами и сенсорным вводом, а не на физических Android/iOS устройствах. HTTP 401 имитируется в браузере; это проверка реакции frontend, а не длительного TTL серверной сессии. Отдельные CI failure вне этапа 10 не следует маскировать специализированным PASS. Full clean-host installations и independent release acceptance относятся к этапам 11 и 14.
