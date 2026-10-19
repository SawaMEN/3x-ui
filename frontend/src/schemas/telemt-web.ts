import { z } from 'zod';

export const TelemtFrontendSchema = z.enum(['nginx', 'caddy', 'haproxy']);
export const TelemtCarrierSchema = z.enum(['https', 'https-lanes', 'websocket', 'websocket-lanes']);
export const TelemtCarriersSchema = z
  .array(TelemtCarrierSchema)
  .max(4)
  .refine((values) => new Set(values).size === values.length);
export const TelemtWebSiteSchema = z
  .object({
    mode: z.enum(['auto', 'generate', 'html']).default('auto'),
    theme: z.enum(['notes', 'technology', 'travel', 'nature']).optional(),
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
  );
export type TelemtWebSite = z.infer<typeof TelemtWebSiteSchema>;

export const TelemtWebProxyRequestSchema = z.object({
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
