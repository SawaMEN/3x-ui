import { describe, expect, it } from 'vitest';
import { TLS_CIPHER_OPTION, TLS_LEGACY_CIPHER_OPTION } from '@/schemas/primitives';

describe('explicit legacy TLS cipher options', () => {
  it('exposes RSA GCM suite from the reported core failure', () => {
    expect(Object.values(TLS_LEGACY_CIPHER_OPTION)).toContain('TLS_RSA_WITH_AES_128_GCM_SHA256');
  });

  it('keeps unsafe suites separate from secure defaults and has no duplicates', () => {
    const legacy = Object.values(TLS_LEGACY_CIPHER_OPTION);
    const modern = Object.values(TLS_CIPHER_OPTION);
    expect(legacy.length).toBe(12);
    expect(new Set(legacy).size).toBe(legacy.length);
    for (const suite of legacy) {
      expect(modern).not.toContain(suite);
    }
  });
});
