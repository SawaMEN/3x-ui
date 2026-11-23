import { z } from 'zod';
import { COVER_SITE_HTML_MAX_BYTES } from './cover-site-html';
import { isTelemtWebCatalogTemplate, TelemtWebThemes } from './telemt-web-catalog';

export const TelemtFrontendSchema = z.enum(['nginx', 'caddy', 'haproxy']);
export const TelemtCarrierSchema = z.enum(['https', 'https-lanes', 'websocket', 'websocket-lanes']);
export const TelemtWebBasePathSchema = z
  .string()
  .trim()
  .refine((value) => {
    const path = value.replace(/^\/+|\/+$/g, '');
    return (
      path === '' ||
      (path.length >= 6 &&
        path.length <= 96 &&
        /^[A-Za-z0-9][A-Za-z0-9_-]*(?:\/[A-Za-z0-9][A-Za-z0-9_-]*)*$/.test(path))
    );
  }, 'Путь 6–96 символов; каждый сегмент должен начинаться с буквы или цифры');

export const TelemtCarriersSchema = z
  .array(TelemtCarrierSchema)
  .max(4)
  .refine((values) => new Set(values).size === values.length);
export const TelemtWebSiteSchema = z
  .object({
    mode: z.enum(['auto', 'catalog', 'generate', 'html', 'origin']).default('auto'),
    catalog: z.enum(['builtin', 'mtproxyl']).optional(),
    template: z.string().optional(),
    theme: z.enum(TelemtWebThemes).optional(),
    title: z.string().max(80).optional(),
    description: z.string().max(300).optional(),
    origin: z.string().optional(),
    originResolve: z.boolean().optional(),
    html: z
      .string()
      .refine(
        (value) => new TextEncoder().encode(value).length <= COVER_SITE_HTML_MAX_BYTES,
        'HTML: максимум 1 МиБ',
      )
      .optional(),
  })
  .refine(
    (site) =>
      site.mode !== 'html' || (/<html[\s>]/i.test(site.html || '') && !site.html?.includes('\0')),
    {
      path: ['html'],
      message: 'Загрузите полный HTML-документ',
    },
  )
  .refine(
    (site) => {
      if (site.mode !== 'origin') return true;
      try {
        const url = new URL(site.origin || '');
        const port = Number(url.port);
        return (
          url.protocol === 'http:' &&
          (['127.0.0.1', '[::1]'].includes(url.hostname) ||
            (site.originResolve === true &&
              (url.hostname === 'localhost' ||
                /^(?:[a-z\d](?:[a-z\d-]{0,61}[a-z\d])?\.)+[a-z]{2,63}$/i.test(url.hostname)))) &&
          !!url.port &&
          port > 0 &&
          port <= 65535 &&
          url.pathname === '/' &&
          !url.username &&
          !url.password &&
          !url.search &&
          !url.hash
        );
      } catch {
        return false;
      }
    },
    {
      path: ['origin'],
      message: 'Укажите локальный HTTP-адрес; DNS-имя требует включить разрешение при запуске',
    },
  )
  .refine(
    (site) => site.mode !== 'catalog' || isTelemtWebCatalogTemplate(site.catalog, site.template),
    { path: ['template'], message: 'Выберите страницу из каталога' },
  );

export type TelemtWebSite = z.infer<typeof TelemtWebSiteSchema>;

export const TelemtWebDomainSchema = z
  .string()
  .trim()
  .min(1)
  .max(253)
  .regex(/^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$/);

export const TelemtSharedIngressSchema = z.object({
  enabled: z.boolean().default(false),
  naiveInboundId: z.number().int().nonnegative().optional(),
});

export const TelemtWebProxyRequestSchema = z.object({
  sharedIngress: TelemtSharedIngressSchema.optional(),
  aliases: z.array(TelemtWebDomainSchema).max(7).default([]),
  retryJitter: z
    .object({
      enabled: z.boolean().default(false),
      minMs: z.number().int().min(0).max(30000).default(0),
      maxMs: z.number().int().min(0).max(30000).default(250),
    })
    .refine((value) => value.minMs <= value.maxMs)
    .default({ enabled: false, minMs: 0, maxMs: 250 }),
  basePath: TelemtWebBasePathSchema.default(''),
  site: TelemtWebSiteSchema.default({ mode: 'auto' }),
  domain: TelemtWebDomainSchema,
  frontend: TelemtFrontendSchema.default('nginx'),
  carrier: TelemtCarrierSchema.default('https'),
  carriers: TelemtCarriersSchema.default([]),
});

// Do not share mutable default arrays across forms or invent public domains.
export function defaultTelemtWebSettings() {
  return {
    enabled: false,
    site: { mode: 'auto' as const },
    domain: '',
    basePath: '',
    aliases: [] as string[],
    frontend: 'nginx' as const,
    carrier: 'https' as const,
    carriers: [] as z.infer<typeof TelemtCarrierSchema>[],
    retryJitter: { enabled: false, minMs: 0, maxMs: 250 },
  };
}

// Complete legacy WEB settings on enable without overwriting user choices.
export function completeTelemtWebSettings(
  value: (Partial<z.infer<typeof TelemtWebProxyRequestSchema>> & { enabled?: boolean }) | undefined,
) {
  const defaults = defaultTelemtWebSettings();
  return {
    ...defaults,
    ...value,
    site: value?.site ?? defaults.site,
    domain: value?.domain ?? defaults.domain,
    basePath: value?.basePath ?? defaults.basePath,
    aliases: value?.aliases ?? defaults.aliases,
    frontend: value?.frontend || defaults.frontend,
    carrier: value?.carrier || defaults.carrier,
    carriers: value?.carriers ?? defaults.carriers,
    retryJitter: { ...defaults.retryJitter, ...value?.retryJitter },
  };
}
