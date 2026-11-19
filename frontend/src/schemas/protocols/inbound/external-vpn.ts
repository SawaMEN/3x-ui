import { z } from 'zod';
import { VetroffOptionsSchema } from './vetroff';

import { NaiveClientSchema } from './naive';

export const PingtunnelInboundSettingsSchema = z.object({
  password: z
    .string()
    .refine((v) => !v || v.length >= 8, 'Password must be at least 8 characters')
    .default(''),
  encrypt: z.enum(['chacha20', 'aes256', 'aes128']).catch('chacha20'),
  maxConn: z.number().int().min(0).default(0),
  connectTimeout: z.number().int().min(0).default(3000),
  congestion: z.enum(['bb', 'none']).default('bb'),
  // These are subscription attachments only; Pingtunnel still authenticates all
  // clients with a single shared ICMP password, not per-client credentials.
  clients: z.array(NaiveClientSchema).default([]),
});
export type PingtunnelInboundSettings = z.infer<typeof PingtunnelInboundSettingsSchema>;

export const TrustTunnelInboundSettingsSchema = z
  .object({
    hostname: z
      .string()
      .trim()
      .min(1)
      .refine((name) => !/[\s/:\\"]/.test(name), {
        message: 'pages.inbounds.form.trusttunnelHostnameInvalid',
      })
      .default('trusttunnel.local'),
    certificate: z.string().default(''),
    privateKey: z.string().default(''),
    clients: z.array(NaiveClientSchema).default([]),
  })
  .superRefine((settings, ctx) => {
    if (!!settings.certificate.trim() !== !!settings.privateKey.trim()) {
      ctx.addIssue({
        code: 'custom',
        path: [settings.certificate.trim() ? 'privateKey' : 'certificate'],
        message: 'pages.inbounds.form.tlsPairRequired',
      });
    }
  });
export type TrustTunnelInboundSettings = z.infer<typeof TrustTunnelInboundSettingsSchema>;

export const VetroffInboundSettingsSchema = z.object({
  hostname: z.string().trim().default(''),
  certificate: z.string().trim().default(''),
  privateKey: z.string().trim().default(''),
  vetroff: VetroffOptionsSchema.optional(),
  clients: z.array(NaiveClientSchema).max(1024).default([]),
});

export type VetroffInboundSettings = z.infer<typeof VetroffInboundSettingsSchema>;
