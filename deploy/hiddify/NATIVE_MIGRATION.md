# Нативная интеграция Hiddify в 3x-ui

Состояние: 2026-11-10. Исходники функций поддерживаются непосредственно в [hiddify-sing-box](https://github.com/SawaMEN/hiddify-sing-box) и [hiddify-core](https://github.com/SawaMEN/hiddify-core). **В `3x-ui` больше не нужно накладывать патчи на ядро.**

## Схема

1. `hiddify-core/main` закрепляет конкретный SHA подмодуля `hiddify-sing-box`.
2. `tools/prepare-panel-kernel.py` в репозитории ядра формирует проверенный манифест исходников для двух Linux-архитектур.
3. `3x-ui/tools/build-hiddify-core.sh` получает конкретные commit SHA, проверяет нативные реализации и security pins, тестирует ядро, собирает бинарник и Cronet и формирует `target.json` и SHA-256.
4. Релиз `hiddify-core` публикуется только после успешных интеграционных тестов MASQUE HTTP/1, HTTP/2, HTTP/3 и ограничений скорости.
5. `3x-ui` при выпуске получает готовые Linux AMD64/ARM64 архивы и проверяет контрольные суммы, gitlink, ABI, обязательные теги и конфигурацию.

## Удалено

Исторические `deploy/hiddify/*.patch`, `deploy/hiddify/*.go.in` и `deploy/speed/masque.patch` перенесены в нативные исходники и проверки. Проверка списков `panel_build.native_source_files` и `source_checks` предотвращает тихую потерю функций.

## Не удалять

- `hiddify-core-target.json`: исходный build/runtime lock, набор обязательных тегов, security pins и контроль нативных файлов.
- `fork-contract.json`: историческая матрица протоколов и статистики, пока на неё ссылается source audit.
- `deploy/speed/mieru.patch`, `deploy/speed/sudoku.patch`: относятся к отдельным companion-серверам и ещё применяются их сборщиком.

Независимые Android-сборки не должны блокировать проверенные Linux-артефакты панели.
