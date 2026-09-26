import { z } from 'zod';

import { NaiveClientSchema } from './naive';

export const PingtunnelInboundSettingsSchema = z.object({
  key: z.number().int().min(0).max(2147483647).default(0),
  encrypt: z.enum(['chacha20', 'aes256', 'aes128']).default('chacha20'),
  encryptKey: z.string().default(''),
  clients: z.array(z.never()).default([]),
});
export type PingtunnelInboundSettings = z.infer<typeof PingtunnelInboundSettingsSchema>;

export const TrustTunnelInboundSettingsSchema = z.object({
  hostname: z.string().min(1).default('trusttunnel.local'),
  certificate: z.string().default(''),
  privateKey: z.string().default(''),
  clients: z.array(NaiveClientSchema).default([]),
});
export type TrustTunnelInboundSettings = z.infer<typeof TrustTunnelInboundSettingsSchema>;
