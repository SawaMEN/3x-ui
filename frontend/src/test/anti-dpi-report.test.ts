import { describe, expect, it } from 'vitest';
import { parseSiberianReport } from '@/pages/settings/anti-dpi-report';
import { getAdditionalSettingsTab } from '@/pages/settings/additional-settings';

function report(alpha: string, beta: string, retried = false) {
  return JSON.stringify({
    ips: [
      {
        ip: '1.1.1.1',
        checks: [
          {
            fingerprint: 'chrome',
            alpha: { error: alpha },
            beta: { error: beta },
            siberian_detected: true,
            retried_with_real_sni: retried,
          },
        ],
      },
    ],
  });
}

describe('Anti-DPI', () => {
  it('opens the advanced tab directly', () => {
    expect(getAdditionalSettingsTab('#advanced/anti-dpi')).toBe('anti-dpi');
  });
  it('distinguishes asymmetry from inconclusive failures', () => {
    expect(parseSiberianReport(report('timeout', ''))[0].evidence).toBe('asymmetry');
    expect(parseSiberianReport(report('timeout', 'timeout'))[0].evidence).toBe('inconclusive');
    expect(parseSiberianReport(report('', 'timeout'))[0].evidence).toBe('inconclusive');
    expect(parseSiberianReport(report('timeout', '', true))[0].evidence).toBe('inconclusive');
    expect(parseSiberianReport(report('', ''))[0].evidence).toBe('no-signal');
  });
  it('rejects malformed input', () => {
    expect(() => parseSiberianReport('{"ips":[]}')).toThrow();
    expect(() => parseSiberianReport('{"ips":[{"ip":"x","checks":[{}]}]}')).toThrow();
  });
});
