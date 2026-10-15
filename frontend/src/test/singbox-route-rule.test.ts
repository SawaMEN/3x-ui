import { describe, expect, it } from 'vitest';
import { prepareRouteRule, routePorts, routeRuleSummary } from '@/pages/singbox/route-rule';

describe('native route rule editor', () => {
  it('preserves existing port ranges while editing a rule', () => {
    const rule = {
      outbound: 'proxy',
      port: [443],
      port_range: ['1000:2000'],
      source_port_range: [':53'],
      custom: true,
    };
    expect(routePorts(rule)).toBe('443, 1000:2000');
    expect(prepareRouteRule(rule)).toEqual({ ...rule, action: 'route' });
  });
  it('converts pasted Xray port lists into native ports and ranges', () => {
    expect(
      prepareRouteRule({ outbound: 'proxy', port: '80, 443, 1000-2000', source_port: '53' }),
    ).toEqual({
      action: 'route',
      outbound: 'proxy',
      port: [80, 443],
      port_range: ['1000:2000'],
      source_port: [53],
    });
  });
  it('rejects invalid ports and missing destinations', () => {
    for (const port of ['65536', '0', '2000:1000', 'abc', ':']) {
      expect(() => prepareRouteRule({ outbound: 'proxy', port })).toThrow();
    }
    expect(() => prepareRouteRule({ domain: ['example.com'] })).toThrow();
  });
  it('removes incompatible outbound fields from non-route actions', () => {
    expect(prepareRouteRule({ action: 'reject', outbound: 'proxy' })).toEqual({ action: 'reject' });
  });
  it('shows conditions that previously looked like any traffic', () => {
    const summary = routeRuleSummary({
      domain_keyword: ['example'],
      port: [443],
      source_port_range: ['1000:2000'],
      invert: true,
    });
    expect(summary).toContain('example');
    expect(summary).toContain('443');
    expect(summary).toContain('1000:2000');
    expect(summary).toMatch(/^НЕ/);
    expect(routeRuleSummary({ network: 'udp' })).toContain('udp');
  });
});
