# Этап 9: API, авторизация, безопасность и multi-node

**COMPLETE — этап 9 закрыт: 5/5 групп. Подтверждено 9/14 этапов и 45/70 групп; осталось 5 этапов и 25 групп.**

Ветка `update`, draft PR #279; `main` не изменяется. Диапазон полного аудита: `v3.9.8@416a66a3` → исходный `main@4aa74411`. На момент закрытия этапа 9: 9/14 этапов и 45/70 групп. Остаток: 5 этапов и 25 групп.

## Пять групп и критерии приёмки

| Группа | Область | Обязательное подтверждение |
| --- | --- | --- |
| 9.1 | API/аутентификация/авторизация | Матрица bearer-прав (monitor, node-sync, node-admin, admin), запрет административных операций через менее привилегированные токены; spoofed proxy mTLS headers не дают доступ |
| 9.2 | mTLS и доверие узлов | Живой TLS handshake: browser без клиентского сертификата работает, чужой клиентский сертификат отвергается, CA доверен; mTLS/pin поверх HTTP или неизвестный режим в runtime fail-closed, неверный client-CA PEM не сохраняется |
| 9.3 | Защита от SSRF через egress | Внешние запросы узлов не следуют HTTP redirect с чувствительными заголовками; прямой и SOCKS transport блокируют внутренние IP на момент подключения; HTTP CONNECT egress допускает internal destinations лишь с явным `AllowPrivateAddress` |
| 9.4 | Изоляция multi-node | Per-node token/config/cache/inbound-ID изолированы при совпадающих tags; изменение одного узла не инвалидирует другой, внешние callback не вызываются под manager lock |
| 9.5 | Ошибки, гонки, статический анализ | `-race` повторяется для чувствительных веток, Go vet не находит новых ошибок; отрицательные сценарии действительно FAIL/denied, mandatory test roots не SKIP |

## Конкретные исправления и оптимизации

- `runtime.HTTPClientForNode`: runtime отвергает недопустимые режимы TLS и `pin`/`mtls` по HTTP даже если конфигурация пришла из старой БД без UI-валидации; все node HTTP clients запрещают автоматический redirect. Один общий redirect callback вместо повторения проверок на каждом API endpoint.
- `NodeService.normalize`: запрет сохранения certificate pin over HTTP выравнивает правила создания/редактирования с runtime.
- `parseCertificateBundlePEM`: доверенные CA должны иметь `IsCA` и `KeyUsageCertSign`; неверная замена не изменяет ранее сохранённый trust. Проверка общего PEM-бандла единая для записи и загрузки.
- `netsafe.SSRFGuardedDialWith`: общий resolved-IP guard переиспользуется direct и SOCKS routes, чтобы SOCKS proxy не резолвил непроверенный hostname после разрешения. На SOCKS node transport отключается `ProxyFromEnvironment`, чтобы HTTP_PROXY не заменял выбранный egress. Отсутствующая стратегия не заменяется прямым выходом.
- Node HTTP client cache identity включает `AllowPrivateAddress`, чтобы смена SSRF политики создавала новый transport pool вместо использования старого.
- Нативные тесты запрета redirect с bearer, proxy SSRF, credential rotation, per-node isolation и scope escalation добавлены без искусственных SKIP.

## Проверка

Workflow `.github/workflows/stage9-security-audit.yml` включает пять обязательных job: `api-auth`, `node-mtls`, `egress-ssrf`, `multi-node`, `failures-and-vet`. Каждый gate требует именованные Go test roots через `tools/ci/require-go-tests.py`; отсутствующий/упавший/пропущенный тест не считается PASS. Параллельно работают существующие CI/CodeQL/release проверки.

**Финальный CI и проверенный SHA:** [workflow 42185497412](https://github.com/SawaMEN/3x-ui/actions/runs/42185497412), **5/5 jobs PASS**, conclusion **success** на коде `e5e8a3ed7df70b54effc57d6085e2e917702fa04`.

- `api-auth` — PASS, матрица bearer scopes и spoofed proxy-certificate headers.
- `node-mtls` — PASS, действительный TLS listener/client-cert handshake, CA trust, cert rotation, reject of insecure trust modes.
- `egress-ssrf` — PASS, private-address/dial guard, SOCKS5/SOCKS5h и redirect credential retention checks.
- `multi-node` — PASS, node-bound mutable caches, token confidentiality, cloned GUID and foreign-client isolation, reentrant resolver.
- `failures-and-vet` — PASS, тройные запуски race-sensitive roots и go vet.

Все именованные mandatory roots выполнены без FAIL/SKIP; JSONL и диагностические артефакты приложены к соответствующим GitHub Actions jobs.

## Границы

- Это регрессия безопасности панели и администрируемых узлов, а не внешний пентест чужой инфраструктуры.
- HTTP CONNECT proxy намеренно отклоняется для public-only узлов, поскольку удалённое разрешение имени у HTTP proxy невозможно закрепить за проверенным IP. Явно разрешённые private destinations остаются ответственностью администратора.
- Полный browser/mobile E2E относится к этапу 10, clean-host install к этапу 11, 24h soak/нагрузка к этапу 12, а окончательное независимое подтверждение всех этапов к этапу 14. Эти проверки не следует объявлять выполненными по проходу этапа 9.
