import { z } from 'zod';

const SudokuClientBaseSchema = z.object({
  email: z.string().min(1),
  sudokuPrivateKey: z.string().default(''),
  limitIp: z.number().int().min(0).default(0),
  totalGB: z.number().int().min(0).default(0),
  expiryTime: z.number().int().default(0),
  enable: z.boolean().default(true),
  tgId: z
    .union([z.number(), z.string()])
    .transform((v) => Number(v) || 0)
    .default(0),
  subId: z.string().default(''),
  comment: z.string().default(''),
  reset: z.number().int().min(0).default(0),
  created_at: z.number().int().optional(),
  updated_at: z.number().int().optional(),
});

export const SudokuClientSchema = SudokuClientBaseSchema;
export type SudokuClient = z.infer<typeof SudokuClientSchema>;

export const SudokuHttpMaskSchema = z.object({
  disable: z.boolean().default(false),
  mode: z.enum(['legacy', 'stream', 'poll', 'auto', 'ws']).default('legacy'),
  tls: z.boolean().default(false),
  host: z.string().default(''),
  pathRoot: z.string().default(''),
  multiplex: z.enum(['off', 'auto', 'on']).default('off'),
});

export const SudokuInboundSettingsSchema = z.object({
  fallbackAddress: z.string().default(''),
  key: z.string().default(''),
  aead: z.enum(['aes-128-gcm', 'chacha20-poly1305', 'none']).default('chacha20-poly1305'),
  suspiciousAction: z.enum(['fallback', 'silent']).default('fallback'),
  paddingMin: z.number().int().min(0).max(65535).default(5),
  paddingMax: z.number().int().min(0).max(65535).default(15),
  ascii: z
    .enum(['prefer_entropy', 'prefer_ascii', 'up_ascii_down_entropy', 'up_entropy_down_ascii'])
    .default('prefer_entropy'),
  customTable: z.string().default(''),
  customTables: z.array(z.string()).default([]),
  enablePureDownlink: z.boolean().default(true),
  multiplex: z.enum(['off', 'auto', 'on']).default('off'),
  httpmask: SudokuHttpMaskSchema.default({
    disable: false,
    mode: 'legacy',
    tls: false,
    host: '',
    pathRoot: '',
    multiplex: 'off',
  }),
  clients: z.array(SudokuClientSchema).default([]),
});
export type SudokuInboundSettings = z.infer<typeof SudokuInboundSettingsSchema>;
