import { describe, expect, it } from 'vitest';
import {
  buildManagerSubscriptionUrl,
  getManagerClientState,
  managerDate,
} from '@/pages/manager/subscription-utils';

describe('manager subscription helpers', () => {
  it('respects a configured panel base path and subscription formats', () => {
    expect(buildManagerSubscriptionUrl('abc', 'base64', '/secret/', 'https://panel.test'))
      .toBe('https://panel.test/secret/bus/abc');
    expect(buildManagerSubscriptionUrl('abc', 'singbox', '/secret', 'https://panel.test'))
      .toBe('https://panel.test/secret/bus/abc?format=singbox');
    expect(buildManagerSubscriptionUrl('abc', 'raw', '/', 'https://panel.test'))
      .toBe('https://panel.test/bus/abc?format=raw');
  });

  it('encodes tokens rather than allowing URL path injection', () => {
    expect(buildManagerSubscriptionUrl('x/y z', 'base64', '/', 'https://panel.test'))
      .toBe('https://panel.test/bus/x%2Fy%20z');
  });

  it('marks a disabled, expired and quota-exhausted client correctly', () => {
    const base = { enabled: true, expiryTime: 0, up: 50, down: 50, total: 100 };
    expect(getManagerClientState(undefined, 1_000)).toBe('active');
    expect(getManagerClientState(base, 1_000)).toBe('quota');
    expect(getManagerClientState({ ...base, total: 0 }, 1_000)).toBe('active');
    expect(getManagerClientState({ ...base, expiryTime: 1_000, total: 0 }, 1_000))
      .toBe('expired');
    expect(getManagerClientState({ ...base, enabled: false }, 1_000)).toBe('disabled');
  });

  it('avoids showing invalid timestamps', () => {
    expect(managerDate(0)).toBe('—');
    expect(managerDate(Number.NaN)).toBe('—');
  });
});
