import { z } from 'zod';
import { createManagerPreset } from './presets';
export const ManagerConfigSchema = z.object({
  shareAddrStrategy: z.string().default('node'),
  shareAddr: z.string().default(''),
  subSortIndex: z.number().default(1),
  protocol: z.string(),
  settings: z.string(),
  streamSettings: z.string(),
  sniffing: z.string(),
  listen: z.string().default(''),
  disableFlow: z.boolean().default(false),
});
export type ManagerConfig = z.infer<typeof ManagerConfigSchema>;
export const EntrySchema = z.object({
  templateId: z.number().default(0),
  preset: z.string().optional(),
  config: ManagerConfigSchema.optional(),
  mode: z.enum(['generate', 'rotate', 'existing', 'custom']).default('rotate'),
  nodeId: z.number().optional(),
  port: z.string().default('random'),
  sni: z.string().default(''),
  sniBackups: z.array(z.string()).max(6).optional(),
  realityTarget: z.string().max(512).optional(),
  label: z.string().max(256).default(''),
  flag: z.string().max(16).default(''),
  relayId: z.number().default(0),
  customLink: z.string().default(''),
  activeInboundId: z.number().default(0),
  sniHealthFailures: z.number().int().min(0).optional(),
  sniHealthCheckedAt: z.number().optional(),
  sniHealthLastSwitchAt: z.number().optional(),
});
export const SubscriptionSchema = z.object({
  id: z.number().default(0),
  token: z.string().default(''),
  name: z.string().min(1).max(256),
  note: z.string().max(4096).default(''),
  supportUrl: z.string().default(''),
  websiteUrl: z.string().default(''),
  clientId: z.number().default(0),
  ownClient: z.boolean().default(false),
  enabled: z.boolean().default(true),
  autoRotate: z.boolean().default(false),
  monitorSni: z.boolean().default(false),
  monitorSniIntervalMinutes: z.number().min(5).max(1440).default(10),
  nextSniHealthCheck: z.number().default(0),
  intervalMinutes: z.number().min(10).max(525600).default(60),
  graceMinutes: z.number().min(60).max(1440).default(120),
  nextRotation: z.number().default(0),
  lastRotation: z.number().default(0),
  happ: z.string().default(''),
  incy: z.string().default(''),
  entries: z.array(EntrySchema).min(1).max(100),
  published: z.array(EntrySchema).nullable().optional(),
});
export const RelaySchema = z.object({
  id: z.number().default(0),
  name: z.string().min(1),
  address: z.string().min(1),
  sshPort: z.number().min(1).max(65535).default(22),
  username: z.string().min(1).default('root'),
  hostKey: z.string().startsWith('SHA256:'),
  credential: z.string().default(''),
  keyAuth: z.boolean().default(false),
  nodeId: z.number().min(1),
  installed: z.boolean().default(false),
});
export const ManagerOperationSchema = z.object({
  id: z.number(),
  subscriptionId: z.number(),
  relayId: z.number(),
  kind: z.string(),
  state: z.string(),
  completed: z.number(),
  total: z.number(),
  attempts: z.number(),
  message: z.string(),
  createdAt: z.number(),
  finishedAt: z.number(),
});
export const SnapshotSchema = z.object({
  subscriptions: z.array(SubscriptionSchema),
  relays: z.array(RelaySchema),
  domains: z.array(z.object({ id: z.number(), name: z.string() })),
  operations: z.array(ManagerOperationSchema),
  activeOperations: z.array(ManagerOperationSchema).default([]),
  operationTotal: z.number(),
  subscriptionTotal: z.number().default(0),
  allSubscriptionsTotal: z.number().default(0),
  autoRotateTotal: z.number().default(0),
  pendingOperations: z.number().default(0),
  subscriptionPage: z.number().default(1),
  pageSize: z.number().default(25),
  clients: z.record(
    z.string(),
    z.object({
      email: z.string(),
      up: z.number(),
      down: z.number(),
      total: z.number(),
      expiryTime: z.number(),
      enabled: z.boolean(),
    }),
  ),
});
export const ClientOptionsSchema = z.object({
  items: z.array(z.object({ id: z.number(), email: z.string() })),
});
export type ManagerSnapshot = z.infer<typeof SnapshotSchema>;
export type Subscription = z.infer<typeof SubscriptionSchema>;
export type Relay = z.infer<typeof RelaySchema>;
export const newEntry = () =>
  EntrySchema.parse({
    mode: 'generate',
    preset: 'vless-tcp-reality',
    nodeId: 0,
    sni: 'random',
    config: createManagerPreset('vless-tcp-reality'),
  });
export const newSubscription = (): Subscription => ({
  ...SubscriptionSchema.parse({ name: 'New', entries: [newEntry()] }),
  name: '',
});

export const PreflightSchema = z.object({
  ready: z.boolean(),
  entries: z.array(z.object({ index: z.number(), ready: z.boolean(), message: z.string() })),
});
