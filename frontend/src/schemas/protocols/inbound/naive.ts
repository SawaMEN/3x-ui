import { z } from 'zod';

export const NaiveClientSchema = z.object({
  email: z.string().min(1),
  password: z.string().default(''),
  limitIp: z.number().int().min(0).default(0),
  totalGB: z.number().int().min(0).default(0),
  expiryTime: z.number().int().default(0),
  enable: z.boolean().default(true),
  tgId: z.union([z.number(), z.string()]).transform((v) => Number(v) || 0).default(0),
  subId: z.string().default(''),
  comment: z.string().default(''),
  reset: z.number().int().min(0).default(0),
  created_at: z.number().int().optional(),
  updated_at: z.number().int().optional(),
});
export type NaiveClient = z.infer<typeof NaiveClientSchema>;

export const NaiveTlsSchema = z.object({
  enabled: z.boolean().default(true),
  serverName: z.string().default(''),
  certificatePath: z.string().default(''),
  keyPath: z.string().default(''),
});
export type NaiveTls = z.infer<typeof NaiveTlsSchema>;

export const NaiveInboundSettingsSchema = z.object({
  network: z.enum(['', 'tcp', 'udp']).default('tcp'),
  quicCongestionControl: z.enum(['bbr', 'cubic', 'reno']).default('bbr'),
  tls: NaiveTlsSchema.default({
    enabled: true,
    serverName: '',
    certificatePath: '',
    keyPath: '',
  }),
  clients: z.array(NaiveClientSchema).default([]),
});
export type NaiveInboundSettings = z.infer<typeof NaiveInboundSettingsSchema>;
