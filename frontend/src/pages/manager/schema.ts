import { z } from 'zod';
export const EntrySchema = z.object({
  templateId: z.number().default(0),
  mode: z.enum(['rotate', 'existing', 'custom']).default('rotate'),
  nodeId: z.number().optional(),
  port: z.string().default('random'),
  sni: z.string().default(''),
  label: z.string().max(256).default(''),
  flag: z.string().max(16).default(''),
  relayId: z.number().default(0),
  customLink: z.string().default(''),
  activeInboundId: z.number().default(0),
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
  intervalMinutes: z.number().min(10).max(525600).default(60),
  nextRotation: z.number().default(0),
  lastRotation: z.number().default(0),
  happ: z.string().default(''),
  incy: z.string().default(''),
  entries: z.array(EntrySchema).min(1).max(20),
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
export const SnapshotSchema = z.object({
  subscriptions: z.array(SubscriptionSchema),
  relays: z.array(RelaySchema),
  domains: z.array(z.object({ id: z.number(), name: z.string() })),
  operations: z.array(
    z.object({
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
    }),
  ),
  operationTotal: z.number(),
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
export type Subscription = z.infer<typeof SubscriptionSchema>;
export type Relay = z.infer<typeof RelaySchema>;
export const newEntry = () => EntrySchema.parse({});
export const newSubscription = (): Subscription => ({
  ...SubscriptionSchema.parse({ name: 'New', entries: [newEntry()] }),
  name: '',
});
