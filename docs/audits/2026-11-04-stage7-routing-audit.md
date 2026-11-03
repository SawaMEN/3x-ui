# Этап 7: маршрутизация, DNS, Gateway и firewall

Статус: **IN_PROGRESS** — финальная матрица ещё выполняется. Проверяемый SHA кода: `c77ce1d159287a54e41546d942c9d86f13aaf558`.

[Финальный workflow](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824). До завершения обязательных jobs этап не отмечается PASS.

Проверяемая ветка: `update`, PR #279. Исходный диапазон аудита: v3.9.8@416a66a3 → main@4aa74411. Код этапа 7 начинается после `fdeba876`; main не изменён.

## Исправления и оптимизация

1. Удаление последнего inboundTag больше не превращает scoped rule в правило для остальных inbound. Повреждённый элемент routing catalog возвращает ошибку вместо молчаливого удаления.
2. Замена nft firewall проходит одной транзакцией с предварительной проверкой. Ошибка replacement сохраняет действующие правила и управление.
3. Gateway pause/boot recovery устанавливают LAN-scoped drop guard вместо прямого forwarding. INPUT и управление остаются доступны. Recovery ждёт TCP и UDP listener.
4. У TPROXY явно указаны TCP/UDP transport context и IPv4 target. Resume использует текстовый формат route inspection; policy-rule table IDs проверяются численно, чтобы aliases не скрывали конфликты.
5. Добавлены строки **НЕИЗВЕСТНО** с выражением внешнего правила и явным удалением по ID. Сервер заново читает правила; ID меняется при изменении выражения, backend/binary/zone. Anonymous packet counters не делают выбор устаревшим. Команды передаются собственными argv, клиент не передаёт shell-команды.
6. Настраиваемый interval (5–3600 секунд, default 60) больше не обходится polling страницы: GET статуса не ставит внеочередную сверку в очередь. Первая инициализация и изменения inbound сохраняют немедленную сверку. Повторные GET не перезаписывают initialized/safety settings.
7. Native status определяет Exists по настоящему безусловному port accept, а не по включённой таблице. Отсутствующий порт, source-restricted allow, чужая chain и DROP не считаются общим разрешением. Учитываются служебные comments iptables.
8. Удалены отдельный frozen-status decorator и повторное чтение managed settings. Status/sync используют общий appendFrozenInboundRules. Новая inspection использует уже выбранный backend.

Исправлен сам сетевой стенд: UDP echo отвечает с IP назначения; готовность selector API ожидается отдельно от SOCKS listener. Операции select и payload после ошибок не повторяются. Измеренное ускорение всей панели не заявляется.

## Пять групп исходного плана

| Группа | Проверка |
| --- | --- |
| 7.1 | Настоящие owner/upstream/Xray: TCP/UDP, маркеры A/B, первый match, AND-порт, OR-домены, inbound scope и отказ unmatched-трафика. Service contracts: rename/delete tags, routing catalog, missing/empty/invalid rules, detour cycles. |
| 7.2 | Owner/upstream: selector A→B→недоступный upstream→A, TCP/UDP markers. Целевой echo предварительно доступен напрямую, поэтому direct fallback обнаруживается. Live selector API switch сохраняет существующее соединение. Service contracts: probes, stale edits, ranking/rotation timers, all-dead/recovery и blocked alias. |
| 7.3 | A/AAAA через UDP/TCP, DoH/DoT, cache/hosts/fakeIP/NXDOMAIN, split DNS. Отдельный netns/resolv.conf прогон. Официальные checksum-pinned OxiDNS/DNSTT: реальные запросы, certificate/name checks и tunnel payload. |
| 7.4 | AMD64/ARM64 private network/mount namespaces: настоящий TUN/TPROXY TCP/UDP, counters, policy rules/marks, pause guard, boot replay, отказ без listener, add/remove, сохранение management и посторонних правил. После stop проверяются исходные policy rules/forwarding, отсутствие TUN/owned tables. |
| 7.5 | Interval, active/expected/unknown status и UI, удаление/stale ID/quoting, backend/zone changes. Настоящее nft-удаление и наблюдение accept rules nft/iptables. Native adblock на Xray/upstream: generation/traffic/scope/priority/managed-server policy. Отказ загрузки списка сохраняет cache; Domain front и YouTube contracts включены. |

## Воспроизведение

Workflow `.github/workflows/routing-audit.yml` требует именованные roots через `require-go-tests.py`: отсутствие теста, FAIL или SKIP обязательного root/его подтеста не принимаются за успех.

До исправлений отдельно воспроизводятся потеря inbound scope, молчаливое удаление invalid catalog item, потеря прежнего nft firewall и direct fallback при Gateway pause. Gate принимает именно падения указанных тестов; compile error недостаточен.

Для ложного native status подставляются `firewall_managed.go` и `firewall_safe_ops.go` из `a2cdc3c4`. `TestStage7NativeStatusDoesNotInventOpenPorts` обязан упасть: старый код показывает отсутствующий 443/tcp открытым. После восстановления текущих файлов тот же сценарий проходит. Локальное независимое воспроизведение также дало `invented open port map[443:true 2222:true]`.

## Границы результата

Linux Ubuntu 24.04, Go 1.27.3, native AMD64/ARM64. Ядра закреплены в download-audit-cores.sh: owner stream1, upstream sing-box 1.14.4 и Xray 26.10.21; SHA256 входят в artifacts.

Kernel tests выполняются в private namespaces и не меняют host rules. Systemctl заменён заглушкой: проверяются generated restore script и настоящий replay, а не полная загрузка VM/systemd. Полные install/reboot matrices относятся к этапу 11.

Unknown inventory: nft inet/ip/ip6 вне owned tables; iptables filter rules IPv4/IPv6 вне owned chain/jump; UFW show-added expressions вне generated managed comment; firewalld runtime rich rules выбранной зоны. Это не универсальный редактор всех tables/chains или permanent firewalld configuration. UFW/firewalld command contracts проверяются отдельно от настоящих nft/iptables packet tests. Ошибка inspection отображается отдельно от пустого списка. Native port status проверяет owned accepts; он не доказывает достижимость через все возможные чужие firewall layers.

Известный дефект fragmented Trojan официального upstream относится к этапу 5 и не объявляется исправленным этими проверками. Этапы 8–14 не закрываются этим отчётом.
