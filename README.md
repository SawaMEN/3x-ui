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

## Благодарности

Спасибо авторам и участникам проектов:

- [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) — исходная кодовая база.
- [telemt/telemt](https://github.com/telemt/telemt) — Telemt.
- [WINGS-N/3x-ui](https://github.com/WINGS-N/3x-ui) — vk-turn-proxy.
- [Liafanx/MTProxyL](https://github.com/Liafanx/MTProxyL) — MTProxyL.
- [Mekotofeuka/MTPROTO_FIX_By_MEKO](https://github.com/Mekotofeuka/MTPROTO_FIX_By_MEKO) — исправления MTProto.

Лицензия: [GPL-3.0](LICENSE).
