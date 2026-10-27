import { z } from 'zod';
import { isTelemtWebCatalogTemplate, TelemtWebThemes } from './telemt-web-catalog';

export const TelemtFrontendSchema = z.enum(['nginx', 'caddy', 'haproxy']);
export const TelemtCarrierSchema = z.enum(['https', 'https-lanes', 'websocket', 'websocket-lanes']);
export const TelemtWebBasePathSchema = z.string().trim().refine(
  (value) => value === '' || (value.length >= 6 && value.length <= 96 && /^[A-Za-z0-9_-]+(?:\/[A-Za-z0-9_-]+)*$/.test(value.replace(/^\/+|\/+$/g, ''))),
  'Путь должен содержать 6–96 символов: буквы, цифры, /, - или _',
);

export const TelemtCarriersSchema = z
  .array(TelemtCarrierSchema)
  .max(4)
  .refine((values) => new Set(values).size === values.length);
export const TelemtWebSiteSchema = z
  .object({
    mode: z.enum(['auto', 'catalog', 'generate', 'html']).default('auto'),
    catalog: z.enum(['builtin', 'mtproxyl']).optional(),
    template: z.string().optional(),
    theme: z.enum(TelemtWebThemes).optional(),
    title: z.string().max(80).optional(),
    description: z.string().max(300).optional(),
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
    (site) => site.mode !== 'catalog' || isTelemtWebCatalogTemplate(site.catalog, site.template),
    { path: ['template'], message: 'Выберите страницу из каталога' },
  );

export type TelemtWebSite = z.infer<typeof TelemtWebSiteSchema>;

export const TelemtWebProxyRequestSchema = z.object({
  basePath: TelemtWebBasePathSchema.default(''),
  site: TelemtWebSiteSchema.optional(),
  domain: z
    .string()
    .trim()
    .min(1)
    .max(253)
    .regex(/^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$/),
  frontend: TelemtFrontendSchema,
  carrier: TelemtCarrierSchema,
  carriers: TelemtCarriersSchema,
});
