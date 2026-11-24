import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { parseClientDiagnostic } from '@/pages/settings/vetroff-diagnostic-report';
import {
  parseConnectivityReport,
  connectivityFindings,
} from '@/pages/settings/connectivity-report';
import {
  parseAddressReport,
  addressFindings,
  compareAddressReports,
} from '@/pages/settings/address-report';
import {
  DEFAULT_ADAPTIVE_POLICY,
  exportAdaptivePolicy,
} from '@/pages/settings/adaptive-connect-policy';

const dir = process.env.VETROFF_CONTRACT_DIR;
const now = Date.parse('2026-11-25T10:01:00Z');
const read = (name: string) => {
  if (!dir) throw new Error('VETROFF_CONTRACT_DIR is required');
  return readFileSync(join(dir, name), 'utf8');
};

describe.skipIf(!dir)('real VetrOFF-Core v1 JSON contract', () => {
  it.each(['go-standard', 'utls-chrome-auto', 'utls-firefox-auto', 'utls-safari-auto'])(
    'imports Go-serialized %s report',
    (fingerprint) => {
      const report = parseClientDiagnostic(read('siberian-' + fingerprint + '.json'));
      expect(report.fingerprint).toBe(fingerprint);
      expect(report.evidence).toBe('asymmetry');
      expect(report.checkedAt).toBe('2026-11-25T10:00:00.123456789Z');
    },
  );
  it('imports nil slices and omitted control from a partial Go report', () => {
    const report = parseClientDiagnostic(read('siberian-partial.json'));
    expect(report.baseline).toEqual([]);
    expect(report.burst).toEqual([]);
    expect(report.control).toBeUndefined();
    expect(report.evidence).toBe('inconclusive');
  });
  it('imports Go connectivity reports and distinguishes HTTP status from network reachability', () => {
    const report = parseConnectivityReport(read('connectivity.json'));
    expect(connectivityFindings(report, now)).toEqual(
      expect.arrayContaining(['reachable', 'http-server-error', 'no-address']),
    );
    expect(
      connectivityFindings(parseConnectivityReport(read('connectivity-partial.json')), now),
    ).toEqual(['incomplete']);
  });
  it('compares address reports without DNS assumptions', () => {
    const physical = parseAddressReport(read('address-physical.json'));
    const tunnel = parseAddressReport(read('address-tunnel.json'));
    expect(compareAddressReports(physical, tunnel, now)).toBe('tunnel-only');
    expect(addressFindings(tunnel, now)).toEqual(['reachable', 'http-server-error']);
    expect(addressFindings(parseAddressReport(read('address-partial.json')), now)).toEqual([
      'incomplete',
    ]);
  });
  it('exports actual panel policies for the strict Go parser', () => {
    expect(JSON.parse(exportAdaptivePolicy({ ...DEFAULT_ADAPTIVE_POLICY }))).toEqual(
      JSON.parse(read('core-policy.json')),
    );
    const enabled = {
      ...DEFAULT_ADAPTIVE_POLICY,
      enabled: true,
      failureThreshold: 10,
      recoveryThreshold: 1,
      cooldownSeconds: 0,
      maxAgeSeconds: 600,
      failback: true,
    };
    const output = dir!;
    writeFileSync(
      join(output, 'panel-default.json'),
      exportAdaptivePolicy({ ...DEFAULT_ADAPTIVE_POLICY }),
      { mode: 0o600, flag: 'wx' },
    );
    writeFileSync(join(output, 'panel-enabled.json'), exportAdaptivePolicy(enabled), {
      mode: 0o600,
      flag: 'wx',
    });
  });
});
