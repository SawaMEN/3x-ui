import {
  canApplyManagedSniCertificate,
  type ManagedSniDomain,
} from '@/api/queries/useManagedSniDomains';
import type { ManagerConfig } from './schema';

export function managerCertificateName(value: string): string {
  return value.trim().replace(/\.$/, '').toLowerCase();
}

// The managed-domain endpoint reports live, certificate-backed hosts. A
// certificate path is meaningful only on its hosting node, even when several
// nodes use the same public SNI.
export function managerTLSCertificatesForNode(
  sites: ManagedSniDomain[],
  nodeId: number,
): ManagedSniDomain[] {
  const node = nodeId > 0 ? nodeId : null;
  const distinct = new Map<string, ManagedSniDomain>();
  for (const site of sites) {
    if (!site || typeof site.name !== 'string' || !canApplyManagedSniCertificate(site, node))
      continue;
    const name = managerCertificateName(site.name);
    if (!name || !name.includes('.')) continue;
    if (!distinct.has(name)) distinct.set(name, { ...site, name });
  }
  return [...distinct.values()].sort((a, b) => a.name.localeCompare(b.name));
}

export function managerTLSCertificateForSni(
  sites: ManagedSniDomain[],
  nodeId: number,
  requestedSni: string,
): ManagedSniDomain | null {
  const candidates = managerTLSCertificatesForNode(sites, nodeId);
  const name = managerCertificateName(requestedSni);
  if (!name || name === 'auto') return candidates[0] ?? null;
  return candidates.find((site) => site.name === name) ?? null;
}

// Only Xray stream TLS uses the streamSettings.tlsSettings certificate shape.
// Native protocols have their own TLS settings and are intentionally not
// modified by this helper. The server's per-node preflight remains mandatory.
export function managerConfigWithTLSCertificate(
  config: ManagerConfig,
  certificate: ManagedSniDomain,
): ManagerConfig {
  const stream = JSON.parse(config.streamSettings) as {
    security?: string;
    tlsSettings?: {
      serverName?: string;
      certificates?: unknown[];
    };
  };
  if (stream.security !== 'tls' || !stream.tlsSettings)
    throw new Error('This recipe does not use Xray stream TLS');
  stream.tlsSettings.serverName = managerCertificateName(certificate.name);
  stream.tlsSettings.certificates = [
    {
      certificateFile: certificate.certFile,
      keyFile: certificate.keyFile,
      ocspStapling: 0,
      oneTimeLoading: false,
      usage: 'encipherment',
      buildChain: false,
    },
  ];
  return { ...config, streamSettings: JSON.stringify(stream) };
}
