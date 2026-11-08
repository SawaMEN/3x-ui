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

Особая благодарность авторам и участникам проектов, на которых основана панель и её ключевые возможности:

- [MHSanaei / 3x-ui](https://github.com/MHSanaei/3x-ui) — исходная кодовая база панели.
- [Hiddify / hiddify-core](https://github.com/hiddify/hiddify-core) и [hiddify-sing-box](https://github.com/hiddify/hiddify-sing-box) — основа модифицированного ядра.
- [XTLS / RealiTLScanner](https://github.com/XTLS/RealiTLScanner) — идея поиска подходящих SNI по соседним IP и проверки TLS-сертификатов.
- [XTLS / Xray-core](https://github.com/XTLS/Xray-core) — ядро Xray, совместимость и решения для сетевого стека.
- [SagerNet / sing-box](https://github.com/SagerNet/sing-box) — сетевое ядро и реализации протоколов.

## Дополнительно

Благодарность авторам сторонних протоколов, интеграций и компонентов, используемых проектом:

- [telemt / telemt](https://github.com/telemt/telemt) — сервер MTProto.
- [WINGS-N / vk-turn-proxy](https://github.com/WINGS-N/vk-turn-proxy) и [WINGS-N / 3x-ui](https://github.com/WINGS-N/3x-ui) — VK TURN-прокси и решения для интеграции.
- [Liafanx / MTProxyL](https://github.com/Liafanx/MTProxyL) — HTML-шаблоны сайтов-приманок (MIT).
- [Mekotofeuka / MTPROTO_FIX_By_MEKO](https://github.com/Mekotofeuka/MTPROTO_FIX_By_MEKO) — идеи защиты и исправления MTProto.
- [Amnezia VPN / amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — реализация AmneziaWG.
- [klzgrad / naiveproxy](https://github.com/klzgrad/naiveproxy) — NaïveProxy.
- [enfein / mieru](https://github.com/enfein/mieru) — Mieru и сервер Mita.
- [SUDOKU-ASCII / sudoku](https://github.com/SUDOKU-ASCII/sudoku) — Sudoku Tunnel.
- [HyNetworks / hysteria](https://github.com/HyNetworks/hysteria) — Hysteria.
- [esrrhs / pingtunnel](https://github.com/esrrhs/pingtunnel) — PingTunnel.
- [TrustTunnel / TrustTunnel](https://github.com/TrustTunnel/TrustTunnel) — TrustTunnel.
- [NNdroid / dnstt](https://github.com/NNdroid/dnstt) — DNSTT.
- [svenshi / oxidns](https://github.com/svenshi/oxidns) — OxiDNS.
- [EAimTY / tuic](https://github.com/EAimTY/tuic) — TUIC.
- [p1neappleXpress / OpenFlux](https://github.com/p1neappleXpress/OpenFlux) — OpenFlux.
- [fptn-project / fptn](https://github.com/fptn-project/fptn) — FPTN.

Часть проектов послужила источником идей или кода, другие интегрируются как отдельные компоненты. Условия лицензий и уведомления об авторстве сторонних материалов сохраняются в соответствующих файлах.

Лицензия панели: [GPL-3.0](LICENSE).
