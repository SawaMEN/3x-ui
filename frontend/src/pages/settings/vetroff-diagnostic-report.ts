import type { Evidence } from './anti-dpi-report';

export type DiagnosticScope = 'client-physical' | 'client-tunnel';
type Stage = 'tcp' | 'tls';
type Outcome = 'ok' | 'error' | 'timeout' | 'canceled';
export interface DiagnosticAttempt {
  stage: Stage;
  outcome: Outcome;
  durationMs: number;
}
export interface ClientDiagnostic {
  scope: DiagnosticScope;
  host: string;
  controlHost: string;
  ip: string;
  fingerprint: string;
  checkedAt: string;
  baseline: DiagnosticAttempt[];
  burst: DiagnosticAttempt[];
  control?: DiagnosticAttempt;
  evidence: Evidence;
}

function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('Invalid report object');
  return value as Record<string, unknown>;
}
function hostname(value: unknown): string {
  if (typeof value !== 'string' || value.length > 253 ||
      !value.includes('.') || value.split('.').some((s) => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(s)) ||
      /^\d+(?:\.\d+){3}$/.test(value)) throw new Error('Invalid hostname');
  return value;
}
function attempt(value: unknown): DiagnosticAttempt {
  const a = object(value);
  if ((a.stage !== 'tcp' && a.stage !== 'tls') ||
      (a.outcome !== 'ok' && a.outcome !== 'error' && a.outcome !== 'timeout' && a.outcome !== 'canceled') ||
      (a.stage === 'tcp' && a.outcome === 'ok') ||
      typeof a.durationMs !== 'number' || !Number.isSafeInteger(a.durationMs) ||
      a.durationMs < 0 || a.durationMs > 30000) throw new Error('Invalid diagnostic attempt');
  return {stage: a.stage, outcome: a.outcome, durationMs: a.durationMs};
}
function attempts(value: unknown, max: number): DiagnosticAttempt[] {
  if (value === null) return [];
  if (!Array.isArray(value) || value.length > max) throw new Error('Invalid diagnostic sequence');
  return value.map(attempt);
}
function classify(baseline: DiagnosticAttempt[], burst: DiagnosticAttempt[], control?: DiagnosticAttempt): Evidence {
  const passed = (a: DiagnosticAttempt) => a.stage === 'tls' && a.outcome === 'ok';
  if (baseline.length !== 2 || !baseline.every(passed) || burst.length !== 4 ||
      !control || !passed(control) ||
      burst.some((a) => a.stage !== 'tls' || (a.outcome !== 'ok' && a.outcome !== 'timeout'))) return 'inconclusive';
  return burst.some((a) => a.outcome === 'timeout') ? 'asymmetry' : 'no-signal';
}

// Imported data is untrusted. Recalculate evidence; never accept the declared
// result as a routing instruction or as proof of operator filtering.
export function parseClientDiagnostic(raw: string): ClientDiagnostic {
  if (raw.length > 65536) throw new Error('Report too large');
  const r = object(JSON.parse(raw));
  if (r.version !== 1 || (r.scope !== 'client-physical' && r.scope !== 'client-tunnel') ||
      typeof r.checkedAt !== 'string' || r.checkedAt.length > 40 ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(r.checkedAt) ||
      !Number.isFinite(Date.parse(r.checkedAt)) ||
      r.fingerprint !== 'go-standard' ||
      typeof r.ip !== 'string' || r.ip.length > 45 ||
      (r.ip !== '' && !validIP(r.ip))) throw new Error('Unsupported client report');
  const host = hostname(r.host), controlHost = hostname(r.controlHost);
  if (host === controlHost) throw new Error('Identical test names');
  const baseline = attempts(r.baseline, 2);
  const burst = attempts(r.burst, 4);
  const control = r.control === undefined ? undefined : attempt(r.control);
  if (!r.ip && (baseline.length || burst.length || control)) throw new Error('Missing test IP');
  return { scope: r.scope, host, controlHost, ip: r.ip, fingerprint: r.fingerprint,
    checkedAt: r.checkedAt, baseline, burst, control,
    evidence: classify(baseline, burst, control) };
}
function validIP(ip: string): boolean {
  if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(ip)) {
    return ip.split('.').every((s) => Number(s) <= 255 && String(Number(s)) === s);
  }
  if (!ip.includes(':') || !/^[0-9a-f:]+$/.test(ip)) return false;
  try { return new URL('http://[' + ip + ']/').hostname.startsWith('['); } catch { return false; }
}
export type Comparison = 'not-comparable' | 'physical-only-signal' | 'tunnel-only-signal' |
  'both-signal' | 'neither-signal' | 'inconclusive';

export function compareClientDiagnostics(a: ClientDiagnostic, b: ClientDiagnostic, now = Date.now()): Comparison {
  if (a.scope === b.scope || a.host !== b.host || a.controlHost !== b.controlHost ||
      !a.ip || a.ip !== b.ip || a.fingerprint !== b.fingerprint) return 'not-comparable';
  const ta = Date.parse(a.checkedAt), tb = Date.parse(b.checkedAt);
  // The two runs are sequential, not simultaneous; require a close time
  // window and recent observations. Network identity is not established.
  if (Math.abs(ta - tb) > 10 * 60_000 || Math.max(ta,tb) > now + 60_000 ||
      now - Math.min(ta,tb) > 24 * 60 * 60_000) return 'not-comparable';
  if (a.evidence === 'inconclusive' || b.evidence === 'inconclusive') return 'inconclusive';
  const physical = a.scope === 'client-physical' ? a : b;
  const tunnel = a.scope === 'client-tunnel' ? a : b;
  if (physical.evidence === 'asymmetry') return tunnel.evidence === 'asymmetry' ? 'both-signal' : 'physical-only-signal';
  return tunnel.evidence === 'asymmetry' ? 'tunnel-only-signal' : 'neither-signal';
}
