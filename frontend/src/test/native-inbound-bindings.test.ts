import { describe, expect, it } from 'vitest';
import { object, records } from '@/pages/singbox/simple-settings';
import {
  nativeInboundRouteTarget,
  setNativeInboundRouteTarget,
} from '@/pages/singbox/native-inbound-bindings';
const config = {
  route: {
    rules: [
      { action: 'reject', domain_suffix: ['ads.test'] },
      { action: 'route', outbound: 'direct' },
    ],
  },
};
describe('native sing-box per-inbound routing', () => {
  it('creates independent inbound routes before default catch-all', () => {
    const one = setNativeInboundRouteTarget(config, 'in-1', 'warp');
    const two = setNativeInboundRouteTarget(one, 'in-2', 'vetroff');
    expect(records(object(two.route).rules).map((row) => row.outbound)).toEqual([
      undefined,
      'warp',
      'vetroff',
      'direct',
    ]);
    expect(nativeInboundRouteTarget(two, 'in-1')).toBe('warp');
    expect(nativeInboundRouteTarget(two, 'in-2')).toBe('vetroff');
    const removed = setNativeInboundRouteTarget(two, 'in-1', '');
    expect(nativeInboundRouteTarget(removed, 'in-1')).toBe('');
    expect(nativeInboundRouteTarget(removed, 'in-2')).toBe('vetroff');
  });
  it('repairs a stale binding shadowed by a native catch-all', () => {
    const stale = {
      route: {
        rules: [
          { action: 'route', outbound: 'direct' },
          { action: 'route', comment: 'panel-inbound:in-1', inbound: ['in-1'], outbound: 'old' },
        ],
      },
    };
    const repaired = setNativeInboundRouteTarget(stale, 'in-1', 'node-route-12');
    expect(records(object(repaired.route).rules).map((rule) => rule.outbound)).toEqual([
      'node-route-12',
      'direct',
    ]);
    expect(nativeInboundRouteTarget(repaired, 'in-1')).toBe('node-route-12');
  });

  it('keeps its position when changed', () => {
    const changed = setNativeInboundRouteTarget(
      setNativeInboundRouteTarget(config, 'one', 'a'),
      'one',
      'b',
    );
    expect(records(object(changed.route).rules)[1]).toMatchObject({
      inbound: ['one'],
      outbound: 'b',
    });
  });
});
