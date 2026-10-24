import { z } from 'zod';
import {
  DomainFrontConfigSchema as ConfigResponseSchema,
  DomainFrontDomainSchema as DomainResponseSchema,
} from '@/generated/zod';

export const DomainModeSchema = z.enum(['direct', 'cdn', 'relay', 'sub_link_only']);
export const DomainTLSModeSchema = z.enum(['auto', 'manual']);
const hostname = z
  .string()
  .trim()
  .toLowerCase()
  .max(253)
  .regex(
    /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/,
    'pages.settings.domains.invalidHostname',
  );
export const DomainFormSchema = DomainResponseSchema.extend({
  name: hostname,
  mode: DomainModeSchema,
  tls: DomainTLSModeSchema,
  inboundIds: z.array(z.number().int().positive()),
});
export type DomainForm = z.infer<typeof DomainFormSchema>;
export const DomainConfigFormSchema = ConfigResponseSchema.extend({
  domains: z.array(DomainFormSchema).max(100),
  httpPort: z.number().int().min(1).max(65535),
  httpsPort: z.number().int().min(1).max(65535),
  publicPort: z.number().int().min(1).max(65535),
  email: z.union([z.literal(''), z.email()]),
}).superRefine((cfg, ctx) => {
  if (cfg.enabled && cfg.domains.some((d) => d.enabled && d.tls === 'auto') && !cfg.acceptTerms)
    ctx.addIssue({
      code: 'custom',
      path: ['acceptTerms'],
      message: 'pages.settings.domains.termsRequired',
    });
  if (cfg.httpPort === cfg.httpsPort)
    ctx.addIssue({
      code: 'custom',
      path: ['httpsPort'],
      message: 'pages.settings.domains.differentPorts',
    });
  if (new Set(cfg.domains.map((d) => d.name)).size !== cfg.domains.length)
    ctx.addIssue({
      code: 'custom',
      path: ['domains'],
      message: 'pages.settings.domains.duplicate',
    });
});
export type DomainConfigForm = z.infer<typeof DomainConfigFormSchema>;
export const defaultDomainConfig: DomainConfigForm = {
  enabled: false,
  listen: '0.0.0.0',
  httpPort: 80,
  httpsPort: 443,
  publicPort: 443,
  email: '',
  acceptTerms: false,
  staging: false,
  subscriptionDomain: '',
  domains: [],
};
export const defaultDomain: DomainForm = {
  name: '',
  mode: 'direct',
  enabled: true,
  panel: false,
  inboundIds: [],
  origin: '',
  tls: 'auto',
  certFile: '',
  keyFile: '',
};
