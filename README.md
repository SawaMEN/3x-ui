# 3X-UI

<p align="center">
  <img src="./media/3x-ui-cyberpunk.svg" alt="3X-UI" width="460">
</p>

Панель управления VPN-подключениями, клиентами, подписками, маршрутизацией и статистикой.

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

Особая благодарность авторам и участникам проектов, на которых основана панель и её ключевые возможности:

- [MHSanaei / 3x-ui](https://github.com/MHSanaei/3x-ui) — исходная кодовая база панели.
- [Hiddify / hiddify-core](https://github.com/hiddify/hiddify-core) и [hiddify-sing-box](https://github.com/hiddify/hiddify-sing-box) — основа модифицированного ядра.
- [XTLS / RealiTLScanner](https://github.com/XTLS/RealiTLScanner) — идея поиска подходящих SNI по соседним IP и проверки TLS-сертификатов.
- [XTLS / Xray-core](https://github.com/XTLS/Xray-core) — ядро Xray, совместимость и решения для сетевого стека.
- [SagerNet / sing-box](https://github.com/SagerNet/sing-box) — сетевое ядро и реализации протоколов.
- [telemt / telemt](https://github.com/telemt/telemt) — сервер MTProto.
- [Mekotofeuka / MTPROTO_FIX_By_MEKO](https://github.com/Mekotofeuka/MTPROTO_FIX_By_MEKO) — идеи защиты и исправления MTProto.
- [WINGS-N / 3x-ui](https://github.com/WINGS-N/3x-ui) — идеи и решения для интеграции VK TURN-прокси.
- [Liafanx / MTProxyL](https://github.com/Liafanx/MTProxyL) — HTML-шаблоны сайтов-приманок.

Дополнительная благодарность авторам сторонних протоколов, интеграций и компонентов, используемых проектом:

[WINGS-N / vk-turn-proxy](https://github.com/WINGS-N/vk-turn-proxy), [AmneziaWG](https://github.com/amnezia-vpn/amneziawg-go), [NaïveProxy](https://github.com/klzgrad/naiveproxy), [Mieru](https://github.com/enfein/mieru), [Sudoku](https://github.com/SUDOKU-ASCII/sudoku), [Hysteria](https://github.com/HyNetworks/hysteria), [PingTunnel](https://github.com/esrrhs/pingtunnel), [TrustTunnel](https://github.com/TrustTunnel/TrustTunnel), [DNSTT](https://github.com/NNdroid/dnstt), [OxiDNS](https://github.com/svenshi/oxidns), [TUIC](https://github.com/EAimTY/tuic), [OpenFlux](https://github.com/p1neappleXpress/OpenFlux), [FPTN](https://github.com/fptn-project/fptn).

Часть проектов послужила источником идей или кода, другие интегрируются как отдельные компоненты. Лицензии и уведомления об авторстве сторонних материалов сохранены.

Лицензия панели: [GPL-3.0](LICENSE).
