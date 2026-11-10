import { z } from 'zod';

// Native Hiddify/sing-box Hysteria options. Preserve extension fields on edit:
// older Xray-only schemas used to erase bandwidth and obfuscation settings.
export const HysteriaMasqueradeSchema = z
  .object({
    type: z.enum(['', 'proxy', 'file', 'string']).default(''),
    dir: z.string().default(''),
    url: z.string().default(''),
    rewriteHost: z.boolean().default(false),
    xForwarded: z.boolean().optional(),
    insecure: z.boolean().default(false),
    content: z.string().default(''),
    headers: z.record(z.string(), z.string()).default({}),
    statusCode: z
      .number()
      .int()
      .min(0)
      .max(599)
      .refine((n) => n === 0 || n >= 100)
      .default(0),
  })
  .loose();
export type HysteriaMasquerade = z.infer<typeof HysteriaMasqueradeSchema>;

export const HysteriaObfsSchema = z
  .object({
    type: z.enum(['salamander', 'gecko']),
    password: z.string().min(1),
    minPacketSize: z.number().int().min(1).max(2048).optional(),
    maxPacketSize: z.number().int().min(1).max(2048).optional(),
  })
  .loose()
  .refine(
    (obfs) =>
      obfs.minPacketSize === undefined ||
      obfs.maxPacketSize === undefined ||
      obfs.minPacketSize <= obfs.maxPacketSize,
    'Minimum packet size must not exceed maximum packet size',
  );

const HysteriaStreamSettingsBaseSchema = z
  .object({
    version: z.union([z.literal(1), z.literal(2)]).default(2),
    auth: z.string().default(''),
    udpIdleTimeout: z.number().int().min(1).max(4294967295).default(60),
    up: z.number().int().min(0).optional(),
    down: z.number().int().min(0).optional(),
    ignoreClientBandwidth: z.boolean().optional(),
    bbrProfile: z.enum(['conservative', 'standard', 'aggressive']).optional(),
    obfs: z.union([z.string(), HysteriaObfsSchema]).optional(),
    masquerade: HysteriaMasqueradeSchema.optional(),
  })
  .loose();
export const HysteriaStreamSettingsSchema = z.preprocess((value) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return value;
  const next = { ...value } as Record<string, unknown>;
  for (const [canonical, aliases] of Object.entries({
    up: ['up_mbps', 'upMbps'],
    down: ['down_mbps', 'downMbps'],
    ignoreClientBandwidth: ['ignore_client_bandwidth'],
    bbrProfile: ['bbr_profile'],
  })) {
    for (const alias of aliases) {
      if (next[canonical] === undefined && next[alias] !== undefined) next[canonical] = next[alias];
      delete next[alias];
    }
  }
  if (next.obfs && typeof next.obfs === 'object') {
    const obfs = { ...next.obfs } as Record<string, unknown>;
    for (const [canonical, alias] of [
      ['minPacketSize', 'min_packet_size'],
      ['maxPacketSize', 'max_packet_size'],
    ]) {
      if (obfs[canonical] === undefined) obfs[canonical] = obfs[alias];
      delete obfs[alias];
    }
    next.obfs = obfs;
  }
  return next;
}, HysteriaStreamSettingsBaseSchema);
export type HysteriaStreamSettings = z.infer<typeof HysteriaStreamSettingsSchema>;
