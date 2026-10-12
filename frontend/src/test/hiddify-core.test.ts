import { describe, expect, it } from 'vitest';

import { nodeCoreType, nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { coreHealth, hasCoreProblem } from '@/lib/node-health';
import { NodeRecordSchema, ProbeResultSchema } from '@/schemas/node';
import { Protocols } from '@/schemas/primitives';

describe('hiddify core selection', () => {
  it('normalizes the legacy owner core name to sing-box', () => {
    expect(nodeCoreType({ coreType: 'hiddify-core', runningCore: 'xray' })).toBe('singbox');
    expect(nodeCoreType({ runningCore: 'hiddify-core' })).toBe('singbox');
  });
  it('uses native core protocol eligibility', () => {
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.SNELL)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.NAIVE)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.WIREGUARD)).toBe(false);
  });
});

describe('remote Hiddify health', () => {
  it('uses selected runtime status and detects mismatches', () => {
    const node = {
      status: 'online',
      coreType: 'hiddify-core',
      coreState: 'stop',
      xrayState: 'running',
    };
    expect(coreHealth(node).configured).toBe('singbox');
    expect(hasCoreProblem(node)).toBe(true);
    expect(
      hasCoreProblem({
        ...node,
        coreState: 'running',
        runningCore: 'hiddify-core',
      }),
    ).toBe(false);
    expect(hasCoreProblem({ ...node, runningCore: 'sing-box' })).toBe(false);
    expect(hasCoreProblem({ ...node, status: 'offline' })).toBe(false);
  });
  it('accepts legacy unknown capabilities without inventing them', () => {
    expect(NodeRecordSchema.parse({ id: 1 }).coreCapabilities).toBeUndefined();
    expect(
      ProbeResultSchema.parse({ status: 'online', coreCapabilities: null }).coreCapabilities,
    ).toBeNull();
  });
  it('offers MASQUE on native nodes only', () => {
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.MASQUE)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'sing-box' }, Protocols.MASQUE)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'xray' }, Protocols.MASQUE)).toBe(false);
  });
});

describe('Hiddify TUN support', () => {
  it('offers translated TUN under the canonical sing-box name', () => {
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.TUN)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'sing-box' }, Protocols.TUN)).toBe(true);
  });
});
