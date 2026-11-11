import {
  canApplyManagedSniCertificate,
  type ManagedSniDomain,
} from '@/api/queries/useManagedSniDomains';

export type LocalCertificateFields = {
  cert: string;
  key: string;
  autoTLS?: string;
};

// Only server-side TLS protocols use a locally stored certificate.
// REALITY/ShadowTLS, upstream SNI and certificate pinning are not server identities.
export function localCertificateFields(
  protocol: string,
  streamSecurity: string,
): LocalCertificateFields | null {
  if (protocol === 'tuic')
    return { cert: 'settings.server.certificate', key: 'settings.server.private_key' };
  if (protocol === 'vetroff')
    return {
      cert: 'settings.certificate',
      key: 'settings.privateKey',
      autoTLS: 'settings.vetroff.autoTLS',
    };
  // FPTN accepts only RSA certificates; the lookup API enforces RSA.
  if (protocol === 'fptn') return { cert: 'settings.certificate', key: 'settings.privateKey' };
  if (protocol === 'trusttunnel')
    return { cert: 'settings.certificate', key: 'settings.privateKey' };
  if (['naive', 'anytls', 'masque'].includes(protocol))
    return { cert: 'settings.tls.certificatePath', key: 'settings.tls.keyPath' };
  if (streamSecurity === 'tls')
    return {
      cert: 'streamSettings.tlsSettings.certificates.0.certificateFile',
      key: 'streamSettings.tlsSettings.certificates.0.keyFile',
    };
  return null;
}

export function canUseManagedSniCertificate(
  site: ManagedSniDomain,
  nodeId: number | null | undefined,
): boolean {
  return (
    canApplyManagedSniCertificate(site, nodeId === 0 ? null : nodeId) &&
    (!site.expiresAt || site.expiresAt > Date.now() / 1000)
  );
}

export function nodeCertificateForSni(
  sites: ManagedSniDomain[],
  sni: string,
  nodeId: number | null | undefined,
): ManagedSniDomain | undefined {
  const name = sni.trim().toLowerCase().replace(/\.$/, '');
  if (!name || !name.includes('.')) return undefined;
  return sites.find(
    (site) =>
      site.name.trim().toLowerCase().replace(/\.$/, '') === name &&
      canUseManagedSniCertificate(site, nodeId),
  );
}

export function mayReplaceCertificatePair(
  currentCert: string,
  currentKey: string,
  sites: ManagedSniDomain[],
  nodeId: number | null | undefined,
): boolean {
  if (!currentCert && !currentKey) return true;
  // Never replace custom, imported or partially filled paths silently.
  return sites.some(
    (site) =>
      canUseManagedSniCertificate(site, nodeId) &&
      site.certFile === currentCert &&
      site.keyFile === currentKey,
  );
}
