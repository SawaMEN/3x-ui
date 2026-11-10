import type { RealityScanResult } from '@/generated/types';

// A neighboring IP may serve a perfectly valid certificate for a hostname
// that resolves elsewhere. We can show that discovery to the administrator,
// but must not silently substitute the hostname into a DNS-based REALITY
// target: doing so would point the inbound at an untested destination.
export function isAutoUsableManagerSNI(row: RealityScanResult): boolean {
  return (
    !!row.host &&
    row.feasible &&
    row.certValid &&
    row.tls13 &&
    row.h2 &&
    row.x25519 &&
    !row.privateTarget &&
    row.dnsMatchesIP === true &&
    (row.attempts ?? 0) >= 4 &&
    (row.successes ?? 0) * 5 >= (row.attempts ?? 0) * 4
  );
}

function resultLatency(row: RealityScanResult): number {
  if ((row.attempts ?? 0) >= 4 && (row.medianMs ?? 0) > 0) return row.medianMs!;
  return row.latencyMs > 0 ? row.latencyMs : Number.POSITIVE_INFINITY;
}

// Rank by DNS safety, repeated-probe reliability, and finally latency.
// Do not confuse a successful TLS probe from a server with a proof that a
// client in another operator/network can reach the VPN endpoint.
export function rankManagerSNISuggestions(rows: RealityScanResult[]): RealityScanResult[] {
  return rows
    .filter((row) => row.host && row.certValid && row.feasible && !row.privateTarget)
    .sort((a, b) => {
      const readyA = isAutoUsableManagerSNI(a);
      const readyB = isAutoUsableManagerSNI(b);
      if (readyA !== readyB) return readyA ? -1 : 1;
      const stableA = (a.attempts ?? 0) ? (a.successes ?? 0) / a.attempts! : 0;
      const stableB = (b.attempts ?? 0) ? (b.successes ?? 0) / b.attempts! : 0;
      if (stableA !== stableB) return stableB - stableA;
      const latency = resultLatency(a) - resultLatency(b);
      if (latency) return latency;
      return a.host.localeCompare(b.host);
    });
}

export function bestManagerAutoSNI(rows: RealityScanResult[]): string | null {
  return rankManagerSNISuggestions(rows).find(isAutoUsableManagerSNI)?.host ?? null;
}
