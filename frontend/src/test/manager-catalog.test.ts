import { describe, expect, it } from 'vitest';
import { buildManagerRecipes, managerCatalog, MAX_MANAGER_ENTRIES } from '@/pages/manager/catalog';
import type { NodeRecord } from '@/schemas/node';

describe('manager multi-node recipe builder', () => {
  const node: NodeRecord = {
    id: 7,
    name: 'Europe',
    enable: true,
    status: 'online',
    coreType: 'xray',
    protocolAvailability: {
      vless: { supported: true, ready: true, version: 'test', speedLimit: true, component: '' },
      trojan: { supported: true, ready: false, version: 'test', speedLimit: true, component: '' },
    },
  };

  it('shows a single categorized catalog including native and Xray protocols', () => {
    const ids = new Set(managerCatalog.map((preset) => preset.id));
    expect(ids.has('vless-tcp-reality')).toBe(true);
    expect(ids.has('vless-xhttp-reality')).toBe(true);
    expect(ids.has('hysteria2-udp')).toBe(true);
    expect(ids.has('naive')).toBe(true);
  });

  it('generates only supported node-protocol combinations', () => {
    const result = buildManagerRecipes({
      presets: ['vless-tcp-reality', 'trojan-tcp-reality'],
      nodeIds: [0, 7],
      nodes: [node],
      domains: ['local.example.org'],
      sniByPreset: { 'vless-tcp-reality': 'auto', 'trojan-tcp-reality': 'sni.example.org' },
      targetByPreset: {},
      verifiedSniByNode: { 7: 'verified.example.org' },
    });
    expect(result.tooMany).toBe(false);
    expect(result.entries).toHaveLength(3);
    expect(result.skipped).toHaveLength(1);
    expect(result.entries[0]).toMatchObject({
      preset: 'vless-tcp-reality',
      nodeId: 0,
      sni: 'random',
      config: { protocol: 'vless' },
    });
    expect(result.entries[2]).toMatchObject({
      preset: 'vless-tcp-reality',
      nodeId: 7,
      sni: 'verified.example.org',
    });
  });

  it('uses camouflage hostname as default SNI instead of an unrelated random domain', () => {
    const result = buildManagerRecipes({
      presets: ['vless-tcp-reality'],
      nodeIds: [0],
      nodes: [],
      domains: ['unrelated.example.org'],
      sniByPreset: { 'vless-tcp-reality': 'auto' },
      targetByPreset: { 'vless-tcp-reality': 'front.example.org:8443' },
      verifiedSniByNode: {},
    });
    expect(result.entries[0]).toMatchObject({
      sni: 'front.example.org',
      realityTarget: 'front.example.org:8443',
    });
  });

  it('keeps separately selected verified backups only for REALITY without a pinned target', () => {
    const opts = {
      presets: ['vless-tcp-reality'],
      nodeIds: [0],
      nodes: [],
      domains: [],
      sniByPreset: { 'vless-tcp-reality': 'primary.example.org' },
      verifiedSniByNode: {},
      backupSniByNode: { 0: ['primary.example.org', 'backup.example.org'] },
      targetByPreset: {},
    };
    const choices = buildManagerRecipes(opts);
    expect(choices.entries[0]).toMatchObject({
      sni: 'primary.example.org',
      sniBackups: ['backup.example.org'],
    });
    const fixed = buildManagerRecipes({
      ...opts,
      targetByPreset: { 'vless-tcp-reality': 'front.example.org:443' },
    });
    expect(fixed.entries[0].sniBackups).toBeUndefined();
  });

  it('rejects oversized plans instead of silently dropping entries', () => {
    const result = buildManagerRecipes({
      presets: ['vless-tcp-reality'],
      nodeIds: Array.from({ length: MAX_MANAGER_ENTRIES + 1 }, () => 0),
      nodes: [],
      domains: ['example.org'],
      sniByPreset: {},
      targetByPreset: {},
      verifiedSniByNode: {},
    });
    expect(result.tooMany).toBe(true);
    expect(result.entries).toHaveLength(0);
  });
});
