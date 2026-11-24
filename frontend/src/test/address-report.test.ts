import { describe, expect, it } from 'vitest';
import {
  addressFindings,
  compareAddressReports,
  parseAddressReport,
} from '@/pages/settings/address-report';

const stage = (outcome: string, durationMs = 10) => ({ outcome, durationMs });
const address = {
  ip: '1.1.1.1',
  tcp: stage('ok'),
  tls: stage('ok'),
  http: stage('ok'),
  httpStatus: 503,
};
const failed = {
  ip: '1.1.1.1',
  tcp: stage('timeout'),
  tls: stage('not-tested', 0),
  http: stage('not-tested', 0),
};
const base = {
  version: 1,
  kind: 'address-connectivity',
  scope: 'client-physical',
  host: 'test.example.com',
  checkedAt: '2026-11-25T10:00:00.123456789Z',
  complete: true,
  address,
};
const parse = (patch = {}) => parseAddressReport(JSON.stringify({ ...base, ...patch }));
const now = Date.parse('2026-11-25T10:01:00Z');

describe('single address report', () => {
  it('distinguishes HTTPS response from application health without DNS claims', () => {
    expect(addressFindings(parse(), now)).toEqual(['reachable', 'http-server-error']);
    expect(addressFindings(parse({ address: failed }), now)).toEqual(['tcp-error']);
    expect(addressFindings(parse({ complete: false, address: undefined }), now)).toEqual([
      'incomplete',
    ]);
  });
  it('rejects malformed and inconsistent reports', () => {
    for (const patch of [
      { version: 2 },
      { kind: 'connectivity' },
      { address: undefined },
      { address: null },
      { address: { ...address, ip: 'not-ip' } },
      { address: { ...address, tcp: stage('timeout') } },
      { address: { ...address, httpStatus: undefined } },
      { checkedAt: 'invalid' },
    ]) {
      expect(() => parse(patch)).toThrow();
    }
  });
  it('compares opposite paths only for matching recent targets', () => {
    const physical = parse({ address: failed });
    const tunnel = parse({ scope: 'client-tunnel' });
    expect(compareAddressReports(physical, tunnel, now)).toBe('tunnel-only');
    expect(compareAddressReports(tunnel, physical, now)).toBe('tunnel-only');
    expect(compareAddressReports(physical, { ...tunnel, host: 'other.example' }, now)).toBe(
      'not-comparable',
    );
    expect(
      compareAddressReports(
        physical,
        parse({ scope: 'client-tunnel', address: { ...address, ip: '8.8.8.8' } }),
        now,
      ),
    ).toBe('not-comparable');
    expect(compareAddressReports(physical, tunnel, now + 86400000 * 2)).toBe('not-comparable');
    expect(compareAddressReports(physical, { ...tunnel, complete: false }, now)).toBe(
      'inconclusive',
    );
  });
});
