# Этап 8: Gateway и дополнительные сервисы

Работа выполнена в `update`. Ядро остаётся собственным:
`SawaMEN/hiddify-core@b5749002266b9782803669138494ef6937eb4e95`,
`SawaMEN/hiddify-sing-box@8a82b70836022a1b56eca15df12f56f903cfb27d`.
Использована закреплённая сборка `sawamen-b5749002266b-panel4`.
Исходники ядра, overlays и build lock на этом этапе не изменялись.

## Gateway и жизненный цикл сети

Web Gateway выбирает Hiddify runtime по типу ядра, полученному для операции,
без повторного чтения глобального выбора внутри проверки/restart. Status
использует Sing-box для native Gateway; HTTP контракт проверен на настоящей
БД и Gin-контроллере. Существующие web/CLI операции enable/disable и native
TPROXY template сохранены.

Обычный native restart теперь снимает собственные nftables interception/NAT
таблицы перед остановкой слушателя. Кандидат проверяется до этого перехода.
После успешного запуска или восстановления предыдущего процесса interception
возвращается только при сохранённом намерении оператора и TCP/UDP readiness.
Stop снимает interception, сохраняя recovery baseline и намерение включённого
Gateway; следующий Start восстанавливает сеть. Explicit Suspend/Disable
продолжает менять намерение на выключенное, и watchdog его не отменяет.

При отменённом контексте панели cleanup выполняется с отдельным ограниченным
контекстом. Ошибки снятия правил останавливают переход; ошибки восстановления
сохраняются и удаляют частичный interception. Watchdog снимает interception
при упавшем ядре до попытки восстановления, а при работающем выбранном ядре
исправляет потерянные forwarding/rp_filter/policy route/NAT/nftables/units.

Наличие любых открытых сокетов на 52345 не считается доказательством Gateway.
В applied config работающего процесса должен быть его TPROXY listener.
После отката к конфигурации без Gateway правила не направляются в чужой процесс.

Boot restore и native runtime используют общий дополнительный flock
`gateway.env.runtime.lock`. Он отделён от web/CLI operation lock, который
может уже удерживаться во время restart. Перезапуск и загрузочная установка
правил не выполняются одновременно. Existing routing/firewall unit ordering,
проверка обоих сокетов, atomic nft transactions и исходный rp_filter/forwarding
snapshot сохранены. Boot script прогнан с инструментами-фикстурами; состояние
политик проверено на idempotence и отказах.

## DNS и маршрутизация

Сгенерированный Hiddify Gateway добавляет первый scoped `hijack-dns` rule для
TCP/UDP порта 53 на `in-tproxy`. Перехваченные запросы проходят настроенный DNS
панели; обычные inbound и DNS/routing settings сохраняются. Rule добавляется
в generated config и исчезает вместе со слушателем, не загрязняя recovery template.

TPROXY Gateway сохраняет текущий **IPv4** контракт: выбранный LAN интерфейс
и subnet, mark `0x40/0xc0`, table 100, local route, optional WAN NAT. IPv6
TPROXY не объявлен поддерживаемым; nft rules его не перехватывают.

## TUN

Новый адаптер Hiddify переводит существующие настройки TUN панели:

| Панель | Собственное ядро |
| --- | --- |
| name | interface_name |
| gateway (CIDR) | address |
| mtu | mtu |
| dns (IP) | dns_address, dns_mode=native |
| autoSystemRoutingTable | route_address; auto_route при непустом списке |
| autoOutboundsInterface=auto | route.auto_detect_interface |
| явный autoOutboundsInterface | route.default_interface |

Используется native system stack. Адреса IPv4/IPv6, целочисленный MTU и имена
интерфейсов проверяются до публикации. TUN требует хотя бы один interface CIDR.
Неотображаемые параметры, ненулевой userLevel, authentication clients или
stream protection возвращают ошибку, а не silently теряются.

Loop prevention применяется после template merge: template не может отключить
обязательное определение исходящего интерфейса или назначить другой интерфейс.
Несовместимые настройки нескольких TUN, повторяющееся имя virtual interface
и outbound bind в собственный TUN отклоняются. Совместное включение automatic
TUN routing и TPROXY Gateway отклонено: конфликт таблиц/приоритетов ещё не проверен
на реальном host. Manual TUN без auto_route не запрещён этим guard.

TUN не занимает TCP/UDP port из DB; старое значение port не мешает Gateway.
Reserved tag `in-tproxy` остаётся защищённым. В форме при выборе TUN порт
обнуляется и выключается, stream settings очищаются. TUN предлагается Hiddify
узлам; stock sing-box сохраняет строгий отказ неподдерживаемого translator.

## Дополнительные сервисы

FPTN outbound больше не требует отдельный sing-box внутри контейнера.
В него монтируется то же архитектурное executable собственного Hiddify,
запускаемое через `srun -c`. Путь вынесен в config без циклической зависимости
адаптеров; относительный XUI_BIN_FOLDER разрешается возле executable панели
так же, как раньше. OpenFlux и прочие sidecar менеджеры сохраняют свои процессы.

FPTN VLESS bridge остаётся на loopback хоста, внутри контейнера использует
bind_interface=fptn0; его DNS использует тот же vpn-out detour. Исчезнувший
FPTN tunnel не должен переключать bridge на обычный default интерфейс.
Точная bridge JSON, используемая запуском, принята настоящим modified-sing-box check.
Настоящий Docker FPTN handshake в этом окружении не запускался.

Выполнены полные пакеты externalvpn, TUIC, MTProto, AmneziaWG, Mieru, Sudoku,
а также соответствующие service/job сценарии и существующие rollback/sidecar
планы Hiddify. Это тесты менеджеров, конфигураций и процесса-фикстуры, а не
подтверждение подключения каждого внешнего сервиса к реальному VPN серверу.

## Выполненные проверки

- Modified Sing-box `check -c`: native TUN IPv4/IPv6/DNS, полный DB TUN config,
  TPROXY Gateway с DNS rule, FPTN bridge и существующие Hiddify config сценарии.
- Lifecycle fixtures: снять interception до Stop, успешный restart, откат
  failed candidate с восстановлением сети, Stop/Start, потеря firewall,
  partial restore/retry, explicit suspension и чужие ready sockets.
- Linux lease: второй процесс `flock` исключён во время перехода, отмена ожидания
  и освобождение проверены. Новые сценарии и существующие Gateway tests — `-race`.
- Полные Go-пакеты config, hiddify, gateway, externalvpn, tuic, mtproto,
  amneziawgnet, mieru, sudoku, runtime, controller, sub, singbox, database.
  Тематические service/job тесты Hiddify, native Gateway, протоколов и портов.
- Frontend: 70 тестов в трёх затронутых наборах, typecheck, lint изменённых
  файлов и production build.
- Go vet затронутых пакетов, сборка панели, git diff --check и source/build-lock
  audit checkout модифицированного Sing-box с --upstream.

## Открытая эксплуатационная приёмка

Программная часть проверена. Приёмка «Gateway включается через веб и проводит
трафик; дополнительные протоколы сохраняют работоспособность» **не закрыта**
реальным сетевым прогоном. Предыдущий настоящий srun в этом окружении получил
`create netlink socket: operation not permitted`; режим check не запускает
TUN/TPROXY, не проверяет Linux forwarding на LAN и не заменяет reboot тест.
Нужен host с необходимыми network privileges для фактического TCP/UDP/DNS,
перезагрузки и handshake внешних протоколов. Ограничение последних неопросенных
байтов при аварии ядра из этапа 6 также остаётся открытым.
