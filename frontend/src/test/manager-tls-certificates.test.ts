import { describe, expect, it } from 'vitest';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import {
  managerTLSCertificateForSni,
  managerTLSCertificatesForNode,
  managerConfigWithTLSCertificate,
} from '@/pages/manager/certificate-recipes';
import { buildManagerRecipes } from '@/pages/manager/catalog';
import { createManagerPreset } from '@/pages/manager/presets';
import type { NodeRecord } from '@/schemas/node';

const local: ManagedSniDomain = {
  name: 'vpn.example.org',
  source: 'gateway',
  certFile: '/etc/panel/fullchain.pem',
  keyFile: '/etc/panel/privkey.pem',
};
const remote: ManagedSniDomain = {
  name: 'VPN.EXAMPLE.ORG.',
  source: 'remote',
  nodeId: 7,
  nodeName: 'Europe',
  certFile: '/etc/nodes/7/cert.pem',
  keyFile: '/etc/nodes/7/key.pem',
};
const another: ManagedSniDomain = {
  name: 'vpn.example.org',
  source: 'remote',
  nodeId: 9,
  nodeName: 'Asia',
  certFile: '/etc/nodes/9/cert.pem',
  keyFile: '/etc/nodes/9/key.pem',
};
const sites = [local, remote, another];
const nodes: NodeRecord[] = [7, 9].map((id) => ({
  id,
  name: 'Node ' + id,
  enable: true,
  status: 'online',
  coreType: 'xray',
  protocolAvailability: {
    vless: { ready: true, supported: true, version: 'test', component: '', speedLimit: true },
  },
}));

function plan(
  certificates: ManagedSniDomain[],
  allowUnconfiguredTLS = false,
  requestedSni = 'auto',
) {
  return buildManagerRecipes({
    presets: ['vless-tcp-tls'],
    nodeIds: [0, 7, 9],
    nodes,
    domains: [],
    sniByPreset: { 'vless-tcp-tls': requestedSni },
    targetByPreset: {},
    verifiedSniByNode: {},
    managedCertificates: certificates,
    allowUnconfiguredTLS,
  });
}

describe('manager per-node TLS certificates', () => {
  it('retains the same hostname independently on the panel and multiple nodes', () => {
    expect(managerTLSCertificatesForNode(sites, 0)).toEqual([
      expect.objectContaining({ certFile: local.certFile }),
    ]);
    expect(managerTLSCertificatesForNode(sites, 7)).toEqual([
      expect.objectContaining({ certFile: remote.certFile }),
    ]);
    expect(managerTLSCertificatesForNode(sites, 9)).toEqual([
      expect.objectContaining({ certFile: another.certFile }),
    ]);
    expect(managerTLSCertificateForSni(sites, 7, 'VPN.EXAMPLE.ORG.')).toMatchObject({
      certFile: remote.certFile,
    });
  });

  it('automatically uses the certificate belonging to each node and preserves SNI', () => {
    const entries = plan(sites).entries;
    expect(entries).toHaveLength(3);
    for (let i = 0; i < entries.length; i++) {
      const stream = JSON.parse(entries[i].config?.streamSettings || '{}');
      expect(stream.security).toBe('tls');
      expect(stream.tlsSettings.serverName).toBe('vpn.example.org');
      expect(entries[i].sni).toBe('vpn.example.org');
      expect(stream.tlsSettings.certificates[0]).toMatchObject({
        certificateFile: sites[i].certFile,
        keyFile: sites[i].keyFile,
      });
    }
  });

  it('skips nodes without a compatible certificate instead of borrowing paths', () => {
    const result = plan([local]);
    expect(result.entries).toHaveLength(1);
    expect(result.entries[0].nodeId).toBe(0);
    expect(result.skipped).toHaveLength(2);
    expect(result.skipped.every((s) => s.includes('no certificate'))).toBe(true);
  });

  it('supports explicit manual TLS drafts but still requires preflight validation', () => {
    const result = plan([], true);
    expect(result.entries).toHaveLength(3);
    expect(result.entries[0].config?.streamSettings).toContain('"certificates":[]');
  });

  it('rejects a requested SNI unless its certificate is available on the node', () => {
    expect(plan(sites, false, 'unknown.example.org').entries).toHaveLength(0);
    expect(plan(sites, false, 'unknown.example.org').skipped).toHaveLength(3);
  });

  it('refuses to change a REALITY configuration into TLS', () => {
    expect(() => managerConfigWithTLSCertificate(createManagerPreset('vless-tcp-reality'), local))
      .toThrow('does not use Xray stream TLS');
  });
});
