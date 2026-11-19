import { z } from 'zod';

// A virtual local SOCKS endpoint, not a native Xray/sing-box protocol.
export const Zapret2OutboundSettingsSchema = z.object({
  port: z.number().int().min(1025).max(65535).default(15900),
  profile: z.enum(['auto', 'safe', 'balanced', 'aggressive']).default('auto'),
  tcp: z.boolean().default(true),
  udp: z.boolean().default(false),
  testDomains: z.array(z.string().trim().min(1).max(253)).max(32).default([]),
});
export type Zapret2OutboundSettings = z.infer<typeof Zapret2OutboundSettingsSchema>;
