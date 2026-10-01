import type { Section } from './endpoints';

const proxyPresetConfigSchema = {
  type: 'object',
  additionalProperties: false,
  properties: {
    port: { type: 'integer', minimum: 0, maximum: 65535 },
    serverDescription: { type: 'string', maxLength: 64 },
    security: { type: 'string', enum: ['', 'same', 'tls', 'none', 'reality'] },
    sni: { type: 'string' },
    hostHeader: { type: 'string' },
    path: { type: 'string' },
    alpn: { type: 'array', items: { type: 'string' } },
    fingerprint: { type: 'string' },
    cipherSuites: { type: 'string' },
    overrideSniFromAddress: { type: 'boolean' },
    keepSniBlank: { type: 'boolean' },
    pinnedPeerCertSha256: { type: 'array', items: { type: 'string' } },
    verifyPeerCertByName: { type: 'string' },
    allowInsecure: { type: 'boolean' },
    echConfigList: { type: 'string' },
    muxParams: { type: 'string', description: 'JSON object encoded as text.' },
    sockoptParams: { type: 'string', description: 'JSON object encoded as text.' },
    finalMask: { type: 'string', description: 'JSON object encoded as text.' },
    vlessRoute: { type: 'string' },
    excludeFromSubTypes: {
      type: 'array',
      items: { type: 'string', enum: ['raw', 'json', 'clash'] },
    },
    mihomoIpVersion: {
      type: 'string',
      enum: ['', 'dual', 'ipv4', 'ipv6', 'ipv4-prefer', 'ipv6-prefer'],
    },
    mihomoX25519: { type: 'boolean' },
    shuffleHost: { type: 'boolean' },
  },
} as const;

const proxyPresetInputSchema = {
  type: 'object',
  required: ['name', 'config'],
  properties: {
    name: { type: 'string', minLength: 1, maxLength: 120 },
    description: { type: 'string', maxLength: 1000 },
    config: proxyPresetConfigSchema,
  },
} as const;

const proxyPresetViewSchema = {
  allOf: [
    proxyPresetInputSchema,
    {
      type: 'object',
      required: ['id', 'createdAt', 'updatedAt'],
      properties: {
        id: { type: 'integer', minimum: 1 },
        createdAt: { type: 'integer', format: 'int64' },
        updatedAt: { type: 'integer', format: 'int64' },
      },
    },
  ],
} as const;

const assignmentSchema = {
  type: 'object',
  required: ['groupId', 'presetId', 'presetName'],
  properties: {
    groupId: { type: 'string' },
    presetId: { type: 'integer', minimum: 1 },
    presetName: { type: 'string' },
  },
} as const;

const hostGroupSchema = {
  type: 'object',
  required: ['groupId', 'inboundIds', 'hosts', 'remark'],
  additionalProperties: true,
  properties: {
    groupId: { type: 'string' },
    inboundIds: { type: 'array', items: { type: 'integer' } },
    hosts: { type: 'array', items: { type: 'string' } },
    remark: { type: 'string' },
    port: { type: 'integer', minimum: 0, maximum: 65535 },
    security: { type: 'string' },
    sni: { type: 'string' },
    nodeGuids: { type: 'array', items: { type: 'string' } },
  },
} as const;

const proxyBundleSchema = {
  type: 'object',
  required: ['version', 'exportedAt', 'presets', 'hosts'],
  properties: {
    version: { type: 'integer', enum: [1] },
    exportedAt: { type: 'integer', format: 'int64' },
    presets: { type: 'array', items: proxyPresetInputSchema },
    hosts: {
      type: 'array',
      items: {
        allOf: [
          hostGroupSchema,
          {
            type: 'object',
            required: ['inboundTags'],
            properties: {
              inboundTags: { type: 'array', minItems: 1, items: { type: 'string' } },
              presetName: { type: 'string' },
            },
          },
        ],
      },
    },
  },
} as const;

const importResultSchema = {
  type: 'object',
  required: [
    'dryRun',
    'createdPresets',
    'updatedPresets',
    'createdHosts',
    'updatedHosts',
    'assignedPresets',
    'skippedHosts',
    'missingInboundTags',
    'missingPresetNames',
  ],
  properties: {
    dryRun: { type: 'boolean' },
    createdPresets: { type: 'integer', minimum: 0 },
    updatedPresets: { type: 'integer', minimum: 0 },
    createdHosts: { type: 'integer', minimum: 0 },
    updatedHosts: { type: 'integer', minimum: 0 },
    assignedPresets: { type: 'integer', minimum: 0 },
    skippedHosts: { type: 'array', items: { type: 'string' } },
    missingInboundTags: { type: 'array', items: { type: 'string' } },
    missingPresetNames: { type: 'array', items: { type: 'string' } },
  },
} as const;

const msgSchema = (obj: Record<string, unknown>) => ({
  type: 'object',
  required: ['success', 'obj'],
  properties: {
    success: { type: 'boolean' },
    msg: { type: 'string' },
    obj,
  },
});

export const proxyPresetSection: Section = {
  id: 'proxy-presets',
  title: 'Proxy presets',
  description:
    'Reusable Host/endpoint overrides, dry-run preview, node-aware assignment and portable Host/preset bundles.',
  endpoints: [
    {
      method: 'GET',
      path: '/panel/api/hosts/presets/list',
      summary: 'List proxy presets',
      responseObjectSchema: { type: 'array', items: proxyPresetViewSchema },
    },
    {
      method: 'GET',
      path: '/panel/api/hosts/presets/get/:id',
      summary: 'Get one proxy preset',
      params: [{ name: 'id', in: 'path', type: 'integer', desc: 'Preset id.' }],
      responseObjectSchema: proxyPresetViewSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/save',
      summary: 'Create a proxy preset',
      requestSchema: proxyPresetInputSchema,
      responseObjectSchema: proxyPresetViewSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/update/:id',
      summary: 'Update a proxy preset',
      params: [{ name: 'id', in: 'path', type: 'integer', desc: 'Preset id.' }],
      requestSchema: proxyPresetInputSchema,
      responseObjectSchema: proxyPresetViewSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/del/:id',
      summary: 'Delete a proxy preset',
      params: [{ name: 'id', in: 'path', type: 'integer', desc: 'Preset id.' }],
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/preview',
      summary: 'Preview a preset without saving',
      description:
        'Applies one saved preset or draft config to the selected Host groups in memory and returns effective values, changed fields and warnings. No database changes are made.',
      requestSchema: {
        type: 'object',
        required: ['groupIds'],
        properties: {
          groupIds: { type: 'array', minItems: 1, items: { type: 'string' } },
          presetId: { type: 'integer', minimum: 1 },
          config: proxyPresetConfigSchema,
        },
        oneOf: [{ required: ['presetId'] }, { required: ['config'] }],
      },
      responseObjectSchema: {
        type: 'array',
        items: {
          type: 'object',
          required: ['groupId', 'remark', 'base', 'effective', 'changedFields', 'warnings'],
          properties: {
            groupId: { type: 'string' },
            remark: { type: 'string' },
            base: hostGroupSchema,
            effective: hostGroupSchema,
            changedFields: { type: 'array', items: { type: 'string' } },
            warnings: { type: 'array', items: { type: 'string' } },
          },
        },
      },
    },
    {
      method: 'GET',
      path: '/panel/api/hosts/presets/assignments',
      summary: 'List Host group preset assignments',
      responseObjectSchema: { type: 'array', items: assignmentSchema },
    },
    {
      method: 'GET',
      path: '/panel/api/hosts/presets/assignment/:groupId',
      summary: 'Get a Host group preset assignment',
      params: [{ name: 'groupId', in: 'path', type: 'string', desc: 'Host group id.' }],
      responseObjectSchema: assignmentSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/assign/:groupId',
      summary: 'Assign a preset to one Host group',
      params: [{ name: 'groupId', in: 'path', type: 'string', desc: 'Host group id.' }],
      requestSchema: {
        type: 'object',
        required: ['presetId'],
        properties: { presetId: { type: 'integer', minimum: 1 } },
      },
      responseObjectSchema: assignmentSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/assign/bulk',
      summary: 'Assign or clear a preset for multiple Host groups',
      requestSchema: {
        type: 'object',
        required: ['groupIds'],
        properties: {
          groupIds: { type: 'array', minItems: 1, items: { type: 'string' } },
          presetId: { type: 'integer', minimum: 1, nullable: true },
        },
      },
      responseObjectSchema: { type: 'array', items: assignmentSchema },
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/unassign/:groupId',
      summary: 'Clear a Host group preset assignment',
      params: [{ name: 'groupId', in: 'path', type: 'string', desc: 'Host group id.' }],
    },
    {
      method: 'GET',
      path: '/panel/api/hosts/presets/bundle',
      summary: 'Export portable Host/preset bundle',
      description:
        'Exports inbound references by tag instead of local numeric ids, allowing the bundle to be imported on another panel.',
      responseObjectSchema: proxyBundleSchema,
    },
    {
      method: 'POST',
      path: '/panel/api/hosts/presets/bundle/import',
      summary: 'Preview or import a portable Host/preset bundle',
      requestSchema: {
        type: 'object',
        required: ['bundle', 'dryRun', 'allowMissingInbounds'],
        properties: {
          bundle: proxyBundleSchema,
          dryRun: { type: 'boolean' },
          allowMissingInbounds: { type: 'boolean' },
        },
      },
      responseObjectSchema: importResultSchema,
    },
  ],
};

export const proxyPresetSchemas = {
  ProxyPresetConfig: proxyPresetConfigSchema,
  ProxyPresetInput: proxyPresetInputSchema,
  ProxyPresetView: proxyPresetViewSchema,
  ProxyPresetAssignment: assignmentSchema,
  ProxyBundle: proxyBundleSchema,
  ProxyBundleImportResult: importResultSchema,
  ProxyPresetMessage: msgSchema(proxyPresetViewSchema),
};
