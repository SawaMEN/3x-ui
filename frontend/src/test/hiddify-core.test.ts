import { describe, expect, it } from 'vitest';

import { nodeCoreType, nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { Protocols } from '@/schemas/primitives';

describe('hiddify core selection', () => {
  it('retains hiddify identity reported by the node', () => {
    expect(nodeCoreType({ coreType: 'hiddify-core', runningCore: 'xray' })).toBe('hiddifycore');
    expect(nodeCoreType({ runningCore: 'hiddify-core' })).toBe('hiddifycore');
  });
  it('uses native core protocol eligibility', () => {
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.SNELL)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.NAIVE)).toBe(true);
    expect(nodeSupportsProtocol({ coreType: 'hiddify-core' }, Protocols.WIREGUARD)).toBe(false);
  });
});
