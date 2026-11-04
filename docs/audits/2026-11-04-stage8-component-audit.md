# Этап 8: Telemt, Vetroff и внешние компоненты

**IN PROGRESS — финальный обязательный CI-прогон ещё не завершён.** Общий подтверждённый прогресс до его окончания: 7/14 этапов, 35/70 групп.

Ветка `update`, draft PR #279; `main` не изменён. Исходный диапазон аудита: v3.9.8@416a66a3 → main@4aa74411. Код этапа 8 начинается после `466c2b10`.

## Исправления

1. Reconcile внешних inbound выполняет ту же валидацию, что и Ensure. Ошибка конфигурации больше не останавливает исправный процесс.
2. Подготовка замены сохраняет прежний процесс. Ошибка запуска или немедленное завершение дочернего процесса восстанавливает прежние настройки и файлы; ошибки отката не скрываются.
3. Список managed outbound целиком проверяется до изменения первого процесса; пустые и повторяющиеся теги отвергаются до побочных действий. Предварительная подготовка FPTN больше не удаляет работающий Docker-контейнер.
4. Отмена managed Linux-команды завершает группу дочерних процессов; сохраняется parent-death signal. Обычный exec.Command без контекста не получает недопустимый Cancel.
5. Неудачный probe удаляет временный каталог с приватными файлами; успешный probe ожидает startup-проверку.
6. Vetroff принимает явный доверенный PEM CA через `caCert` и официальный `--ca-cert`; путь должен быть абсолютным. Проверка имени сервера и pin остаётся включённой.
7. Установщик FPTN распаковывает официальный CLI-пакет в отдельный staging-каталог и копирует только бинарник. Распаковка Debian-пакета поверх `/` ломала merged `/lib` и выполнение команд. Добавлена libstdc++6 и проверка запуска CLI при сборке образа.
8. Проверка официального Vetroff bundle выбирает SHA256 по фактической архитектуре вместо единственного AMD64-хеша.
9. Лимит nofile серверного FPTN-контейнера повышен с 65 536 до 262 144: HAProxy официального образа требует 250 023 и с прежним лимитом не открывал TLS endpoint. Реальный run 41850476330 воспроизвёл этот дефект.

10. FPTN inbound ждёт HAProxy и backend внутри контейнера; outbound ждёт настоящий адрес `fptn0` и внутренний listener owner core с timeout/cancel, а не открытый Docker proxy. На Docker host порт был доступен до аутентификации и запуска bridge. Обычный TLS ClientHello не используется как readiness-проба: Reality mode намеренно перенаправляет его на decoy.

Оптимизация: общий prepare helper вместо дублирования render/launch, удалена повторная валидация в Ensure, снимок настроек создаётся только при новом запуске, а не при каждой неизменившейся сверке. Клиент HTTP метрик использует ограниченный общий пул соединений; regexp ключа FPTN компилируется один раз. Измеренное ускорение всей панели не заявляется.

## Пять групп исходного плана

| Группа | Обязательная проверка |
| --- | --- |
| 8.1 Telemt | Сохранение legacy MTProto secret на клиенте при миграции; официальный Telemt: config, API stats и native reload; WEB carrier config и реальные cover pages/assets. Настоящие Caddy/HAProxy: HTTPS с проверкой имени сертификата и двунаправленный WebSocket. MEKO: production helper, реальный nft в private netns, apply/reapply/remove и сохранение посторонней таблицы на AMD64/ARM64. |
| 8.2 Vetroff | Официальный bundle, профили/опции и установка с проверкой digest. Managed inbound/outbound: настоящие TCP и UDP через H2 CONNECT, WSS и H3 CONNECT; H2/WSS auto Necko/NSS, H3 Go TLS. Неверные SNI, pin и password должны отвергаться на том же H3/Go TLS, где проверен положительный payload. Занятый порт не останавливает действующий сервер. SOCKS readiness, неизменившаяся сверка, parent death и subscription/service contracts. |
| 8.3 Протоколы | Mieru TCP/UDP и TUIC native senders в contracts; официальный Sudoku traffic/CLI. Owner Snell 5/6, TUIC, Naive и прочие режимы: реальный payload и отказ неверных credentials. MASQUE connections; официальный TrustTunnel через core SOCKS; OpenFlux 0.3.0 через managed exit/client; FPTN server 0.4.4 + CLI 0.4.6 в Docker, сертификат, access token, TCP/UDP через owner bridge и запрет прямого выхода после остановки server. |
| 8.4 OlcRTC | Pinned manager с production patch, реальные Linux netns/квоты; production installer, systemd service, loopback API, credentials/auth, prerequisite checks. AMD64/ARM64. Контракты update/remove и deployment rollback/recovery. |
| 8.5 Отказы | До исправлений отдельно воспроизводятся пять regressions: invalid reconcile, failed inbound replacement, partial invalid outbound set, surviving descendant, leaked probe files. Текущий код проходит те же проверки с -race; digest/download/interrupted install, API failure, rollback и cleanup покрыты contracts. Native Vetroff bind collision; отсутствие FPTN server/client контейнеров после stop. |

## Воспроизведение и доказательства

Workflow `.github/workflows/component-audit.yml`: contracts, native AMD64/ARM64, MEKO kernel AMD64/ARM64, OlcRTC AMD64/ARM64 и FPTN AMD64 — восемь jobs.

`require-go-tests.py` требует именованные roots и их подтесты: отсутствие, FAIL и SKIP обязательного сценария не принимаются за PASS. Ожидаемые падения старой версии из before-components.jsonl отделены от результатов исправленного кода; compile error недостаточен — должны упасть все пять конкретных тестов.

Компоненты закреплены в `docs/stage8-components.json`, downloader проверяет SHA256 до распаковки. В artifacts сохраняются JSONL и контрольные суммы бинарников; FPTN server закреплён digest `sha256:073de56151a4f4f3803bbce10e84d352d1a5195fe771f4a851691c4aebbbb415`, фиксируется ID собранного client image. Owner/Xray/upstream cores закреплены существующим download-audit-cores.sh.

Предварительный прогон [41848972032](https://github.com/SawaMEN/3x-ui/actions/runs/41848972032) подтвердил contracts, native AMD64, MEKO и OlcRTC на обеих архитектурах. FPTN installer действительно собрал образ; payload fixture остановился на слишком коротком ключе метрик. Fixture исправлен. Этот предварительный run не считается итоговым PASS.

## Границы результата

Linux Ubuntu 24.04; native AMD64/ARM64, кроме полного Docker FPTN, который проверяется на AMD64. FPTN ARM64 не объявляется проверенным. OpenFlux закреплён на используемой интеграцией версии 0.3.0; новые версии не подставляются молча. Его реальный TCP payload направляется на IPv4 не loopback: userspace exit stack этой версии не предоставляет обычный host loopback.

Telemt native tests проверяют принятие config, reload/stats и WEB serving; публичная Telegram end-to-end сессия не заявляется. Полная OS/install/reboot matrix и browser matrix относятся к этапам 11 и 10. Не все экзотические Vetroff profiles прогоняются реальным payload: полный список проверяется CLI/config contracts, TCP/UDP payload отдельно проверяет H2/WSS/H3.

Ограничения UDP/TCP-only, accounting и shaping конкретных протоколов остаются явными; PASS не означает добавления неподдерживаемых upstream возможностей. Результат не закрывает этапы 9–14 и не отменяет ранее зафиксированные ограничения fragmented Trojan/распределённого shaping.
