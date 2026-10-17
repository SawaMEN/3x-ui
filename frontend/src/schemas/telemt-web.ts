import { z } from 'zod';

export const TelemtFrontendSchema = z.enum(['nginx', 'caddy', 'haproxy']);
export const TelemtCarrierSchema = z.enum(['https', 'https-lanes', 'websocket', 'websocket-lanes']);
export const TelemtCarriersSchema = z
  .array(TelemtCarrierSchema)
  .max(4)
  .refine((values) => new Set(values).size === values.length);
export const TelemtWebProxyRequestSchema = z.object({
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
