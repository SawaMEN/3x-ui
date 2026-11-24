import { parseAddressCheck, type AddressCheck } from './connectivity-report';
import { diagnosticHostname, type DiagnosticScope } from './vetroff-diagnostic-report';

export interface AddressReport {
  scope: DiagnosticScope;
  host: string;
  checkedAt: string;
  complete: boolean;
  address?: AddressCheck;
}

export function parseAddressReport(raw: string): AddressReport {
  if (raw.length > 65536) throw new Error('Report too large');
  const value: unknown = JSON.parse(raw);
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('Invalid report');
  const r = value as Record<string, unknown>;
  if (
    r.version !== 1 ||
    r.kind !== 'address-connectivity' ||
    (r.scope !== 'client-physical' && r.scope !== 'client-tunnel') ||
    typeof r.complete !== 'boolean' ||
    typeof r.checkedAt !== 'string' ||
    !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(r.checkedAt) ||
    !Number.isFinite(Date.parse(r.checkedAt))
  )
    throw new Error('Invalid address report');
  const address = r.address === undefined ? undefined : parseAddressCheck(r.address);
  if (r.complete && !address) throw new Error('Missing complete observation');
  return {
    scope: r.scope,
    host: diagnosticHostname(r.host),
    checkedAt: r.checkedAt,
    complete: r.complete,
    address,
  };
}

export type AddressFinding =
  | 'incomplete'
  | 'stale'
  | 'tcp-error'
  | 'certificate-error'
  | 'tls-error'
  | 'http-error'
  | 'http-denied'
  | 'http-server-error'
  | 'reachable';

function fresh(r: AddressReport, now: number): boolean {
  const at = Date.parse(r.checkedAt);
  return Number.isFinite(at) && at <= now + 60000 && now - at <= 86400000;
}

export function addressFindings(r: AddressReport, now = Date.now()): AddressFinding[] {
  const result: AddressFinding[] = [];
  if (!fresh(r, now)) result.push('stale');
  if (!r.complete) result.push('incomplete');
  const a = r.address;
  if (!a) return result;
  if (a.tcp.outcome !== 'ok') result.push('tcp-error');
  else if (a.tls.outcome === 'certificate-error') result.push('certificate-error');
  else if (a.tls.outcome !== 'ok') result.push('tls-error');
  else if (a.http.outcome !== 'ok') result.push('http-error');
  else {
    result.push('reachable');
    if (a.httpStatus !== undefined && a.httpStatus >= 500) result.push('http-server-error');
    else if (a.httpStatus !== undefined && a.httpStatus >= 400) result.push('http-denied');
  }
  return result;
}

export type AddressComparison =
  | 'not-comparable'
  | 'inconclusive'
  | 'physical-only'
  | 'tunnel-only'
  | 'both-responded'
  | 'neither-responded';

export function compareAddressReports(
  a: AddressReport,
  b: AddressReport,
  now = Date.now(),
): AddressComparison {
  if (
    a.scope === b.scope ||
    a.host !== b.host ||
    !a.address ||
    !b.address ||
    a.address.ip !== b.address.ip ||
    !fresh(a, now) ||
    !fresh(b, now) ||
    Math.abs(Date.parse(a.checkedAt) - Date.parse(b.checkedAt)) > 600000
  )
    return 'not-comparable';
  if (
    !a.complete ||
    !b.complete ||
    [a, b].some((r) =>
      [r.address!.tcp, r.address!.tls, r.address!.http].some((s) => s.outcome === 'canceled'),
    )
  )
    return 'inconclusive';
  const physical = a.scope === 'client-physical' ? a : b;
  const tunnel = a.scope === 'client-tunnel' ? a : b;
  const p = physical.address!.http.outcome === 'ok',
    t = tunnel.address!.http.outcome === 'ok';
  return p ? (t ? 'both-responded' : 'physical-only') : t ? 'tunnel-only' : 'neither-responded';
}
