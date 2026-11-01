import { z } from 'zod';

export const VETROFF_TRANSPORTS = [
  'h2-connect',
  'h2-post',
  'h3-connect',
  'h3-post',
  'wss',
  'xhttp-stream-one',
  'xhttp-stream-up',
  'xhttp-packet-up',
] as const;
export const VetroffUserPolicySchema = z.object({
  email: z.string().default(''),
  maxRequests: z.number().int().min(1).max(4096).default(128),
  maxTunnels: z.number().int().min(1).max(4096).default(32),
  requestsPerSecond: z.number().min(0.1).max(10000).default(100),
  burst: z.number().int().min(1).max(20000).default(200),
  quotaBytes: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER).default(0),
});
const origin = z.url().refine((raw) => {
  const u = new URL(raw);
  return (
    u.protocol === 'https:' &&
    !u.username &&
    !u.password &&
    (!u.pathname || u.pathname === '/') &&
    !u.search &&
    !u.hash
  );
});
const optionalOrigin = z.union([z.literal(''), origin]).default('');
export const VetroffOptionsSchema = z.object({
  autoTLS: z.boolean().default(true),
  autoPin: z.boolean().default(true),
  transport: z.enum(VETROFF_TRANSPORTS).default('h2-connect'),
  tlsBackend: z.enum(['auto', 'go', 'nss']).default('auto'),
  http3: z.boolean().default(true),
  enableUDP: z.boolean().default(true),
  h3Listen: z.string().default(''),
  xhttpPath: z.string().default(''),
  xhttpHost: z.string().default(''),
  protocolPolicy: z.enum(['builtin', 'strict']).default('builtin'),
  padding: z.boolean().default(false),
  maxTunnels: z.number().int().min(1).max(65536).default(256),
  maxConnections: z.number().int().min(1).max(65536).default(512),
  webRoot: z.string().default(''),
  allowPrivate: z.boolean().default(false),
  egressProxy: z.string().default(''),
  egressDNS: z.string().default(''),
  publicServer: optionalOrigin,
  fallbackServers: z.array(origin).max(7).default([]),
  pin: z.string().default(''),
  balance: z.enum(['failover', 'round-robin', 'least-active', 'adaptive']).default('failover'),
  bond: z.boolean().default(false),
  discover: z.boolean().default(false),
  pipeline: z.enum(['auto', '4', '8']).default('auto'),
  connectTimeout: z.string().min(1).default('15s'),
  attemptTimeout: z.string().min(1).default('5s'),
  bondHub: z.boolean().default(false),
  bondUpstream: optionalOrigin,
  bondCA: z.string().default(''),
  bondSNI: z.string().default(''),
  bondPin: z.string().default(''),
  nodes: z
    .array(
      z.object({
        url: origin,
        weight: z.number().int().min(1).max(100).default(100),
        disabled: z.boolean().default(false),
        sni: z.string().default(''),
        pin: z.string().default(''),
      }),
    )
    .max(8)
    .default([]),
  attemptsPerSecond: z.number().min(0.1).max(10000).default(200),
  attemptBurst: z.number().int().min(1).max(20000).default(400),
  accessMaxRequests: z.number().int().min(1).max(65536).default(1024),
  userPolicy: VetroffUserPolicySchema.default(() => VetroffUserPolicySchema.parse({})),
  userPolicies: z.array(VetroffUserPolicySchema).max(1024).default([]),
});
export type VetroffOptions = z.infer<typeof VetroffOptionsSchema>;
