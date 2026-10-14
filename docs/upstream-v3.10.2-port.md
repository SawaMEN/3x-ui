# Адаптация MHSanaei/3x-ui v3.10.2

Источник: https://github.com/MHSanaei/3x-ui/releases/tag/v3.10.2
Upstream: `608ef4e4`. База форка: `21bfb5c9` (адаптация v3.10.0 уже в main).
Сравнены все 14 коммитов после v3.10.0, включая изменения релиза.

## Перенесено и адаптировано

| Область | Источник | Результат |
| --- | --- | --- |
| Синхронизация трафика узлов | `f8b72de9` | Пропуск неизменившихся client_traffics и node_client_traffics; сохранены freeze, tombstones, квоты, продления и модель coremodel.ClientTraffic форка |
| Общие клиенты | `f8b72de9` | Flow определяется привязками, peer-поля общих WireGuard/AmneziaWG клиентов принадлежат туннелю с наименьшим ID; повторные snapshots больше не переключают поля между inbounds |
| INCY/Happ | `61885c91` | INCY-строка выдаётся только INCY; заголовок Happ не перебивает профиль INCY; общий JSON-профиль остаётся fallback и используется JSON/Clash-подписками |
| Hysteria2 Gecko | `cb4f2ec9` | Clash/Mihomo получают obfs=gecko и целочисленные границы размера пакетов, обычный Salamander сохранён |
| VMess mKCP/finalmask | `ff3f42ee` | Seed из path поддержан frontend/backend импортерами; явный seed форка имеет приоритет; fm сохраняется, диапазоны MTU/TTI проверяются существующим кодом |
| VMess XHTTP | `6bd1b867` | Проверено сохранение inline-параметров padding/session/uplink/XMUX; основная поддержка уже была в форке, backend дополнен legacy sessionPlacement/sessionKey алиасами |

Не добавлены Xray-специфичные проверки MASQUE-имён и ResetWeekday, которых нет
в нативной модели форка. Существующая TUIC-валидация сохранена.

## Изменения, не применимые к архитектуре форка

| Источник | Причина |
| --- | --- |
| `a66b360c` Shadowsocks hot apply | Использует Xray HandlerService. Форк управляет Hiddify-core целой конфигурацией; обещать обновление без обрыва соединений через отсутствующий API нельзя |
| `760f428c` отчёты Xray panic | Xray-процесс не используется; нативный processOutput уже собирает последовательные части stdout/stderr в защищённый mutex буфер 64 KiB и сохраняет их в ошибке завершения |
| `b177b98c` лимит handshakes MTProto | Параметр mtg-multi 1.16 не является параметром Telemt, который используется форком; несовместимый TOML не добавлен |
| `319e6932`, `6f1dd0d7`, `35b5e098` спонсоры | SponsorCard, SponsorsPage и загрузчик спонсорских данных отсутствуют в форке |
| `db8b01b8`, `c45ffcda` Windows | Release workflow форка собирает Linux-пакеты с нативным ядром; Windows executable/OpenSSL MSI не входят в него |
| `608ef4e4` версия | Собственная версия форка не заменяется upstream-номером |

## Проверки

- Регрессионные тесты INCY/Happ, Gecko, VMess XHTTP/mKCP/finalmask, приоритета seed.
- Тест количества записей узлов для VLESS с/без flow, WireGuard+VLESS и двух туннелей.
- Сохранение flow после последней отвязки; локальные peer-параметры разных туннелей.
- Проверки типов frontend, lint и production build.
