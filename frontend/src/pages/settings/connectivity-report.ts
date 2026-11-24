import { diagnosticHostname, validIP, type DiagnosticScope } from './vetroff-diagnostic-report';

type Outcome =
  | 'ok'
  | 'error'
  | 'timeout'
  | 'canceled'
  | 'certificate-error'
  | 'not-found'
  | 'no-address'
  | 'invalid-answer'
  | 'not-tested';
interface Stage {
  outcome: Outcome;
  durationMs: number;
}
export interface AddressCheck {
  ip: string;
  tcp: Stage;
  tls: Stage;
  http: Stage;
  httpStatus?: number;
}
export interface FamilyCheck {
  family: 'ip4' | 'ip6';
  dns: Stage;
  resolved: number;
  truncated: boolean;
  addresses: AddressCheck[];
}
export interface ConnectivityReport {
  scope: DiagnosticScope;
  host: string;
  checkedAt: string;
  complete: boolean;
  families: FamilyCheck[];
}

function object(v: unknown): Record<string, unknown> {
  if (!v || typeof v !== 'object' || Array.isArray(v)) throw new Error('Invalid object');
  return v as Record<string, unknown>;
}
const errors = ['error', 'timeout', 'canceled'] as const;
function stage(v: unknown, allowed: readonly string[]): Stage {
  const s = object(v);
  if (
    typeof s.outcome !== 'string' ||
    !allowed.includes(s.outcome) ||
    typeof s.durationMs !== 'number' ||
    !Number.isSafeInteger(s.durationMs) ||
    s.durationMs < 0 ||
    s.durationMs > 30000 ||
    (s.outcome === 'not-tested' && s.durationMs !== 0)
  )
    throw new Error('Invalid stage');
  return { outcome: s.outcome as Outcome, durationMs: s.durationMs };
}
export function parseAddressCheck(item: unknown): AddressCheck {
  const a = object(item);
  if (typeof a.ip !== 'string' || a.ip.length > 45 || !validIP(a.ip)) throw new Error('Invalid IP');
  const tcp = stage(a.tcp, ['ok', ...errors]);
  const tls = stage(a.tls, ['ok', ...errors, 'certificate-error', 'not-tested']);
  const http = stage(a.http, ['ok', ...errors, 'not-tested']);
  if (
    (tcp.outcome === 'ok') !== (tls.outcome !== 'not-tested') ||
    (tls.outcome === 'ok') !== (http.outcome !== 'not-tested')
  )
    throw new Error('Inconsistent stages');
  const httpStatus = a.httpStatus;
  if (
    httpStatus !== undefined &&
    (typeof httpStatus !== 'number' ||
      !Number.isInteger(httpStatus) ||
      httpStatus < 100 ||
      httpStatus > 999)
  )
    throw new Error('Invalid HTTP status');
  if (http.outcome === 'ok' && (httpStatus === undefined || httpStatus < 200 || httpStatus > 599))
    throw new Error('Missing final response');
  if (http.outcome === 'not-tested' && httpStatus !== undefined)
    throw new Error('Unexpected HTTP status');
  return { ip: a.ip, tcp, tls, http, httpStatus };
}
export function parseConnectivityReport(raw: string): ConnectivityReport {
  if (raw.length > 65536) throw new Error('Report too large');
  const r = object(JSON.parse(raw));
  if (
    r.version !== 1 ||
    r.kind !== 'connectivity' ||
    (r.scope !== 'client-physical' && r.scope !== 'client-tunnel') ||
    typeof r.complete !== 'boolean' ||
    typeof r.checkedAt !== 'string' ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(r.checkedAt) ||
    !Number.isFinite(Date.parse(r.checkedAt)) ||
    !Array.isArray(r.families) ||
    r.families.length > 2 ||
    (r.complete && r.families.length !== 2)
  )
    throw new Error('Invalid connectivity report');
  const families: FamilyCheck[] = [];
  for (const entry of r.families) {
    const f = object(entry);
    if (
      (f.family !== 'ip4' && f.family !== 'ip6') ||
      families.some((x) => x.family === f.family) ||
      typeof f.resolved !== 'number' ||
      !Number.isSafeInteger(f.resolved) ||
      f.resolved < 0 ||
      f.resolved > 32 ||
      typeof f.truncated !== 'boolean' ||
      !Array.isArray(f.addresses) ||
      f.addresses.length > Math.min(f.resolved, 2) ||
      f.truncated !== f.resolved > 2 ||
      (r.complete && f.addresses.length !== Math.min(f.resolved, 2))
    )
      throw new Error('Invalid address family');
    const dns = stage(f.dns, ['ok', ...errors, 'not-found', 'no-address', 'invalid-answer']);
    if ((dns.outcome === 'ok') !== f.resolved > 0) throw new Error('Inconsistent DNS result');
    const addresses: AddressCheck[] = [];
    for (const item of f.addresses) {
      const a = parseAddressCheck(item);
      if ((f.family === 'ip4') !== !a.ip.includes(':') || addresses.some((x) => x.ip === a.ip))
        throw new Error('Invalid address family or duplicate IP');
      addresses.push(a);
    }
    families.push({
      family: f.family,
      dns,
      resolved: f.resolved,
      truncated: f.truncated,
      addresses,
    });
  }
  return {
    scope: r.scope,
    host: diagnosticHostname(r.host),
    checkedAt: r.checkedAt,
    complete: r.complete,
    families,
  };
}

export type Finding =
  | 'incomplete'
  | 'stale'
  | 'sampled'
  | 'no-address'
  | 'dns-error'
  | 'tcp-error'
  | 'certificate-error'
  | 'tls-error'
  | 'http-error'
  | 'http-denied'
  | 'http-server-error'
  | 'family-difference'
  | 'reachable';
export function connectivityFindings(r: ConnectivityReport, now = Date.now()): Finding[] {
  const result = new Set<Finding>();
  const at = Date.parse(r.checkedAt);
  if (at > now + 60000 || now - at > 86400000) result.add('stale');
  if (!r.complete) result.add('incomplete');
  const usable = new Map<string, boolean>();
  for (const f of r.families) {
    if (f.truncated) result.add('sampled');
    if (f.dns.outcome === 'no-address' || f.dns.outcome === 'not-found') result.add('no-address');
    else if (f.dns.outcome !== 'ok') result.add('dns-error');
    let reachable = false;
    for (const a of f.addresses) {
      if (a.tcp.outcome !== 'ok') result.add('tcp-error');
      else if (a.tls.outcome === 'certificate-error') result.add('certificate-error');
      else if (a.tls.outcome !== 'ok') result.add('tls-error');
      else if (a.http.outcome !== 'ok') result.add('http-error');
      else {
        reachable = true;
        if (a.httpStatus! >= 500) result.add('http-server-error');
        else if (a.httpStatus! >= 400) result.add('http-denied');
      }
    }
    if (f.addresses.length) usable.set(f.family, reachable);
    if (reachable) result.add('reachable');
  }
  if (r.complete && usable.size === 2 && usable.get('ip4') !== usable.get('ip6'))
    result.add('family-difference');
  return [...result];
}
