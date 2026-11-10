# 3X-UI

<p align="center">
  <img src="./media/3x-ui-cyberpunk.svg" alt="3X-UI" width="460">
</p>

Панель управления VPN-подключениями, клиентами, подписками, маршрутизацией и статистикой.

## Установка на Linux

Поддерживаются **amd64** и **arm64** (Ubuntu, Debian и другие Linux с systemd; Alpine с OpenRC). Для установки нужны права **root**, `bash`, `curl` и доступ к GitHub.

### 1. Автоматический выбор версии

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh)
```

Установщик выбирает последний совместимый стабильный релиз, но **может автоматически перейти на DEV**, если Stable пока не поддерживает новую миграцию ядра.

### 2. Версия для разработки (DEV)

Установка последней опубликованной предварительной сборки `dev-latest`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) dev-latest
```

### 3. Определённая стабильная версия (Stable)

Укажите тег нужного релиза из [GitHub Releases](https://github.com/SawaMEN/3x-ui/releases). Например, `v3.9.8`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/SawaMEN/3x-ui/main/install.sh) v3.9.8
```

Старые стабильные версии могут быть несовместимы с новым ядром.

## Обновление

```bash
x-ui update                         # последний совместимый релиз (возможно DEV)
x-ui update-dev                     # последняя DEV-сборка
XUI_UPDATE_TAG=v3.9.8 x-ui update   # конкретный Stable-релиз
```

Перед переключением версий сделайте резервную копию. Откат на несовместимую старую версию с новым ядром блокируется.

## Управление

```bash
x-ui             # меню управления
x-ui start       # запуск
x-ui stop        # остановка
x-ui restart     # перезапуск
x-ui status      # состояние
x-ui settings    # настройки
x-ui enable      # автозапуск включить
x-ui disable     # автозапуск отключить
x-ui log         # журналы
x-ui update      # обновление
x-ui update-dev  # обновление DEV
x-ui uninstall   # удаление
```

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
