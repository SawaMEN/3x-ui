# Этап 9: миграция установленных панелей

Ветка: `update`. Целевое ядро: модифицированный **Sing-box**,
собранный из `SawaMEN/hiddify-core` по текущему
[hiddify-core-target.json](../../hiddify-core-target.json).
Старый тип `hiddify-core` остаётся входным алиасом; результат — `sing-box`.
Этап 10 (удаление старых интеграций и смена поведения новых установок) сюда не входит.

## Реализованный переход

1. Подготовленный release проверяет закреплённый бинарник до остановки панели
   и удаления файлов. Установщики сериализуются через flock.
2. После остановки создаётся приватная копия всей установки, пользовательских
   бинарников/ключей, CLI и service units. Offline CLI сохраняет БД и управляемые
   конфигурации до InitDB, изменения схемы и преобразования старых клиентов.
3. SQLite сохраняется вместе с WAL/SHM/journal. PostgreSQL требует совместимые
   pg_dump и pg_restore: custom dump снимается до изменений, восстановление
   выполняется с single-transaction/exit-on-error; добавленные миграцией таблицы
   удаляются перед восстановлением старой схемы.
4. Manifest связывает snapshot с конкретной БД без пароля, фиксирует revision,
   состояние и SHA-256. Файлы копируются потоково с fsync/атомарной публикацией;
   каталог имеет режим 0700, секретные копии — 0600. До восстановления
   проверяются все хеши и список разрешённых путей.
5. Shared lease исключает одновременную работу панели и CLI. Проверка /proc
   отклоняет оставшиеся процессы панели и ядра с управляемой конфигурацией.
   Журнал прерванной миграции блокирует запуск панели до восстановления.
6. Активные несовместимые inbound отклоняют миграцию без отключения пользователей.
   Native generator сохраняет клиентские идентификаторы и создаёт конфигурацию;
   закреплённое ядро выполняет check и пробный server start.
7. Только после успешного старта и остановки trial транзакция БД выбирает
   sing-box. Gateway interception при переходе приостановлен; желаемое
   состояние сохраняется, сеть возвращается после готовности listener панели.
8. Установщики дополнительно проверяют наличие управляемого owner process
   после запуска сервиса. Ошибки приводят к восстановлению БД, конфигураций,
   предыдущей установки и units. Исходное состояние автозапуска сохраняется;
   ранее остановленная панель при откате не запускается.
9. Snapshot не удаляется после успеха. Повторный запуск с committed snapshot
   допускается только при совпадении текущего migration marker и типа ядра.
   Если trial не удаётся остановить или snapshot повреждён, автоматический
   откат прекращается и панель остаётся остановленной.

## Ручное использование

Для ручной миграции уже установлен новый panel binary с modified Sing-box. Каталог
snapshot должен быть новым, постоянным и находиться вне заменяемой установки.
При нестандартных путях передаются те же XUI_DB_FOLDER, XUI_DB_DSN,
XUI_MAIN_FOLDER, XUI_BIN_FOLDER и параметры node-token keyring, что у сервиса.

```sh
systemctl stop x-ui
/usr/local/x-ui/x-ui core-migrate --backup-dir /var/backups/x-ui-stage9-001
systemctl start x-ui
/usr/local/x-ui/x-ui core-migrate --verify-running
```

Для разделения backup и apply используются `--backup-only --backup-dir DIR`,
затем `--backup-dir DIR`. Проверка release binary: `--check-binary`.
CLI не заменяет установку самостоятельно; полный rollback файлов обеспечивает
core-migration.sh в install.sh/update.sh.

Восстановление требует остановить панель и оставшийся trial process:

```sh
systemctl stop x-ui
/usr/local/x-ui/x-ui core-migrate --restore --backup-dir /var/backups/x-ui-stage9-001
# При обновлении вручную вернуть предыдущий panel binary/units из своей копии.
systemctl start x-ui
```

Автоматические установщики выводят путь полного recovery snapshot.
При отказе автоматического восстановления его и staged release нужно сохранить.
Не запускать панель поверх незавершённого журнала. Snapshot содержит секреты:
доступ предоставляется только администратору.

## Проверки и границы приёмки

Проверены сохранение users, UUID/паролей, subscription IDs/URL, clients,
client-inbound links, node tokens, API hashes, traffic counters и checkpoints;
включая старый inline JSON формат клиентов. CoreType и migration marker —
ожидаемые изменения settings. Не меняются ключи узлов.

Go-тесты покрывают успешный переход, идемпотентность, явное восстановление,
неудачный check/start/stop, несовместимые inbound, отсутствующую БД, повреждённую
копию, чужой pending snapshot, lease collision, interrupted journal, Gateway
desired state и блокировку запуска панели. Отдельные тесты команд PostgreSQL
проверяют restore flags и отсутствие credentials в arguments.
Тест реального закреплённого бинарника Sing-box выполняет check -c.
Пробный supervision проверен shell fixture с настоящим дочерним процессом.

Семь shell-тестов проверяют успешное обновление, откат при ошибке apply и
после отсутствия ядра, отказ preflight, сохранение disabled autostart,
откат ранее остановленной панели и wiring обоих установщиков.
Пройдены полные затронутые Go-пакеты и тематические service-регрессии,
race-проверки, go vet, сборка панели, bash -n и git diff --check.

**Эксплуатационная приёмка остаётся открытой.** Настоящий srun в окружении
не может создать netlink socket (operation not permitted). Нужны привилегированный
Linux host, фактическое обновление старой панели, TCP/UDP/DNS/Gateway traffic
и reboot. Live PostgreSQL backup/restore не выполнен: здесь отсутствуют сервер
и клиентские утилиты; тесты команд с fixtures не заменяют этот прогон.
Сохранены уже записанные счётчики, ограничение последних неопросенных байтов
при аварии ядра из этапа 6 остаётся. Docker image rollback не автоматизирован
этими host install/update scripts.

## Исправления повторной проверки

Применение prepared snapshot теперь проверяет свежесть БД; изменившиеся SQLite
или PostgreSQL требуют нового backup directory. Проверка оставшихся процессов
учитывает inline config flags и symlinks. Failed service stop прерывает rollback
до изменений, archive preflight PostgreSQL соблюдает deadline.
Runtime использует пути после загрузки EnvironmentFile.
Подробности: [повторная проверка этапов 1–9](hiddify-core-stages-1-9-review.md).
