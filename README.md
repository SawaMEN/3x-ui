# 3X-UI

Панель управления VPN-подключениями, клиентами, подписками, маршрутизацией и статистикой.
Используется модифицированное ядро [SawaMEN/hiddify-core](https://github.com/SawaMEN/hiddify-core).

## Установка на Linux

Поддерживаются amd64 и arm64. Установка через системную службу:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh)
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

Проект основан на [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui).
Лицензия: [GPL-3.0](LICENSE).
