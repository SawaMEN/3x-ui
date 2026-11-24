import { describe, expect, it } from 'vitest';
import {
  parseConnectivityReport,
  connectivityFindings,
} from '@/pages/settings/connectivity-report';

const stage = (outcome: string, durationMs = 10) => ({ outcome, durationMs });
const address = {
  ip: '1.1.1.1',
  tcp: stage('ok'),
  tls: stage('ok'),
  http: stage('ok'),
  httpStatus: 503,
};
const ipv4 = {
  family: 'ip4',
  dns: stage('ok'),
  resolved: 1,
  truncated: false,
  addresses: [address],
};
const ipv6 = {
  family: 'ip6',
  dns: stage('no-address'),
  resolved: 0,
  truncated: false,
  addresses: [],
};
const base = {
  version: 1,
  kind: 'connectivity',
  scope: 'client-physical',
  host: 'test.example.com',
  checkedAt: '2026-11-25T10:00:00Z',
  complete: true,
  families: [ipv4, ipv6],
};
const parse = (patch = {}) => parseConnectivityReport(JSON.stringify({ ...base, ...patch }));
const now = Date.parse('2026-11-25T10:01:00Z');
describe('connectivity report v1', () => {
  it('distinguishes HTTP response from application health and absent AAAA', () => {
    expect(connectivityFindings(parse(), now)).toEqual(
      expect.arrayContaining(['reachable', 'http-server-error', 'no-address']),
    );
    expect(connectivityFindings(parse(), now)).not.toContain('family-difference');
  });
  it('rejects contradictory stages and invalid reports', () => {
    for (const patch of [
      { version: 2 },
      { families: [ipv4, ipv4] },
      { families: [ipv4] },
      { families: [{ ...ipv4, resolved: 2 }, ipv6] },
      { families: [{ ...ipv4, addresses: [{ ...address, tcp: stage('timeout') }] }, ipv6] },
      { families: [{ ...ipv4, addresses: [{ ...address, httpStatus: undefined }] }, ipv6] },
      { families: [{ ...ipv4, addresses: [{ ...address, ip: '::1' }] }, ipv6] },
    ])
      expect(() => parse(patch)).toThrow();
  });
  it('reports certificate failure and incomplete observations', () => {
    const r = parse({
      complete: false,
      families: [
        {
          ...ipv4,
          addresses: [
            {
              ...address,
              tls: stage('certificate-error'),
              http: stage('not-tested', 0),
              httpStatus: undefined,
            },
          ],
        },
      ],
    });
    expect(connectivityFindings(r, now)).toEqual(
      expect.arrayContaining(['incomplete', 'certificate-error']),
    );
    expect(connectivityFindings(r, now)).not.toContain('reachable');
    expect(connectivityFindings(r, now + 48 * 3600000)).toContain('stale');
  });
  it('reports different reachability only when both families have samples', () => {
    const v6 = {
      family: 'ip6',
      dns: stage('ok'),
      resolved: 1,
      truncated: false,
      addresses: [
        {
          ip: '2606:4700:4700::1111',
          tcp: stage('timeout'),
          tls: stage('not-tested', 0),
          http: stage('not-tested', 0),
        },
      ],
    };
    expect(connectivityFindings(parse({ families: [ipv4, v6] }), now)).toEqual(
      expect.arrayContaining(['tcp-error', 'family-difference']),
    );
  });
});
