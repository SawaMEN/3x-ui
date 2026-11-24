import { describe, expect, it } from 'vitest';
import {
  DEFAULT_ADAPTIVE_POLICY,
  exportAdaptivePolicy,
} from '@/pages/settings/adaptive-connect-policy';

describe('Adaptive Connect policy v1', () => {
  it('exports the disabled canonical policy compatible with VetrOFF-Core', () => {
    expect(exportAdaptivePolicy({ ...DEFAULT_ADAPTIVE_POLICY })).toBe(
      '{\n  "version": 1,\n  "enabled": false,\n  "failureThreshold": 2,\n  "recoveryThreshold": 3,\n  "cooldownSeconds": 30,\n  "maxAgeSeconds": 90,\n  "failback": false\n}\n',
    );
  });
  it('rejects invalid limits', () => {
    for (const patch of [
      { failureThreshold: 0 },
      { recoveryThreshold: 11 },
      { cooldownSeconds: -1 },
      { maxAgeSeconds: 601 },
      { maxAgeSeconds: 4 },
      { failureThreshold: 1.5 },
    ])
      expect(() => exportAdaptivePolicy({ ...DEFAULT_ADAPTIVE_POLICY, ...patch })).toThrow();
  });
  it('excludes unrelated data', () => {
    const input = {
      ...DEFAULT_ADAPTIVE_POLICY,
      enabled: true,
      token: 'secret',
      fallback: 'direct',
    };
    const parsed = JSON.parse(exportAdaptivePolicy(input));
    expect(parsed.enabled).toBe(true);
    expect(parsed).not.toHaveProperty('token');
    expect(parsed).not.toHaveProperty('fallback');
  });
});
