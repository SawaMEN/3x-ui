# Документация 3X-UI

В каталоге `docs/` находятся инструкции и исходники сайта документации на Next.js/Fumadocs.

## Локальная разработка

Требуются Node.js (версия из [`.nvmrc`](../.nvmrc)) и pnpm.

```bash
cd docs
pnpm install --frozen-lockfile
pnpm dev
```

Проверка сборки: `pnpm typecheck && pnpm lint && pnpm test && pnpm build`.

Начните с [README проекта](../README.md) и [руководства разработчика](../CONTRIBUTING.md). В `content/docs/` находятся пользовательские инструкции на нескольких языках.

Лицензия: [GPL-3.0](../LICENSE).
