import { describe, expect, it } from 'vitest';
import {
  TLS_CIPHER_OPTION,
  TLS_CIPHER_PRESETS,
  TLS_LEGACY_CIPHER_OPTION,
} from '@/schemas/primitives';

describe('explicit legacy TLS cipher options', () => {
  it('exposes RSA GCM suite from the reported core failure', () => {
    expect(Object.values(TLS_LEGACY_CIPHER_OPTION)).toContain('TLS_RSA_WITH_AES_128_GCM_SHA256');
  });

  it('never offers a preset that the pinned TLS core rejects', () => {
    const supported = new Set<string>(Object.values(TLS_CIPHER_OPTION));
    const obsolete = new Set<string>(Object.values(TLS_LEGACY_CIPHER_OPTION));
    for (const [name, preset] of Object.entries(TLS_CIPHER_PRESETS)) {
      for (const suite of preset.split(':')) {
        expect(supported.has(suite), `${name} offers unsupported ${suite}`).toBe(true);
        expect(obsolete.has(suite), `${name} contains obsolete ${suite}`).toBe(false);
      }
    }
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
