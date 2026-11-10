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
    if (
      !site ||
      typeof site.name !== 'string' ||
      !canApplyManagedSniCertificate(site, node) ||
      (site.expiresAt !== undefined && site.expiresAt <= Date.now() / 1000)
    )
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

// Map the certificate to the exact configuration shape consumed by the
// protocol. A native TLS protocol must never receive Xray-only tlsSettings.
export function managerConfigNeedsTLSCertificate(config: ManagerConfig): boolean {
  const stream = JSON.parse(config.streamSettings) as { security?: string };
  if (stream.security === 'tls') return true;
  const settings = JSON.parse(config.settings) as Record<string, any>;
  if (['tuic', 'vetroff', 'trusttunnel'].includes(config.protocol)) return true;
  const nativeTLS = settings.tls as
    | { enabled?: boolean; certificatePath?: string; keyPath?: string }
    | undefined;
  return (
    !!nativeTLS &&
    nativeTLS.enabled !== false &&
    'certificatePath' in nativeTLS &&
    'keyPath' in nativeTLS
  );
}

export function managerConfigWithTLSCertificate(
  config: ManagerConfig,
  certificate: ManagedSniDomain,
): ManagerConfig {
  const stream = JSON.parse(config.streamSettings) as {
    security?: string;
    tlsSettings?: { serverName?: string; certificates?: unknown[] };
  };
  const hostname = managerCertificateName(certificate.name);
  if (stream.security === 'tls') {
    if (!stream.tlsSettings) throw new Error('Xray stream TLS settings are missing');
    stream.tlsSettings.serverName = hostname;
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
  const settings = JSON.parse(config.settings) as Record<string, any>;
  if (
    settings.tls &&
    settings.tls.enabled !== false &&
    'certificatePath' in settings.tls &&
    'keyPath' in settings.tls
  ) {
    settings.tls.enabled = true;
    settings.tls.serverName = hostname;
    settings.tls.certificatePath = certificate.certFile;
    settings.tls.keyPath = certificate.keyFile;
    return { ...config, settings: JSON.stringify(settings) };
  }
  if (config.protocol === 'tuic') {
    const server = (settings.server ??= {});
    server.certificate = certificate.certFile;
    server.private_key = certificate.keyFile;
    server.sni = hostname;
    settings.certificate = certificate.certFile;
    settings.private_key = certificate.keyFile;
    settings.sni = hostname;
    return { ...config, settings: JSON.stringify(settings) };
  }
  if (config.protocol === 'vetroff' || config.protocol === 'trusttunnel') {
    settings.hostname = hostname;
    settings.certificate = certificate.certFile;
    settings.privateKey = certificate.keyFile;
    if (config.protocol === 'vetroff' && settings.vetroff) settings.vetroff.autoTLS = false;
    return { ...config, settings: JSON.stringify(settings) };
  }
  throw new Error('No supported TLS certificate mapping for protocol ' + config.protocol);
}
