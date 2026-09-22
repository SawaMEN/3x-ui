<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./media/3x-ui-dark.png">
    <img alt="3x-ui" src="./media/3x-ui-light.png">
  </picture>
</p>

# 3X-UI

**3X-UI** — веб-панель управления Xray-core и Sing-box с веб-интерфейсом для управления inbound'ами, клиентами, подписками, маршрутизацией, статистикой и настройками сервера.

Интерфейс по умолчанию использует русский язык; английский также доступен. UI построен на Ant Design и адаптирован для desktop и mobile.

> [!IMPORTANT]
> Проект предназначен для личного использования и тестирования. Перед эксплуатацией на реальном сервере проверьте конфигурацию, сетевые правила, доступ к панели, TLS и остальные параметры безопасности.

## Быстрый старт

### Установка

Установочный скрипт находится в этом репозитории:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh)
```

По умолчанию устанавливается релиз из **SawaMEN/3x-ui**.

Для установки конкретной версии:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) v3.9.2
```

Для rolling dev-сборки:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) dev-latest
```

Установщик поддерживает Linux-архитектуры **amd64** и **arm64/aarch64**.

После установки управление панелью доступно через:

```bash
x-ui
```

Основные команды:

```text
x-ui              # меню управления
x-ui start        # запуск
x-ui stop         # остановка
x-ui restart      # перезапуск
x-ui status       # статус
x-ui settings     # текущие настройки
x-ui enable       # включить автозапуск
x-ui disable      # отключить автозапуск
x-ui log          # логи панели
x-ui banlog       # логи Fail2ban
x-ui update       # обновление
x-ui install      # установка
x-ui uninstall    # удаление
```

Результат установки сохраняется в защищённом файле:

```text
/etc/x-ui/install-result.env
```

### Обновление

Обновление панели:

```bash
x-ui update
```

Установщик сохраняет пользовательские файлы в `bin/`, которые не входят в новый релиз, и не перезаписывает существующую конфигурацию Telemt.

## Благодарности

Этот репозиторий является форком основного проекта [https://github.com/MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui).

Отдельная благодарность авторам и участникам следующих проектов:

- [telemt/telemt](https://github.com/telemt/telemt) — за **Telemt**.
- [WINGS-N/3x-ui](https://github.com/WINGS-N/3x-ui) — за **vk-turn-proxy**.
- [Liafanx/MTProxyL](https://github.com/Liafanx/MTProxyL) — за **MTProxyL**.
- [Mekotofeuka/MTPROTO_FIX_By_MEKO](https://github.com/Mekotofeuka/MTPROTO_FIX_By_MEKO) — за фикс **MTPROTO**.
- Основному проекту [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) — за исходную кодовую базу и развитие проекта.


## Возможности

- Управление `Xray-core` и `Sing-box`.
- Создание и управление inbound'ами и клиентами.
- Лимиты трафика, срок действия, IP-лимиты и статистика.
- Поддержка подписок и REST API.
- Управление узлами и просмотр системной статистики.
- SQLite по умолчанию и PostgreSQL в качестве альтернативной БД.
- Интеграции с Telegram, Discord и Fail2ban.
- MTProto через встроенный `mtg` и отдельный **Telemt**.
- TUIC и AmneziaWG.
- Управление версиями Xray.
- Темы: светлая, тёмная, ultra-dark, colorful и blue-gray.
- Русский интерфейс по умолчанию, английский — дополнительный язык.

## Поддержка ядер

### Xray-core

Xray запускается панелью как отдельный процесс. Панель управляет его конфигурацией, состоянием, статистикой, логами и inbound'ами.

### Sing-box

Sing-box также запускается как отдельный процесс. Панель использует его native API для статистики соединений и управления активными подключениями.

Периодический сбор traffic statistics оптимизирован для больших наборов соединений:

- состояние трафика отдельных соединений не хранится в памяти панели между опросами;
- для traffic polling используется облегчённый protobuf-декодер;
- в traffic polling не декодируются поля соединения, которые не нужны для статистики;
- события закрытых соединений не удерживаются панелью после обработки.

При этом **RSS процесса Sing-box и RSS панели — разные величины**. Сам Sing-box может потреблять дополнительную память в зависимости от количества соединений, DNS-кэша, конфигурации маршрутизации и других факторов.


## Базы данных

По умолчанию используется **SQLite**.

Для PostgreSQL:

```bash
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

Для небольших VDS SQLite имеет уменьшенные значения cache/mmap и количества idle/open connections. Эти параметры можно переопределить через:

```bash
XUI_DB_CACHE_MB=16
XUI_DB_MMAP_MB=64
```

## Язык интерфейса

Русский (`ru-RU`) — основной язык интерфейса.

Английский (`en-US`) — дополнительный язык.

Если язык браузера не поддерживается, интерфейс использует русский.

## Лицензия

Проект распространяется по лицензии **GPL-3.0**.
