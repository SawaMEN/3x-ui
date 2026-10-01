import { describe, expect, it } from 'vitest';

import { ProxyBundleSchema, ProxyPresetConfigSchema } from '@/schemas/api/proxyPreset';

describe('ProxyPresetConfigSchema', () => {
  it('keeps omitted fields distinct from explicit zero values', () => {
    const inherited = ProxyPresetConfigSchema.parse({ security: 'tls' });
    expect(inherited.port).toBeUndefined();
    expect(inherited.allowInsecure).toBeUndefined();

    const explicit = ProxyPresetConfigSchema.parse({ port: 0, allowInsecure: false, sni: '' });
    expect(explicit.port).toBe(0);
    expect(explicit.allowInsecure).toBe(false);
    expect(explicit.sni).toBe('');
  });

  it('rejects invalid reusable host settings', () => {
    expect(() => ProxyPresetConfigSchema.parse({ port: 70000 })).toThrow();
    expect(() => ProxyPresetConfigSchema.parse({ security: 'bogus' })).toThrow();
  });
});

describe('ProxyBundleSchema', () => {
  it('accepts a portable host without a local group id', () => {
    const bundle = ProxyBundleSchema.parse({
      version: 1,
      exportedAt: 0,
      presets: [],
      hosts: [
        {
          inboundTags: ['in-vless-443'],
          hosts: ['edge.example.com:443'],
          remark: 'edge',
          security: 'tls',
        },
      ],
    });

    expect(bundle.hosts[0].groupId).toBe('');
    expect(bundle.hosts[0].inboundTags).toEqual(['in-vless-443']);
  });

  it('rejects bundles with unsupported versions', () => {
    expect(() =>
      ProxyBundleSchema.parse({ version: 2, exportedAt: 0, presets: [], hosts: [] }),
    ).toThrow();
  });
});
