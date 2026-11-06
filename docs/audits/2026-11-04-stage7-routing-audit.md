# Этап 7: маршрутизация, DNS, Gateway и firewall

**PASS — 5/5 групп этапа 7. Общий прогресс: 7/14 этапов, 35/70 групп.**

Проверенный SHA кода: `c77ce1d159287a54e41546d942c9d86f13aaf558`.

## Итоговая проверка

[Workflow 41703473824](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824) на `c77ce1d159287a54e41546d942c9d86f13aaf558`: **6/6 jobs PASS**.

| Job                                                                                                  | PASS test/subtest events | FAIL | SKIP |
| ---------------------------------------------------------------------------------------------------- | -----------------------: | ---: | ---: |
| [Contracts](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153262919), текущий код |                     1213 |    0 |   43 |
| [Native AMD64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153263290)           |                       85 |    0 |    0 |
| [Native ARM64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153263369)           |                       85 |    0 |    0 |
| [Kernel/DNS AMD64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153263456)       |                       40 |    0 |    0 |
| [Kernel/DNS ARM64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153263316)       |                       40 |    0 |    0 |
| [OxiDNS/DNSTT](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/job/126153263339)           |                       40 |    0 |    3 |

Frontend: **17 файлов, 111 тестов PASS**, typecheck PASS; vet PASS. Go выполнялся с `-race`. Числа обозначают события tests/subtests, не количество уникальных сценариев во всех jobs.

43 и 3 SKIP относятся к environment-dependent тестам общих пакетов. Все обязательные roots и их подтесты прошли без SKIP; native/kernel/DNS gates выполняются отдельно. Ожидаемые падения старого кода из before-policy/before-status не включены в числа текущей версии.

Последующие коммиты меняют только этот отчёт. Обязательные проверки этапа завершены; это не заявление о завершении этапов 8–14 или всех соседних release workflows.

### Исходные artifacts

| Artifact                                                                                                         | SHA256 архива                                                      |
| ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| [routing-native-amd64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13265705322)          | `e73e46f6b99f4dc3fc69a531cfd9626fa3df0e117a2b1f67921a4133d94196a9` |
| [routing-contract-evidence](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13265546019)     | `40174f47d6649d2fa643ffe3b9b0f0500d9cd997a7758e8610e44852145e2d48` |
| [routing-kernel-evidence-amd64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13265311607) | `1a6be85298c08fe6e0a1c3e13996eb2511108795c237ebe8fc91048697022df7` |
| [routing-kernel-evidence-arm64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13265251855) | `c2db7fd0a953b5a57c8d3f21ec558bf1b86bbd432a294a692f3475317cef080a` |
| [dns-component-evidence](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13265031743)        | `ca68142cee6e51c8e5c697e17cf64addeb6543fb4a8e02b5fc7d700ad33b635b` |
| [routing-native-arm64](https://github.com/SawaMEN/3x-ui/actions/runs/41703473824/artifacts/13264777190)          | `87d1f59590fd3a3604d407bc7cb3c66fcd2f3f82ed28b2d117b204aaee7713fc` |

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

| Группа | Проверка                                                                                                                                                                                                                                                                                                                                   |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 7.1    | Настоящие owner/upstream/Xray: TCP/UDP, маркеры A/B, первый match, AND-порт, OR-домены, inbound scope и отказ unmatched-трафика. Service contracts: rename/delete tags, routing catalog, missing/empty/invalid rules, detour cycles.                                                                                                       |
| 7.2    | Owner/upstream: selector A→B→недоступный upstream→A, TCP/UDP markers. Целевой echo предварительно доступен напрямую, поэтому direct fallback обнаруживается. Live selector API switch сохраняет существующее соединение. Service contracts: probes, stale edits, ranking/rotation timers, all-dead/recovery и blocked alias.               |
| 7.3    | A/AAAA через UDP/TCP, DoH/DoT, cache/hosts/fakeIP/NXDOMAIN, split DNS. Отдельный netns/resolv.conf прогон. Официальные checksum-pinned OxiDNS/DNSTT: реальные запросы, certificate/name checks и tunnel payload.                                                                                                                           |
| 7.4    | AMD64/ARM64 private network/mount namespaces: настоящий TUN/TPROXY TCP/UDP, counters, policy rules/marks, pause guard, boot replay, отказ без listener, add/remove, сохранение management и посторонних правил. После stop проверяются исходные policy rules/forwarding, отсутствие TUN/owned tables.                                      |
| 7.5    | Interval, active/expected/unknown status и UI, удаление/stale ID/quoting, backend/zone changes. Настоящее nft-удаление и наблюдение accept rules nft/iptables. Native adblock на Xray/upstream: generation/traffic/scope/priority/managed-server policy. Отказ загрузки списка сохраняет cache; Domain front и YouTube contracts включены. |

## Воспроизведение

Workflow `.github/workflows/routing-audit.yml` требует именованные roots через `require-go-tests.py`: отсутствие теста, FAIL или SKIP обязательного root/его подтеста не принимаются за успех.

До исправлений отдельно воспроизводятся потеря inbound scope, молчаливое удаление invalid catalog item, потеря прежнего nft firewall и direct fallback при Gateway pause. Gate принимает именно падения указанных тестов; compile error недостаточен.

Для ложного native status подставляются `firewall_managed.go` и `firewall_safe_ops.go` из `a2cdc3c4`. `TestStage7NativeStatusDoesNotInventOpenPorts` обязан упасть: старый код показывает отсутствующий 443/tcp открытым. После восстановления текущих файлов тот же сценарий проходит. Локальное независимое воспроизведение также дало `invented open port map[443:true 2222:true]`.

## Границы результата

Linux Ubuntu 24.04, Go 1.27.3, native AMD64/ARM64. Ядра закреплены в download-audit-cores.sh: owner stream1, upstream sing-box 1.14.4 и Xray 26.10.21; SHA256 входят в artifacts.

Kernel tests выполняются в private namespaces и не меняют host rules. Systemctl заменён заглушкой: проверяются generated restore script и настоящий replay, а не полная загрузка VM/systemd. Полные install/reboot matrices относятся к этапу 11.

Unknown inventory: nft inet/ip/ip6 вне owned tables; iptables filter rules IPv4/IPv6 вне owned chain/jump; UFW show-added expressions вне generated managed comment; firewalld runtime rich rules выбранной зоны. Это не универсальный редактор всех tables/chains или permanent firewalld configuration. UFW/firewalld command contracts проверяются отдельно от настоящих nft/iptables packet tests. Ошибка inspection отображается отдельно от пустого списка. Native port status проверяет owned accepts; он не доказывает достижимость через все возможные чужие firewall layers.

Известный дефект fragmented Trojan официального upstream относится к этапу 5 и не объявляется исправленным этими проверками. Этапы 8–14 не закрываются этим отчётом.
