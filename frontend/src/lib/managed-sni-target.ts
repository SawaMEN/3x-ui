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

// A certificate on another host cannot be used by the selected inbound.
// Remote summaries intentionally omit paths, but include verified expiry.
export function managedSniForNode(
  domains: ManagedSniDomain[],
  nodeId: number | null | undefined,
  purpose: 'tls' | 'reality' | 'shadowtls' | 'fakeTls',
): ManagedSniDomain[] {
  const selectedNode = nodeId ?? 0;
  const now = Date.now() / 1000;
  return domains.filter((domain) => {
    if (domain.name.startsWith('*.') || (domain.expiresAt != null && domain.expiresAt <= now))
      return false;
    if (domain.source === 'remote') {
      if (selectedNode <= 0 || domain.nodeId !== selectedNode) return false;
    } else if (selectedNode !== 0) return false;
    if (purpose !== 'tls') return !!managedRealityTarget(domain);
    return domain.source === 'remote'
      ? domain.expiresAt != null && domain.expiresAt > now
      : !!domain.certFile && !!domain.keyFile;
  });
}
