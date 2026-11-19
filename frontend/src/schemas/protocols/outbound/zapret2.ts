import { z } from 'zod';

export const Zapret2SelectedStrategySchema = z.object({
  kind: z.enum(['http', 'https', 'quic']),
  ip: z.enum(['4', '6']),
  test: z.enum(['curl_test_http', 'curl_test_https_tls12', 'curl_test_https_tls13', 'curl_test_http3']),
  args: z.array(z.string().min(1).max(512)).min(1).max(24),
  passed: z.number().int().min(1),
  total: z.number().int().min(1),
});

export const Zapret2CalibrationSchema = z.object({
  source: z.literal('blockcheck2'),
  testedAt: z.string().min(1),
  domains: z.array(z.string().min(1).max(253)).min(1).max(12),
  level: z.enum(['quick', 'standard', 'force']),
  selected: z.array(Zapret2SelectedStrategySchema).min(1).max(6),
});

export type Zapret2Calibration = z.infer<typeof Zapret2CalibrationSchema>;

// A virtual local SOCKS endpoint, not a native Xray/sing-box protocol.
export const Zapret2OutboundSettingsSchema = z.object({
  port: z.number().int().min(1025).max(65535).default(15900),
  profile: z.enum(['auto', 'safe', 'balanced', 'aggressive']).default('auto'),
  tcp: z.boolean().default(true),
  udp: z.boolean().default(false),
  testDomains: z.array(z.string().trim().min(1).max(253)).max(32).default([]),
  calibration: Zapret2CalibrationSchema.optional(),
});
export type Zapret2OutboundSettings = z.infer<typeof Zapret2OutboundSettingsSchema>;
