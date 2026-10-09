# Интеграция hiddify-core: этап 2

Ветка `update` поддерживает выбранное ядро `hiddify-core` v5.0.0. Существующие
настройки выбора ядра сохраняются. Автоматическая миграция и удаление Xray/sing-box
относятся к следующему этапу после проверки трафика и оставшихся ограничений.

## Подключённые функции

- Отдельные бинарник `bin/hiddify-core-linux-amd64` (либо `linux-arm64`), конфиг
  `hiddify-core.json` и журнал `hiddify-core.log`. `XUI_BIN_FOLDER` поддерживается.
- Запуск через `srun -c`; кандидат проверяется через `check -c` до замены файла и
  остановки старого ядра. Переключение ядра использует существующий механизм
  отката настроек, совместимости клиентов, Gateway и процесса.
- Сохранение выбора в настройках, запуск при старте панели, watchdog,
  отложенный перезапуск после изменения клиентов/инбаундов, узлы и подписки.
- Общий генератор DNS, маршрутизации, sidecar bridges и Gateway с адаптерами
  hiddify для XHTTP, VLESS encryption/decryption и outbound FinalMask.
- Статистика пользователей и инбаундов через V2Ray gRPC на loopback:10086.
  Один потребитель сбрасывает снимок; неуспешная запись БД сохраняется до повтора.
  Outbound-статистика не суммируется повторно с пользовательской.
- Онлайн-пользователи, IP и отключение сеансов через Clash на loopback:10090.
  Патч `deploy/hiddify/clash-user.patch` добавляет в JSON имя аутентифицированного
  пользователя. Без этого upstream v5 не предоставляет идентичность для лимитов IP.
- Выбор ядра и состояние на главной странице/в настройках. Endpoint
  `/panel/api/setting/hiddify/status`; общий `/panel/api/server/status` содержит
  выбранное ядро и объект `hiddify`. Поле `singbox` сохраняется как совместимый
  канал состояния native core для старого протокола heartbeat.
- Release workflow и Dockerfile собирают закреплённое ядро и включает его в архив панели.
  Обновление ядра выполняется вместе с панелью; установка stock sing-box под видом
  hiddify и произвольное переключение версии не предлагаются.

## Сборка и применение

```bash
bash tools/build-hiddify-core.sh /path/to/panel/bin
```

Затем в настройках выбрать `hiddify-core` и сохранить. Stock library v5 не
подходит: нужны overlay `check`, `with_v2ray_api` и пользовательский Clash JSON.
Runtime проверяет версию и обязательные build tags, включая `with_xui_panel`.
Go для ядра закреплён отдельно от Go панели на 1.26.3.

Native cores используют общий native JSON template в настройках, но отдельные
конфиги процессов. При переходе sing-box ↔ hiddify Gateway template не переносится
сам в себя. Gateway OS networking остаётся под управлением панели.

## Проверки и оставшиеся ограничения

Панель и frontend собраны; Docker image и ARM64 runtime в этой среде не проверены. Целевые тесты покрывают конфиг из БД с пользователем,
XHTTP и Gateway, независимость перезапусков, счётчики, Clash presence/auth и
отключение только нужного IP. Проверки настоящим бинарником охватывают encrypted
VLESS/XHTTP, Snell6, MASQUE/TLS, Hysteria2 gecko, FinalMask и XHTTP download.

Полный серверный набор тестов не выполнялся: автоматическая проверка заблокировала
внешний запрос теста к api4.ipify.org. Вместо него выполнены целевые тесты без
запроса внешнего IP. Constructor/check не подтверждает VPN handshake или передачу
трафика. E2E на Linux с TUN/netlink, TCP/UDP, REALITY, Gateway и клиентами ещё нужен.

Ограничения матрицы этапа 1 остаются: ML-DSA/seed, mKCP, новые Xray session ID
опции, часть fallback/sockopt/TLS. Значимые неподдерживаемые настройки должны
отклоняться preflight. Peer metering MASQUE/WireGuard/AWG требует проверки трафика;
сохранены существующие специализированные sidecars. Naive outbound требует
подходящую libcronet на сервере. Полное HotReload пока заменено проверенным restart.
