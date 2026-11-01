import { z } from 'zod';
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
    html: z
      .string()
      .refine(
        (value) => new TextEncoder().encode(value).length <= 256 * 1024,
        'HTML: максимум 256 КиБ',
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
          ['127.0.0.1', '[::1]'].includes(url.hostname) &&
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
    { path: ['origin'], message: 'Укажите локальный HTTP-адрес, например http://127.0.0.1:8080' },
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

export const TelemtWebProxyRequestSchema = z.object({
  aliases: z.array(TelemtWebDomainSchema).max(7).default([]),
  basePath: TelemtWebBasePathSchema.default(''),
  site: TelemtWebSiteSchema.optional(),
  domain: TelemtWebDomainSchema,
  frontend: TelemtFrontendSchema,
  carrier: TelemtCarrierSchema,
  carriers: TelemtCarriersSchema,
});
