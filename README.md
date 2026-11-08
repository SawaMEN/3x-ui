# 3X-UI

Веб-панель для управления VPN-подключениями, клиентами, подписками, маршрутизацией и статистикой. Использует модифицированный **sing-box** из [SawaMEN/hiddify-core](https://github.com/SawaMEN/hiddify-core). Интерфейс на русском и английском языках.

## Установка

Linux **amd64 / arm64**:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh)
```

Установка DEV-сборки:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) dev-latest
```

## Управление

```bash
x-ui             # меню управления
x-ui status      # состояние
x-ui restart     # перезапуск
x-ui update      # обновление
x-ui log         # журналы
x-ui uninstall   # удаление
```

После установки настройте TLS и ограничьте доступ к панели.

## Возможности

- Подключения и клиенты: VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria, TUIC, NaiveProxy, AmneziaWG и другие протоколы.
- Лимиты трафика и скорости, подписки, ссылки и QR-коды.
- Маршрутизация, балансировка, узлы, статистика и резервное копирование.
- Дополнительные ядра Xray и sing-box, устанавливаемые через менеджер компонентов.
- SQLite или PostgreSQL, интеграции с Telegram и Fail2ban.

## Документация и разработка

- [Документация](docs/README.md)
- [Интеграция ядра](docs/hiddify-core-integration.md)
- [Руководство разработчика](CONTRIBUTING.md)

Основано на [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui).
Лицензия: [GPL-3.0](LICENSE).
