import { z } from 'zod';
import { NaiveClientSchema } from './naive';

// PSK is allowed to be empty only before the first save: the backend then
// seeds a cryptographically random key, and preserves it on future edits.
export const SnellInboundSettingsSchema = z.object({
  version: z.union([z.literal(5), z.literal(6)]).default(6),
  psk: z.string().default(''),
  mode: z.enum(['default', 'unshaped']).default('default'),
  obfsMode: z.enum(['none', 'http']).default('none'),
  obfsHost: z.string().default('bing.com'),
  clients: z.array(NaiveClientSchema).default([]),
}).superRefine((value, ctx) => {
  const containsControl = (input: string) =>
    Array.from(input).some((character) => {
      const code = character.codePointAt(0) ?? 0;
      return code < 32 || (code >= 127 && code <= 159);
    });
  if (containsControl(value.psk)) {
    ctx.addIssue({ code: 'custom', path: ['psk'], message: 'PSK must not contain control characters' });
  }
  if (value.version === 6 && value.psk !== '') {
    const bytes = new TextEncoder().encode(value.psk).length;
    if (bytes < 12 || bytes > 255) {
      ctx.addIssue({ code: 'custom', path: ['psk'], message: 'Snell v6 PSK must be 12–255 bytes' });
    }
  }
  if (containsControl(value.obfsHost)) {
    ctx.addIssue({ code: 'custom', path: ['obfsHost'], message: 'HTTP Host must not contain control characters' });
  }
});
export type SnellInboundSettings = z.infer<typeof SnellInboundSettingsSchema>;
