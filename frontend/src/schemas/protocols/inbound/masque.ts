import { z } from 'zod';
import { NaiveClientSchema, NaiveTlsSchema } from './naive';

export const MasqueInboundSettingsSchema = z.object({
  version: z
    .array(z.union([z.literal(1), z.literal(2), z.literal(3)]))
    .min(1)
    .refine(
      (versions) => new Set(versions).size === versions.length,
      'HTTP versions must be unique',
    )
    .default([3, 2, 1]),
  path: z
    .string()
    .startsWith('/')
    .refine((path) => {
      // Only the CONNECT-IP variables and simple/query expansion operators
      // supported by sing-box can be shared with a client.
      if (!/^\/[\x21-\x7e]*$/.test(path) || path.includes('#')) return false;
      const literal = path.replace(
        /\{(?:target|ipproto|[?&](?:target|ipproto)(?:,(?:target|ipproto))*)\}/g,
        '',
      );
      return !/[{}]/.test(literal) && !/%(?![0-9a-fA-F]{2})/.test(literal);
    }, 'Invalid MASQUE URI template')
    .default('/.well-known/masque/ip/{target}/{ipproto}/'),
  address: z.array(z.string()).min(1).default(['172.31.255.1/24', 'fd7a:115c:a1e0::1/64']),
  advertiseRoutes: z.array(z.string()).default([]),
  mtu: z.number().int().min(1280).max(65535).default(1280),
  tls: NaiveTlsSchema.default({ enabled: true, serverName: '', certificatePath: '', keyPath: '' }),
  clients: z
    .array(
      NaiveClientSchema.extend({
        email: z
          .string()
          .min(1)
          .refine(
            (email) =>
              email.trim() !== '' &&
              !email.includes(':') &&
              Array.from(email).every(
                (character) => character.charCodeAt(0) >= 0x20 && character.charCodeAt(0) !== 0x7f,
              ),
            'Invalid HTTP Basic username',
          ),
      }),
    )
    .refine(
      (clients) =>
        new Set(clients.map((client) => client.email.toLowerCase())).size === clients.length,
      'Usernames must be unique',
    )
    .default([]),
});
export type MasqueInboundSettings = z.infer<typeof MasqueInboundSettingsSchema>;
