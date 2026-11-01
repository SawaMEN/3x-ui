import { describe, expect, it } from 'vitest';

import { Base64, TimeFormatter } from '@/utils';

describe('Base64 large UTF-8 input', () => {
  it('encodes beyond the JavaScript function argument limit', () => {
    const source = 'профиль 🌍\0'.repeat(30000);
    expect(Base64.decode(Base64.encode(source))).toBe(source);
    expect(Base64.decode(Base64.encode(source, true))).toBe(source);
  });

  it('preserves empty input and normal Unicode links', () => {
    for (const source of ['', 'vless://id@host:443#Москва']) {
      expect(Base64.decode(Base64.encode(source))).toBe(source);
    }
  });
});

describe('TimeFormatter rounding', () => {
  it('carries a rounded 24 hours into the next day', () => {
    expect(TimeFormatter.formatSecond(2 * 86400 - 1)).toBe('2d');
    expect(TimeFormatter.formatSecond(3 * 86400 - 1)).toBe('3d');
    expect(TimeFormatter.formatSecond(86400)).toBe('1d');
    expect(TimeFormatter.formatSecond(90000)).toBe('1d 1h');
  });
});
