import { describe, it, expect } from 'vitest';
import { inboundRouteTarget, setInboundRouteTarget } from '@/pages/xray/routing/inbound-bindings';
import type { RuleObject } from '@/schemas/routing';

describe('Xray inbound to outbound routing', () => {
  const rules: RuleObject[] = [
    { type: 'field', ip: ['geoip:private'], outboundTag: 'blocked' },
    { type: 'field', outboundTag: 'direct' },
  ];
  it('inserts individual inbound routes before a generic catch-all', () => {
    let next = setInboundRouteTarget(rules, 'in-a', 'warp');
    next = setInboundRouteTarget(next, 'in-b', 'vetroff-out');
    expect(next.map((r) => r.outboundTag)).toEqual(['blocked', 'warp', 'vetroff-out', 'direct']);
    expect(inboundRouteTarget(next, 'in-a')).toBe('warp');
    expect(inboundRouteTarget(next, 'in-b')).toBe('vetroff-out');
    expect(rules).toHaveLength(2);
  });
  it('updates and removes only one managed inbound route', () => {
    const original = setInboundRouteTarget(
      setInboundRouteTarget(rules, 'a', 'warp'),
      'b',
      'vetroff',
    );
    const changed = setInboundRouteTarget(original, 'a', 'direct');
    expect(inboundRouteTarget(changed, 'a')).toBe('direct');
    expect(inboundRouteTarget(changed, 'b')).toBe('vetroff');
    expect(setInboundRouteTarget(changed, 'a', '').map((r) => r.outboundTag)).toEqual([
      'blocked',
      'vetroff',
      'direct',
    ]);
  });
  it('appends fallback when no generic catch-all exists', () => {
    const next = setInboundRouteTarget([rules[0]], 'a', 'warp');
    expect(next[1]).toMatchObject({ inboundTag: ['a'], outboundTag: 'warp' });
  });
});
