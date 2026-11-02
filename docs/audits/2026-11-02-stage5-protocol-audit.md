# Этап 5: протоколы, транспорты и TLS

Дата: 2026-11-02. Ветка: `update`, draft PR [#279](https://github.com/SawaMEN/3x-ui/pull/279).
Исходный диапазон: v3.9.8 `416a66a3` → main `4aa74411`.
Проверяемый SHA кода и тестов: `9b365d1b2ab098aab1743e3682dde88e497021a2`.

**Этап 5 завершён: 5/5 пунктов. Общий прогресс: 5/14 этапов, 25/70 групп.**

| Пункт | Результат проверки |
| --- | --- |
| 5.1 | Реестр моделей/форм/переводчиков, 78 core-policy rows и binary constructors; ранние отказы до сохранения |
| 5.2 | 68 classic protocol/transport pairs, SS/SOCKS/HTTP, form-switch contracts, XDrive/QUIC и конкретная outbound chain |
| 5.3 | Перечисленные ниже native расширенные протоколы и WG/AWG; внешние режимы отдельно от native, по исходной границе этапа 8 |
| 5.4 | TLS/SNI/ALPN, legacy cipher, Reality, сохранение/отказ ECH/FinalMask; актуальный XDNS формат |
| 5.5 | Нативная клиентская матрица обеих архитектур, source evidence и фиксированные версии; ограничения и сторонний дефект не замаскированы под data-pass |

Завершение этапа означает выполнение указанной матрицы аудита. Это не утверждение
об отсутствии ошибок в сторонних бинарниках или успехе всех возможных сочетаний
протоколов, клиентов и защитных расширений. Непроверенные сочетания не объявлены
совместимыми по одному лишь успешному constructor check.

## Исправления и удаление лишней работы

1. **Локальная проверка ядра обходила общую политику совместимости.** До исправления
   принимались Snell в upstream sing-box и Tunnel в обоих sing-box. Проверка теперь
   общая для local/node, с авторитетным сохранённым NodeID при обновлении. Add,
   Update и Enable отказывают до изменения строки; возможность отключить строку
   сохраняется. Неизвестный inbound-протокол больше не считается поддерживаемым
   по умолчанию. Удалены повторная проверка при Add и три дублирующих core-switch.
2. **Прямой переводчик игнорировал `enable:false`.** Панельный сборщик уже фильтровал
   отключённых клиентов; это не доказательство обхода отключения в обычном UI.
   Теперь низкоуровневые переводчики также исключают таких пользователей, включая
   ShadowTLS inner и VLESS reverse. Отсутствующий `enable` сохраняет прежний смысл.
   Полностью отключённый список proxy-пользователей не превращается в anonymous.
3. **HTTP/SOCKS/Mixed обходили пользователей дважды.** Собирается один правильный
   список с `username/password`. Объединение `clients` и `accounts` больше не
   записывает в общий backing array исходного списка. Reverse-список выделяется
   только при фактическом наличии reverse-параметров.
4. **Закреплённый owner fork терял данные HTTPUpgrade.** Сервер выбрасывал буфер
   `Hijack`, клиент читал ноль байт из уже заполненного HTTP reader. Патч
   `deploy/hiddify/stream-handshake.patch` сохраняет оба буфера. Ошибка проявилась
   при настоящем Trojan/HTTPUpgrade/IPv6 соединении и отдельно воспроизведена
   детерминированными тестами буферов.
5. **Trojan предполагал один `Read` для всего ключа.** Теперь корректный hex-префикс
   дочитывается до полной длины. Короткий HTTP-запрос по-прежнему сразу получает
   fallback. Тест с чтением по одному байту падал до патча и проходит после него.
6. **Нормализация TLS копировала всё дерево конфигурации.** Теперь копируются только
   изменяемые ветви и их предки. Неизменяемые ветви сразу сериализуются. Справочник
   Go TLS cipher suites строится один раз. Проверены вложенные TLS, общие ссылки,
   повторная сериализация и неизменность исходных объектов при ошибке.

7. **Неподдерживаемый transport сохранялся до отказа runtime.** Введён ранний
   guard для protocol/core/transport; panel QUIC не подменяется TCP. Update
   проверяет новый streamSettings с сохранённым NodeID, Enable учитывает приоритет
   `method` над `network` в импортированных строках. Проверена неизменность БД.
8. **QUIC получал несовместимый TLS-профиль.** Native QUIC требует включённый
   Go TLS 1.3; uTLS/Reality и TLS 1.2-only отвергаются до запуска. Проверены
   реальные QUIC TCP/UDP соединения и неправильное SNI.
9. **Встроенный Xray validator отставал от выбранного бинарника.** Обновлён до
   того же release commit `1b1647cd9ddf`; проверены настоящие XDrive keys и MUX
   constraints. `go mod tidy` удалил невостребованные зависимости старой версии.
10. **Форма XDNS генерировала удалённый формат списков строк.** Форма и golden
    fixture используют object domains/resolvers. Старый формат явно отвергается
    перед Add/Update/Enable; сохранённые строки и RR types не удаляются и не
    преобразуются молча. Старые resolver strings связывают domain с resolver,
    новый формат разделяет их; переход требует явной настройки обоих peers.
11. **Форма оставляла параметры неактивного транспорта.** При сохранении
    удаляются только известные неактивные transport subtrees; выбранный транспорт,
    TCP/raw aliases и неизвестные будущие поля сохраняются. Проверена неизменность
    исходного объекта. Каталог транспорта вынесен из каждого вызова; удалены
    повторяющиеся пункты Sudoku и лишние проверки совместимости.

Патч ядра включён в обычный build script вместе с четырьмя regression tests.
Версия owner: `sawamen-b310097c8e7a-338f0c9457a4-mask1-ciphers1-net62-sec1-stream1`.
Исходные parent/fork/ray2sing commits сохранены; изменены явно учтённые overlays.

## Измеренная оптимизация TLS

Один вызов нормализации 100 inbound, Go 1.27.3/linux-amd64, без `-race`, три замера;
время — медиана. Это измерение нормализации, не ускорение всей панели или TLS handshake.

| Данные           | До, ns/op | После, ns/op | До → после, B/op | До → после, allocs/op |
| ---------------- | --------: | -----------: | ---------------: | --------------------: |
| Без TLS          |    201171 |        75612 |   182775 → 23372 |            2502 → 901 |
| Явные TLS suites |    562674 |       162202 |   554256 → 99520 |           6504 → 1602 |

Выделяемая память уменьшилась примерно на 87% и 82%, соответственно.

## Проверенные бинарники и воспроизводимость

Owner собран нативно на amd64/arm64 из закреплённых исходников в
[прогоне 41421583366](https://github.com/SawaMEN/3x-ui/actions/runs/41421583366), SHA панели
`2498633e456fcc2b7af371eb5e41ea4373cc6ca3`. Там же выполнены source regressions
Trojan/HTTPUpgrade и binary-backed проверки: **1019 PASS events на архитектуру,
0 FAIL, 0 обязательных skip**. Аудит повторно использует точные owner artifacts и проверяет SHA-256, а не
пересобирает ядро на каждую правку панели. `libcronet.so` также закреплена по
commit и SHA-256: этот audit-профиль — purego. Статический musl release-профиль
отдельный; его свойства не выводятся из purego-проверок.

| Компонент                                         | amd64 SHA-256                                                      | arm64 SHA-256                                                      |
| ------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Owner stream1                                     | `bab8ca4bb3bebe2c9e2555ee49d376d7f321265ff09b54af37591a8ff4f00c37` | `5bf6d2f6bd99a01fb89dd99203980e7e0d6807bbf341bddef0c86b446912a77b` |
| Xray v26.10.21                                    | `a27e3f95ab6ce270956d0cecd32eda4686a811afc71c64a6a734a3dede14883e` | `dc3324f437132d70d29ddadea19b202949c2d1230b5d90dac469b03f68829c8b` |
| Upstream sing-box v1.14.4                         | `1264361b5f8868c8768f7555d3e77e20118bf503c41ac08bb290a0d99056aa3a` | `d953af885f11d6b547c21a3c0ffe70796963afc802fab959d548311002b4908a` |
| Cronet `0d28acc44093df24b2526dea3d6ffefd6b0a54f0` | `23109c55b08829bb1a68682a61a64852b1bbf243753b153cbd8f6e4ea2c05294` | `3c1fcfcb56261a4b318ffb6d2227b9726782edb5a894e74db2b534406db62498` |

## Матрица и реальные клиенты

`protocol-inventory.json`: 26 inbound models, 26 соответствующих frontend forms,
78 model/core policy rows, 13 outbound forms, 21 native outbound constructor.
Policy, schema и constructor не означают успешной передачи данных.
`protocol-matrix.csv` строится из завершённых Go JSONL events; пропуск, отсутствие
обязательного subtest или FAIL останавливают генерацию. У каждой строки указаны
protocol, direction/endpoint role, core, transport, client, payload, result,
source file/test, commit и architecture. Клиенты — закреплённые core binaries;
для WireGuard/AWG peer — Go amneziawg/v3 `v3.1.20260828`, для фрагментированного
Trojan — Go 1.27.3 crypto/tls и ручной Trojan frame.

| Область | Проверенные сценарии | Точная граница |
| --- | --- | --- |
| Owner VLESS/VMess/Trojan | 28 protocol/transport pairs: TCP, raw, WS, gRPC, HTTP, HTTPUpgrade, XHTTP, KCP, native QUIC; XDrive только VLESS | Native QUIC отделён от неподдерживаемого panel `network=quic` |
| Upstream VLESS/VMess/Trojan | 18 pairs: TCP/raw/WS/gRPC/HTTP/HTTPUpgrade | Нет заявленного успеха XHTTP/KCP/XDrive в upstream |
| Xray VLESS/VMess/Trojan | 22 pairs: TCP/raw/WS/gRPC/HTTPUpgrade/XHTTP/KCP; XDrive только VLESS с MUX | Удалённые HTTP/QUIC и MASQUE с чужим protocol отвергаются |
| TCP/UDP для этих 68 pairs | Оба payload, два разрешённых пользователя, disabled/unknown, reconnect | TCP: IPv4/IPv6 destination и localhost DNS; UDP: IPv4 destination. Outer UDP KCP/QUIC не подменяет proxied UDP |
| Расширенные owner protocols | SOCKS, HTTP, SS AES-128-GCM, SS2022 AES-128-GCM, Hysteria1/2, TUIC, AnyTLS, Naive, Snell5/6 | TCP IPv4/IPv6/DNS, reconnect, неверный пароль; UDP IPv4 у поддерживающих его protocols. HTTP/Naive — TCP-only. SS profiles — single PSK |
| WireGuard | Native initiator и responder: IPv4/IPv6 TCP/UDP, peer revocation | Это роли endpoint/handshake; не проверка всех gateway/TUN routing modes |
| AWG | Native outbound initiator: IPv4/IPv6 TCP/UDP, peer revocation, header protection и padding | Фиксированный listening port в native mode **unsupported**, явно отвергается. Внешний server mode — этап 8 |
| AnyTLS → ShadowTLS | TCP/UDP через реально сгенерированную subscription chain; неверный пароль каждого слоя отдельно | IPv4; не все сочетания wrapper/inner |
| MASQUE | HTTP 1/2/3 без version fallback, TCP/UDP, неверный пароль | IPv4; без утверждения об IPv6 CONNECT-IP |
| XHTTP | Owner: 11 modes none/TLS; Xray: 18 none/TLS/Reality; split download и большие HTTP bodies | Только явно перечисленные security/mode pairs |
| TLS | Trusted local cert, SNI/ALPN, legacy TLS 1.2 с явным RSA cipher, Xray Reality; ECH/FinalMask translation, preservation/rejection и constructors | ECH и каждый отдельный mask не объявляются полным data cross-product |

IPv6 в этом стенде означает loopback destination или адрес внутри WG tunnel;
DNS — `localhost`. Это не проверка публичного IPv6-интернета или DNS leak.
Фоновый owner outbound monitoring может обращаться к public IP-info endpoints;
локальные echo targets не доказывают полной сетевой изоляции процесса.

## Открытый дефект стороннего бинарника

Официальный upstream sing-box 1.14.4 отвергает фрагментированный Trojan auth key.
Тест сначала подтверждает обычную передачу, затем TLS-record fragmentation и
конкретную ошибку `bad request size`. В CSV это `KNOWN_UPSTREAM_DEFECT`,
**не** успешный fragmented transfer. Owner stream1 проходит оба сценария.
Официальный upstream binary не переписывался под прежней версией/контрольной суммой.

## Разделение с последующими этапами

- Учёт, speed/device/quota limits, все URI/JSON/YAML/Clash/Hiddify import/export
  combinations и смена состояния активных сессий — этап 6.
- DNS leak, TUN/TPROXY, selector/urltest/failover, routing loops и многозвенные
  routing chains — этап 7. Их constructor rows не являются data-pass.
- Внешние MTProto/Vetroff/Mieru/Sudoku/TUIC/TrustTunnel/FPTN/OpenFlux/VKTurnProxy/
  Pingtunnel/Naive/AWG processes и server lifecycle — этап 8, как в исходном плане.
  Native checks этого этапа не закрывают внешний режим того же названия.
- Браузерная проверка всех форм и UI взаимодействий — этап 10. Здесь выполнены
  schema/form-switch contracts, typecheck и production build.

## Подтверждения

Итоговый native run: [41474843098](https://github.com/SawaMEN/3x-ui/actions/runs/41474843098).
Оба задания завершены PASS на SHA выше. На каждую архитектуру:

- 422 строки CSV: **399 data-pass**, 21 constructor-only, один unsupported AWG
  fixed-listen mode и один KNOWN_UPSTREAM_DEFECT. Последние 23 строки не
  засчитаны как успешная передача.
- 1897 PASS test/subtest events с учётом повторов, 14 package PASS, **0 FAIL**.
- 15 opt-in SKIP в начальном unit-шаге без binary-specific env; 12 затем реально
  выполнены в обязательных шагах. Три оставшихся — MASQUE speed limits, geo routing
  и live selector, относящиеся к этапам 6–7. **Обязательных SKIP — 0**.
- MASQUE HTTP1/2/3 TCP/UDP и неверные credentials: три завершённых повторения.
- Матрица повторно построена из скачанных JSONL; совпали все поля
  строк CSV. ZIP evidence каждого job проверен по GitHub artifact SHA-256.

Общий Go suite и production frontend build:
[41474843105](https://github.com/SawaMEN/3x-ui/actions/runs/41474843105), job `125377864581` — PASS.
[Protocol integration build](https://github.com/SawaMEN/3x-ui/actions/runs/41474843054),
[runtime lifecycle](https://github.com/SawaMEN/3x-ui/actions/runs/41474842984) и
[database compatibility](https://github.com/SawaMEN/3x-ui/actions/runs/41472714384) — PASS.
БД-проверка выполнена на `ba519756`; последующий `9b365d1b` меняет только
MASQUE test, повторяемость CI и metadata поля клиента в CSV, без изменения
production кода или моделей БД.

Предыдущий run `41439907544` выявил ошибку AWG fixture: тест принудительно включал
запрещённый native fixed-listen режим. Это не засчитано как исправленный баг
приложения или успешный AWG responder. Общий Go run `41439907562` выявил устаревшую
XDNS fixture и форму. Отдельный локальный `-race` подтвердил новые XDNS save guards.
Местный Vitest не запускается из-за `uv_interface_addresses: EPERM`; результат
frontend tests берётся из native CI, а не заменяется локальным typecheck.

В run `41472714293` AMD64 завершил весь gate; ARM64 обнаружил ошибку отрицательного
MASQUE fixture: немедленный ECONNRESET после неверного пароля считался падением
теста. Теперь именно ECONNRESET допускается как отказ, а FATAL/panic и потеря
listener отдельно запрещены. Отказы другого вида не превращены в PASS.
Итоговый gate повторяет MASQUE три раза на архитектуру.
