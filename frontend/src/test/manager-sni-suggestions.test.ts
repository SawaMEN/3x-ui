import { describe, expect, it } from 'vitest';
import type { RealityScanResult } from '@/generated/types';
import {
  bestManagerAutoSNI,
  isAutoUsableManagerSNI,
  rankManagerSNISuggestions,
} from '@/pages/manager/sni-suggestions';

function result(values: Partial<RealityScanResult> = {}): RealityScanResult {
  return {
    alpn: 'h2',
    certChainBytes: 4000,
    certChainValid: true,
    certIssuer: 'Test CA',
    certSubject: 'example.org',
    certValid: true,
    curveID: 'X25519',
    feasible: true,
    h2: true,
    host: 'example.org',
    ip: '198.51.100.20',
    latencyMs: 80,
    notAfter: '2030-01-01T00:00:00Z',
    port: 443,
    privateTarget: false,
    reason: '',
    serverNames: ['example.org'],
    target: '198.51.100.20:443',
    tls13: true,
    tlsVersion: 'TLS1.3',
    x25519: true,
    dnsMatchesIP: true,
    attempts: 5,
    successes: 5,
    medianMs: 80,
    ...values,
  };
}

describe('manager automatic REALITY SNI selection', () => {
  it('prefers a stable DNS-matching site over a faster but mismatched neighbor', () => {
    const safe = result({ host: 'safe.example.org', medianMs: 120, latencyMs: 120 });
    const neighbor = result({
      host: 'neighbor.example.org',
      dnsMatchesIP: false,
      medianMs: 12,
      latencyMs: 12,
      source: 'neighbors',
    });
    expect(bestManagerAutoSNI([neighbor, safe])).toBe('safe.example.org');
    expect(rankManagerSNISuggestions([neighbor, safe])[0].host).toBe('safe.example.org');
  });

  it('never uses an unrelated hostname from a neighboring IP when DNS differs', () => {
    const neighbor = result({
      host: 'neighbor.example.org',
      source: 'neighbors',
      dnsMatchesIP: false,
    });
    expect(isAutoUsableManagerSNI(neighbor)).toBe(false);
    expect(bestManagerAutoSNI([neighbor])).toBeNull();
    expect(rankManagerSNISuggestions([neighbor])).toHaveLength(1);
  });

  it('rejects stale, unstable or unsafe automatic candidates', () => {
    for (const changed of [
      { dnsMatchesIP: null },
      { attempts: 5, successes: 3 },
      { attempts: 0, successes: 0 },
      { tls13: false },
      { x25519: false },
      { h2: false },
      { privateTarget: true },
      { certValid: false },
    ]) {
      expect(isAutoUsableManagerSNI(result(changed))).toBe(false);
    }
  });

  it('ranks without changing the original scan history order', () => {
    const reliable = result({ host: 'reliable.example.org', successes: 5 });
    const unstable = result({ host: 'unstable.example.org', successes: 1 });
    const original = [unstable, reliable];
    expect(rankManagerSNISuggestions(original).map((row) => row.host)).toEqual([
      'reliable.example.org',
      'unstable.example.org',
    ]);
    expect(original[0].host).toBe('unstable.example.org');
  });

  it('sorts stable results by reliability before latency', () => {
    const reliable = result({ host: 'reliable.example.org', latencyMs: 100, successes: 5 });
    const fast = result({ host: 'fast.example.org', latencyMs: 15, medianMs: 15, successes: 4 });
    expect(rankManagerSNISuggestions([fast, reliable])[0].host).toBe('reliable.example.org');
  });
});
