import { describe, expect, it } from 'vitest';
import { managedNodeRoutes } from '@/pages/xray/outbounds/managed-node-routes';

describe('managedNodeRoutes', () => {
  it('shows each referenced Xray cascade once, with the number of active rules', () => {
    expect(
      managedNodeRoutes(
        [
          { outboundTag: 'direct' },
          { outboundTag: 'node-route-42', type: 'field' },
          { outboundTag: 'node-route-42', type: 'field' },
          { outboundTag: 'node-route-7', enabled: false },
          { outboundTag: 'node-route-3', type: 'field' },
        ],
        'xray',
      ),
    ).toEqual([
      { tag: 'node-route-42', ruleCount: 2 },
      { tag: 'node-route-3', ruleCount: 1 },
    ]);
  });

  it('recognizes native destinations, ignores disabled rules and rejects unsafe tags', () => {
    expect(
      managedNodeRoutes(
        [
          { outbound: 'node-route-36', action: 'route' },
          { outbound: 'node-route-36', action: 'reject' },
          { outbound: 'node-route-01', action: 'route' },
          { outbound: 'node-route-999999999999999999999999999999', action: 'route' },
          { outbound: 'node-route-0', action: 'route' },
          { outbound: 'node-route-12', enabled: false },
          { outbound: 'node-route-5', action: 'route' },
        ],
        'sing-box',
      ),
    ).toEqual([
      { tag: 'node-route-36', ruleCount: 1 },
      { tag: 'node-route-5', ruleCount: 1 },
    ]);
  });
});
