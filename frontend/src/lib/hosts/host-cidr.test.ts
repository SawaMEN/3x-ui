import { describe, expect, it, vi } from 'vitest';

import { isCIDR, randomIPsFromCIDR } from './host-cidr';

describe('host CIDR', () => {
  it('expands IPv4-mapped IPv6 using a 128-bit prefix', () => {
    expect(isCIDR('[::ffff:192.0.2.14]/127')).toBe(true);
    expect(randomIPsFromCIDR('::ffff:192.0.2.14/127', 2)).toEqual([
      '::ffff:192.0.2.14',
      '::ffff:192.0.2.15',
    ]);
  });

  it.each(['[2001:db8::/64', '2001:db8::]/64', '192.0.02.1/24', '2001:db8:: 192.0.2.1/64'])(
    'rejects invalid network %s',
    (input) => expect(isCIDR(input)).toBe(false),
  );

  it('returns the exact available count even if the RNG repeats', () => {
    const random = vi
      .spyOn(globalThis.crypto, 'getRandomValues')
      .mockImplementation((buffer) => buffer);
    try {
      expect(randomIPsFromCIDR('192.0.2.0/30', 8).sort()).toEqual(['192.0.2.1', '192.0.2.2']);
      expect(new Set(randomIPsFromCIDR('2001:db8::/121', 100)).size).toBe(100);
    } finally {
      random.mockRestore();
    }
  });

  it('normalizes invalid and fractional counts', () => {
    expect(randomIPsFromCIDR('192.0.2.0/24', Number.NaN)).toHaveLength(1);
    expect(randomIPsFromCIDR('192.0.2.0/24', 2.5)).toHaveLength(2);
  });
});
