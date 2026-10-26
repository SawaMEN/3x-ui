# Этап 7: подписки и удалённые узлы

Изменения выполнены только в `update`. Источник ядра —
`SawaMEN/hiddify-core@b5749002266b9782803669138494ef6937eb4e95`,
подмодуль `SawaMEN/hiddify-sing-box@8a82b70836022a1b56eca15df12f56f903cfb27d`.
Закреплённая сборка `sawamen-b5749002266b-panel4` и её overlays не менялись.

## Подписки

Генератор Hiddify JSON использует адаптер собственного ядра, включая XHTTP,
VLESS encryption и FinalMask. Раньше он вызывал строгий адаптер обычного
sing-box: поддерживаемый собственным ядром XHTTP делал весь native JSON
недоступным. Обычный sing-box сохраняет строгую проверку совместимости.
Выбор формата зависит от клиента, а не от локального ядра панели.

Hiddify получает один объект с outbounds/endpoints; обычные sing-box профили,
URI, Xray JSON и Clash сохраняют прежние маршруты и форматы. Запрос Hiddify
к прежнему `/sub/<subId>?format=sing-box` проверен через HTTP контроллер.
Нативный JSON сохраняет UUID, пароль, адрес удалённого узла и XHTTP path.
Повторный экспорт не изменяет ClientRecord, SubID или импортированный HiddifySubURI.
Существующие проверки импортированных путей, доменов, старых подписок и
смешанных протоколов выполнены в полном наборе `internal/sub`.

Одинаковые имена внешних ссылок больше не отклоняют весь Hiddify документ.
Копии повторяющихся tags получают стабильные суффиксы, не занимая заранее
существующие имена. Исходные ссылки не переписываются. Hidden ShadowTLS
transport tags остаются уникальными и привязанными к своему прокси.

## Контракт узлов

`/panel/api/server/status` передаёт `core.type`, `running`, `version`, `state`,
`errorMsg` и `capabilities`. Объекты `xray`, `singbox`, `hiddify` описывают
соответствующие процессы независимо. Hiddify больше не дублируется в singbox.
Outbound delay probe проверяет процесс выбранного native ядра.

Heartbeat сохраняет `coreVersion`, `coreState`, `coreError`, `coreCapabilities`
в Node и возвращает их в probe, NodeView и summaries удалённых потомков.
Транзитивные узлы сохраняют эти поля. SQLite JSON serializer и map updates
проверены записью и повторным чтением. Неудачный probe сохраняет ранее известные
метаданные. Успешный ответ старой панели без capabilities оставляет их неизвестными
(null), а не назначает по типу ядра; при переходе на такую панель старые capabilities
очищаются. Старый alias Hiddify в singbox игнорируется.

Интерфейс распознаёт hiddify-core, показывает версию выбранного ядра, его ошибку
или несовпадение с фактически запущенным процессом. MASQUE доступен при выборе
native удалённого узла и исключён для Xray в соответствии с backend.

## Проверки и приёмка

- Настоящий HTTP API с node-sync токеном и адаптером Runtime.Remote: master
  отправляет VLESS+XHTTP, узел сохраняет UUID/пароль/SubID/transport и ставит
  один apply. Два чтения статистики возвращают одинаковые накопительные
  байты. Применение ядра записывается тестовым callback; процесс не запускается.
- Status этого же HTTP узла возвращает capabilities Hiddify. Heartbeat, БД,
  UI projection и дерево потомков проверены отдельно, включая старые панели.
- Сгенерированную Hiddify подписку проверяет настоящая закреплённая сборка
  командой `check -c`. Тесты адаптера собственного ядра также проходят её проверку.
- Полные Go-наборы `internal/sub`, `internal/web/controller`,
  `internal/web/runtime`, `internal/database/...`, `internal/core`,
  `internal/hiddify`, `internal/singbox`; тематические service тесты Hiddify,
  heartbeat и дерева узлов. Новые сценарии и дерево проверены с `-race`.
- Frontend: пять тестов Hiddify, typecheck, lint изменённых файлов и production
  build. Go: vet затронутых пакетов и сборка панели. `git diff --check`.
- Source/build-lock audit собственного checkout с `--upstream`: пройден.

Программная часть этапа проверена. Полная эксплуатационная приёмка — обновление
подписок установленными клиентами и фактический VPN TCP/UDP между запущенными
узлами — остаётся открытой. В этом окружении настоящий `srun` ранее отклонён
с `create netlink socket: operation not permitted`; режим `check` не заменяет
runtime тест. Открытая гарантия этапа 6 о последнем неопросенном трафике при
остановке/аварии ядра также сохраняется: этот этап не добавляет durable journal.
