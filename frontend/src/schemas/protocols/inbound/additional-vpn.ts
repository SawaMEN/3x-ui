import { z } from 'zod';
import { NaiveClientSchema } from './naive';
import {
  OPENFLUX_TRANSPORTS,
  validOpenFluxChannelURL,
  validOpenFluxMAXToken,
  validOpenFluxMAXUID,
} from '@/lib/openflux';

export const OpenFluxTransportSchema = z
  .object({
    type: z.enum(OPENFLUX_TRANSPORTS),
    url: z.string().default(''),
    token: z.string().max(16384).optional(),
    uid: z.string().optional(),
    priority: z.number().int().min(0).max(1000).default(50),
  })
  .superRefine((value, ctx) => {
    if (value.type === 'oneme') {
      if (!validOpenFluxMAXToken(value.token ?? ''))
        ctx.addIssue({
          code: 'custom',
          path: ['token'],
          message: 'pages.inbounds.form.openfluxMaxTokenInvalid',
        });
      if (value.uid && !validOpenFluxMAXUID(value.uid))
        ctx.addIssue({
          code: 'custom',
          path: ['uid'],
          message: 'pages.inbounds.form.openfluxMaxIDInvalid',
        });
    }
    if (value.type !== 'direct' && value.type !== 'oneme' && !validOpenFluxChannelURL(value.url)) {
      ctx.addIssue({
        code: 'custom',
        path: ['url'],
        message: 'pages.inbounds.form.openfluxURLRequired',
      });
    }
  });
export const FptnInboundSettingsSchema = z.object({
  image: z.string().default('fptnvpn/fptn-vpn-server:0.4.4'),
  hostname: z.string().default('fptn.local'),
  certificate: z.string().default(''),
  privateKey: z.string().default(''),
  certificatePEM: z.string().default(''),
  privateKeyPEM: z.string().default(''),
  metricsKey: z.string().default(''),
  mtu: z.number().int().min(576).max(9000).default(1400),
  maxSessions: z.number().int().min(1).max(1000).default(3),
  bandwidth: z.number().int().min(0).max(2000).default(0),
  detectProbing: z.boolean().default(true),
  allowedSni: z.string().default('www.bing.com'),
  adsFilter: z.boolean().default(false),
  torrentFilter: z.boolean().default(false),
  spamFilter: z.boolean().default(true),
  clients: z.array(NaiveClientSchema).default([]),
});
export const OpenFluxInboundSettingsSchema = z
  .object({
    maxPacketSize: z
      .number()
      .int()
      .refine(
        (v) => v === 0 || (v >= 1280 && v <= 65000),
        'pages.inbounds.form.openfluxPacketInvalid',
      )
      .default(0),
    refusePeerLogin: z.boolean().default(false),
    debug: z.number().int().min(0).max(2).default(0),
    context: z
      .string()
      .max(4096)
      .refine(
        (value) =>
          value.trim() === value &&
          !/[\r\n\0]/.test(value) &&
          new TextEncoder().encode(value).length <= 4096,
        'pages.inbounds.form.openfluxContextInvalid',
      )
      .default(''),
    transports: z
      .array(OpenFluxTransportSchema)
      .min(1)
      .max(OPENFLUX_TRANSPORTS.length)
      .default([{ type: 'direct', url: '', priority: 100 }]),
    clients: z
      .array(
        NaiveClientSchema.superRefine((client, ctx) => {
          if (
            client.enable &&
            client.password &&
            (client.password.length < 16 ||
              client.password.trim() !== client.password ||
              /[\r\n\0]/.test(client.password))
          ) {
            ctx.addIssue({
              code: 'custom',
              path: ['password'],
              message: 'pages.inbounds.form.openfluxSecretInvalid',
            });
          }
        }),
      )
      .max(1)
      .default([]),
  })
  .superRefine((value, ctx) => {
    const seen = new Set<string>();
    value.transports.forEach((transport, index) => {
      if (seen.has(transport.type))
        ctx.addIssue({
          code: 'custom',
          path: ['transports', index, 'type'],
          message: 'pages.inbounds.form.openfluxDuplicate',
        });
      seen.add(transport.type);
    });
  });
