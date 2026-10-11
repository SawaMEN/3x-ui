# Этап 10: единственное управляемое ядро

Рабочая ветка: `update`. Ядро — модифицированный **sing-box** владельца из
`SawaMEN/hiddify-core`, revision `b5749002266b9782803669138494ef6937eb4e95`,
версия `sawamen-b5749002266b-panel4`. Его исходники остаются в репозитории
владельца; панель проверяет закреплённую версию, revision и build tags.

## Изменения

- Удалены управляемый процесс Xray, его gRPC-клиент, hot-apply, счётчики
  старого API и standalone загрузчики Xray/stock sing-box.
- Основной процесс, временные процессы проверки исходящих, генерация
  конфигурации, сессии, watchdog и статистика используют owner-профиль.
  Пустой `SingBoxService{}` также означает owner-сборку: отдельного флага
  выбора stock runtime больше нет.
- Данные онлайн-клиентов и узлов вынесены в независимый `Presence`.
  Уведомления Telegram/Discord, история online и обновление геоданных
  проверяют единственный sing-box; резервирование конфигурации использует
  его фактический файл.
- Удалены переключатель ядра, страница Xray и standalone установщики из
  интерфейса. `/xray` перенаправляет на sing-box для старых закладок;
  `/outbound` и `/routing` открывают native редактор.
- Удалены HTTP endpoints самостоятельной установки/удаления ядер и старые
  Xray balancer/route-test RPC. OpenAPI синхронизирован. Legacy имена
  restart/stop endpoints сохранены как aliases единственного runtime для
  совместимости узлов и автоматизации.
- HY2 и Naive в окне обновлений отмечены как встроенные: они обновляются
  вместе с панелью. Mieru/mita, Telemt и другие sidecars сохраняются.
- X25519 и ECH генерируются без Xray executable. VLESS encryption использует
  CLI закреплённого owner core. Неподдерживаемые ML-DSA/ML-KEM Reality
  операции возвращают явную ошибку вместо обращения к удалённому ядру.
- Release workflow и DockerInit не загружают Xray; installer/updater используют
  `hiddify-core-linux-<arch>`. CLI проверяет процесс owner-сборки.

## Совместимость и миграция

Клиентские форматы подписок **Xray JSON**, **sing-box JSON**, share links и
старые поля БД не удалены. Пакет `internal/xray` содержит wire-конфигурации,
geodata и проверку legacy формата; он не запускает Xray. Зависимость
`github.com/xtls/xray-core` нужна этим форматам, protobuf/geodata и связанным
адаптерам данных. Её наличие в `go.mod` не означает наличие управляемого ядра.
`go mod tidy` выполнен.

Новая установка выбирает и сохраняет `sing-box`. Первый запуск пустой БД
разрешён. БД с явным `xray`, либо со старым неявным ядром и существующими
inbounds/template, требует **offline core-migrate** до запуска панели.
Миграция сохраняет прежнее значение неявного default как Xray, чтобы новый
default панели не изменил интерпретацию исходной конфигурации.
`hiddify-core` принимается как прежнее имя owner-профиля и сохраняется как
`sing-box`; это не второй движок.

В install/update порядок следующий: остановка, snapshot установки и БД,
preflight/trial owner core, применение, запуск панели, `--verify-running`,
восстановление Gateway, удаление рабочих legacy executables. Удаляются только исполняемые файлы с
именами `xray`, `xray-*`, `sing-box`, `sing-box-*` в управляемом bin каталоге.
Неисполняемые конфигурации, геоданные и sidecars не затрагиваются. Snapshot сохраняется. Для нестандартного
внешнего `XUI_BIN_FOLDER` бинарники резервируются отдельно и возвращаются
при откате.

Инструкции offline миграции и восстановления:
[этап 9](hiddify-core-stage-9-verification.md). Не запускайте новую панель с
legacy БД напрямую и не используйте прямой restore старой БД вместо миграции.

## Контейнеры

Новый образ собирает закреплённое ядро владельца; standalone Xray/stock
sing-box в него не добавляются. Для старого volume остановите старый контейнер,
сохраните его image/tag и данные, выполните `core-migrate --backup-dir DIR`
через `/app/x-ui` нового образа с теми же volumes, переменными XUI и сетевыми
capabilities. DIR должен быть постоянным volume вне заменяемой установки.
После запуска проверьте `core-migrate --verify-running`. При ошибке используйте
`--restore --backup-dir DIR` при остановленной панели и верните прежний образ.
Entrypoint не выполняет скрытую миграцию пользовательской БД.

## Проверка

- `go test ./... -count=1 -timeout=180s` с `HIDDIFY_CONFIG_CHECK_BINARY` на
  реальную закреплённую сборку владельца.
- Проверки generated ECH, пар X25519, единственного default runtime,
  запрета standalone обновления и сохранения migration/default identity.
- Race-проверки миграции, owner runtime, presence, native apply и сессий.
- `go vet ./...`, сборка панели; frontend typecheck, lint, build.
- Frontend unit/components: **151 файл, 1544 теста passed, 1 skipped**.
- Десять shell-сценариев `tools/test-core-migration-shell.py`: успешное
  применение, ошибки preflight/apply/verify, восстановление данных/установки,
  сохранение enabled/stopped состояния, удаление старых бинарников после
  commit и их наличие в snapshot/rollback, включая внешний bin каталог.
- `bash -n` для installer/updater, migration helper, CLI и build script.

Реальные privileged traffic/reboot, Docker build/run и live PostgreSQL restore
в этом окружении не выполнены. Также не выполнен browser/Storybook smoke:
Chromium здесь отсутствует. Результаты выше подтверждают регрессии и проверку
конфигураций, но не заменяют сетевую эксплуатационную приёмку из этапов 8–9.
