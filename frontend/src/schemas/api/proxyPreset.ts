import { z } from 'zod';

import { AlpnSchema } from '@/schemas/protocols/security/tls';
import { HostSecuritySchema, MihomoIpVersionSchema, SubTypeSchema } from '@/schemas/api/host';

export const ProxyPresetConfigSchema = z.object({
  port: z.number().int().min(0).max(65535).optional(),
  serverDescription: z.string().max(64).optional(),
  security: HostSecuritySchema.or(z.literal('')).optional(),
  sni: z.string().optional(),
  hostHeader: z.string().optional(),
  path: z.string().optional(),
  alpn: z.array(AlpnSchema).optional(),
  fingerprint: z.string().optional(),
  cipherSuites: z.string().optional(),
  overrideSniFromAddress: z.boolean().optional(),
  keepSniBlank: z.boolean().optional(),
  pinnedPeerCertSha256: z.array(z.string()).optional(),
  verifyPeerCertByName: z.string().optional(),
  allowInsecure: z.boolean().optional(),
  echConfigList: z.string().optional(),
  muxParams: z.string().optional(),
  sockoptParams: z.string().optional(),
  finalMask: z.string().optional(),
  vlessRoute: z.string().optional(),
  excludeFromSubTypes: z.array(SubTypeSchema).optional(),
  mihomoIpVersion: MihomoIpVersionSchema.or(z.literal('')).optional(),
  mihomoX25519: z.boolean().optional(),
  shuffleHost: z.boolean().optional(),
});
export type ProxyPresetConfig = z.infer<typeof ProxyPresetConfigSchema>;

export const ProxyPresetInputSchema = z.object({
  name: z.string().trim().min(1).max(120),
  description: z.string().max(1000).default(''),
  config: ProxyPresetConfigSchema,
});
export type ProxyPresetInput = z.infer<typeof ProxyPresetInputSchema>;

export const ProxyPresetViewSchema = ProxyPresetInputSchema.extend({
  id: z.number().int().positive(),
  createdAt: z.number(),
  updatedAt: z.number(),
});
export type ProxyPresetView = z.infer<typeof ProxyPresetViewSchema>;
export const ProxyPresetListSchema = z.array(ProxyPresetViewSchema);

export const ProxyPresetAssignmentSchema = z.object({
  groupId: z.string(),
  presetId: z.number().int().positive(),
  presetName: z.string(),
});
export type ProxyPresetAssignment = z.infer<typeof ProxyPresetAssignmentSchema>;

export const ProxyBundleHostSchema = z.object({
  groupId: z.string(),
  inboundIds: z.array(z.number()).nullable().optional(),
  inboundTags: z.array(z.string()),
  hosts: z.array(z.string()).nullable().optional(),
  sortOrder: z.number().optional(),
  remark: z.string(),
  serverDescription: z.string().optional(),
  isDisabled: z.boolean().optional(),
  isHidden: z.boolean().optional(),
  tags: z.array(z.string()).nullable().optional(),
  port: z.number().optional(),
  security: z.string().optional(),
  sni: z.string().optional(),
  hostHeader: z.string().optional(),
  path: z.string().optional(),
  alpn: z.array(z.string()).nullable().optional(),
  fingerprint: z.string().optional(),
  cipherSuites: z.string().optional(),
  overrideSniFromAddress: z.boolean().optional(),
  keepSniBlank: z.boolean().optional(),
  pinnedPeerCertSha256: z.array(z.string()).nullable().optional(),
  verifyPeerCertByName: z.string().optional(),
  allowInsecure: z.boolean().optional(),
  echConfigList: z.string().optional(),
  muxParams: z.string().optional(),
  sockoptParams: z.string().optional(),
  finalMask: z.string().optional(),
  vlessRoute: z.string().optional(),
  excludeFromSubTypes: z.array(z.string()).nullable().optional(),
  nodeGuids: z.array(z.string()).nullable().optional(),
  mihomoIpVersion: z.string().optional(),
  mihomoX25519: z.boolean().optional(),
  shuffleHost: z.boolean().optional(),
  presetName: z.string().optional(),
});

export const ProxyBundleSchema = z.object({
  version: z.literal(1),
  exportedAt: z.number(),
  presets: z.array(ProxyPresetInputSchema),
  hosts: z.array(ProxyBundleHostSchema),
});
export type ProxyBundle = z.infer<typeof ProxyBundleSchema>;

export const ProxyBundleImportResultSchema = z.object({
  dryRun: z.boolean(),
  createdPresets: z.number().int().min(0),
  updatedPresets: z.number().int().min(0),
  createdHosts: z.number().int().min(0),
  updatedHosts: z.number().int().min(0),
  assignedPresets: z.number().int().min(0),
  skippedHosts: z.array(z.string()),
  missingInboundTags: z.array(z.string()),
  missingPresetNames: z.array(z.string()),
});
export type ProxyBundleImportResult = z.infer<typeof ProxyBundleImportResultSchema>;
