export type Evidence = 'asymmetry' | 'inconclusive' | 'no-signal';

export interface SiberianEvidence {
  ip: string;
  fingerprint: string;
  alphaError: string;
  betaError: string;
  evidence: Evidence;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('Invalid report');
  return value as Record<string, unknown>;
}

function stringField(value: unknown, max: number): string {
  if (typeof value !== 'string' || value.length > max) throw new Error('Invalid report field');
  return value;
}

function attemptError(value: unknown): string {
  const attempt = record(value);
  return attempt.error === undefined ? '' : stringField(attempt.error, 1000);
}

// Never treat two failures as confirmation. Preserve the observable evidence
// and do not trust the upstream summary or automatically alter configuration.
export function parseSiberianReport(raw: string): SiberianEvidence[] {
  if (raw.length > 1_000_000) throw new Error('Report exceeds 1 MB');
  const root = record(JSON.parse(raw));
  if (!Array.isArray(root.ips) || root.ips.length < 1 || root.ips.length > 128) {
    throw new Error('Invalid IP list');
  }
  const rows: SiberianEvidence[] = [];
  for (const entry of root.ips) {
    const ip = record(entry);
    const address = stringField(ip.ip, 128);
    if (!address || !Array.isArray(ip.checks) || ip.checks.length < 1 || ip.checks.length > 32) {
      throw new Error('Invalid checks');
    }
    for (const item of ip.checks) {
      const check = record(item);
      const alphaError = attemptError(check.alpha);
      const betaError = attemptError(check.beta);
      const fingerprint = stringField(check.fingerprint, 128);
      const evidence: Evidence =
        betaError || check.retried_with_real_sni === true
          ? 'inconclusive'
          : alphaError
            ? 'asymmetry'
            : 'no-signal';
      rows.push({ ip: address, fingerprint, alphaError, betaError, evidence });
    }
  }
  return rows;
}
