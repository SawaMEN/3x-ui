import { describe, expect, it } from 'vitest';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import {
  localCertificateFields,
  nodeCertificateForSni,
  mayReplaceCertificatePair,
} from '@/pages/inbounds/form/security/local-certificate';

const local: ManagedSniDomain = { name: 'vpn.example.org', source: 'gateway', certFile: '/main/fullchain.pem', keyFile: '/main/privkey.pem' };
const remote: ManagedSniDomain = { name: 'vpn.example.org', source: 'remote', nodeId: 5, certFile: '/remote/fullchain.pem', keyFile: '/remote/privkey.pem' };
describe('local TLS certificate binding', () => {
  it('never treats remote private key paths as main node paths', () => {
    expect(nodeCertificateForSni([remote], 'vpn.example.org', null)).toBeUndefined();
    expect(nodeCertificateForSni([remote, local], 'VPN.EXAMPLE.ORG.', 0)?.certFile).toBe(local.certFile);
    expect(nodeCertificateForSni([remote, local], 'vpn.example.org', 5)?.certFile).toBe(remote.certFile);
  });
  it('does not overwrite manual or incomplete certificate paths', () => {
    expect(mayReplaceCertificatePair('', '', [local], 0)).toBe(true);
    expect(mayReplaceCertificatePair(local.certFile, local.keyFile, [local], 0)).toBe(true);
    expect(mayReplaceCertificatePair('/manual.pem', '/manual.key', [local], 0)).toBe(false);
    expect(mayReplaceCertificatePair(local.certFile, '', [local], 0)).toBe(false);
  });
  it('distinguishes local TLS identities from REALITY and upstream SNI', () => {
    expect(localCertificateFields('naive', 'none')).toMatchObject({ cert: 'settings.tls.certificatePath', key: 'settings.tls.keyPath' });
    expect(localCertificateFields('tuic', 'none')).toMatchObject({ cert: 'settings.server.certificate' });
    expect(localCertificateFields('trusttunnel', 'none')).toMatchObject({ key: 'settings.privateKey' });
    expect(localCertificateFields('vless', 'tls')).toMatchObject({ cert: 'streamSettings.tlsSettings.certificates.0.certificateFile' });
    expect(localCertificateFields('vless', 'reality')).toBeNull();
    expect(localCertificateFields('fptn', 'none')).toBeNull();
    expect(localCertificateFields('shadowtls', 'none')).toBeNull();
  });
  it('ignores expired local certificates', () => {
    expect(nodeCertificateForSni([{ ...local, expiresAt: 1 }], local.name, 0)).toBeUndefined();
  });
});
