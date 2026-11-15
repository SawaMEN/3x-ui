import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';

// Older peers and certificate-only entries remain usable for certificate
// installation, but cannot safely supply a REALITY target automatically.
export function managedRealityTarget(domain: ManagedSniDomain): string {
  if (typeof domain.realityTarget !== 'string' || domain.name.startsWith('*.')) return '';
  const match = /^([^:/\s]+):(\d+)$/.exec(domain.realityTarget);
  if (!match || match[1].toLowerCase() !== domain.name.toLowerCase()) return '';
  const port = Number(match[2]);
  return port >= 1 && port <= 65535 ? `${domain.name}:${port}` : '';
}
