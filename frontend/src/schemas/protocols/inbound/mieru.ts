import { z } from 'zod';

export const MieruClientSchema = z.object({
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
export type MieruClient = z.infer<typeof MieruClientSchema>;

const MieruPortSchema = z.preprocess(
  (value) =>
    Array.isArray(value)
      ? value
          .map((item) => (typeof item === 'string' ? Number(item.trim()) : item))
          .filter((item) => Number.isFinite(item))
      : value,
  z.array(z.number().int().min(1).max(65535)).default([]),
);

export const MieruInboundSettingsSchema = z.object({
  protocols: z.array(z.enum(['TCP', 'UDP'])).min(1).default(['TCP', 'UDP']),
  additionalPorts: MieruPortSchema,
  mtu: z.number().int().min(1280).max(1400).default(1400),
  loggingLevel: z.enum(['OFF', 'ERROR', 'WARN', 'INFO', 'DEBUG']).default('INFO'),
  userHintIsMandatory: z.boolean().default(false),
  clients: z.array(MieruClientSchema).default([]),
});
export type MieruInboundSettings = z.infer<typeof MieruInboundSettingsSchema>;
