import { describe, expect, it } from 'vitest';
import { singBoxHealthIssues } from '@/pages/singbox/config-health';

describe('sing-box outbound health checks', () => {
  it('accepts a managed node route materialized at runtime', () => {
    const config = {
      outbounds: [{ tag: 'direct', type: 'direct' }],
      route: {
        rules: [
          { outbound: 'direct' },
          { outbound: 'direct' },
          { action: 'reject' },
          { action: 'route', outbound: 'node-route-36' },
        ],
      },
    };
    expect(singBoxHealthIssues(config)).toEqual([]);
  });

  it('still reports missing ordinary outbounds and malformed managed route tags', () => {
    const config = {
      outbounds: [{ tag: 'direct', type: 'direct' }],
      route: {
        rules: [
          { outbound: 'missing-proxy' },
          { outbound: 'node-route-0' },
          { outbound: 'node-route-036' },
          { outbound: 'node-route-36x' },
          { outbound: 'node-route-9999999999999999999999999' },
        ],
      },
    };
    const issues = singBoxHealthIssues(config);
    expect(issues).toHaveLength(5);
    for (const tag of [
      'missing-proxy', 'node-route-0', 'node-route-036', 'node-route-36x',
      'node-route-9999999999999999999999999',
    ]) {
      expect(issues.some((issue) => issue.includes('"' + tag + '"'))).toBe(true);
    }
  });

  it('does not treat a node-route tag as generated in selector or route.final', () => {
    const config = {
      outbounds: [{ tag: 'choice', type: 'selector', outbounds: ['node-route-36'] }],
      route: { final: 'node-route-36', rules: [{ outbound: 'node-route-36' }] },
    };
    const issues = singBoxHealthIssues(config);
    expect(issues).toHaveLength(2);
    expect(issues.some((issue) => issue.includes('selector "choice"'))).toBe(true);
    expect(issues.some((issue) => issue.includes('route.final'))).toBe(true);
  });

  it('keeps the original duplicate, invalid endpoint, and disabled-rule behavior', () => {
    const config = {
      outbounds: [
        { tag: 'duplicate', type: 'vless' },
        { tag: 'duplicate', type: 'direct' },
      ],
      route: { rules: [{ outbound: 'missing', enabled: false }] },
    };
    const issues = singBoxHealthIssues(config);
    expect(issues).toHaveLength(4);
    expect(issues.some((issue) => issue.includes('Дублирующийся outbound tag'))).toBe(true);
    expect(issues.some((issue) => issue.includes('не указан сервер'))).toBe(true);
    expect(issues.some((issue) => issue.includes('не указан порт'))).toBe(true);
    expect(issues.some((issue) => issue.includes('не указан UUID'))).toBe(true);
  });
});
