import { z } from 'zod';
export const AwgClientOptionsSchema = z.object({
  jc: z.number().int().min(0).max(4294967295).optional(),
  jmin: z.number().int().min(0).max(4294967295).optional(),
  jmax: z.number().int().min(0).max(4294967295).optional(),
  i1: z.string().optional(),
  i2: z.string().optional(),
  i3: z.string().optional(),
  i4: z.string().optional(),
  i5: z.string().optional(),
  mtu: z.number().int().min(1280).max(1500).optional(),
});
export type AwgClientOptions = z.infer<typeof AwgClientOptionsSchema>;
