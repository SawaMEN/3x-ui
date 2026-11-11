import { describe, expect, it } from 'vitest';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import {
  managerCertificateName,
  managerTLSCertificateForSni,
  managerTLSCertificatesForNode,
  managerConfigNeedsTLSCertificate,
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
  tlsSniByNode: Record<number, string> = {},
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
    tlsSniByNode,
    allowUnconfiguredTLS,
  });
}

describe('manager per-node TLS certificates', () => {
  it('accepts explicit node-local cert/key paths for each TLS protocol without borrowing from other nodes', () => {
    const result = buildManagerRecipes({
      presets: ['naive'],
      nodeIds: [0, 7],
      nodes: [{ ...nodes[0], protocolAvailability: { naive: { ready: true, supported: true, version: 'test', component: '', speedLimit: true } } }],
      domains: [],
      sniByPreset: { naive: 'vpn.example.org' },
      targetByPreset: {},
      verifiedSniByNode: {},
      managedCertificates: [],
      certificateFilesByPair: { 'naive|0': { certFile: '/main/fullchain.pem', keyFile: '/main/privkey.pem' } },
    });
    expect(result.entries).toHaveLength(1);
    const settings = JSON.parse(result.entries[0].config!.settings);
    expect(settings.tls.certificatePath).toBe('/main/fullchain.pem');
    expect(result.skipped).toHaveLength(1);
  });

  it('refuses a half-filled explicit certificate override instead of deploying a broken pair', () => {
    const result = buildManagerRecipes({
      presets: ['vless-tcp-tls'],
      nodeIds: [0],
      nodes: [],
      domains: [],
      sniByPreset: { 'vless-tcp-tls': 'vpn.example.org' },
      targetByPreset: {},
      verifiedSniByNode: {},
      managedCertificates: sites,
      certificateFilesByPair: { 'vless-tcp-tls|0': { certFile: '/my/fullchain.pem', keyFile: '' } },
      allowUnconfiguredTLS: true,
    });
    expect(result.entries).toHaveLength(0);
    expect(result.skipped[0]).toContain('both TLS certificate and private key');
  });

  it('normalizes case and trailing dots before matching an SNI certificate', () => {
    expect(managerCertificateName(' VPN.EXAMPLE.ORG. ')).toBe('vpn.example.org');
  });

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

  it('never automatically selects a reported certificate that has expired', () => {
    const expired: ManagedSniDomain = {
      ...remote,
      expiresAt: Math.floor(Date.now() / 1000) - 60,
    };
    const selected = managerTLSCertificatesForNode([expired, local], 7);
    expect(selected).toHaveLength(0);
    const built = plan([expired, local]);
    expect(built.entries).toHaveLength(1);
    expect(built.skipped).toHaveLength(2);
  });

  it('skips nodes without a compatible certificate instead of borrowing paths', () => {
    const result = plan([local]);
    expect(result.entries).toHaveLength(1);
    expect(result.entries[0].nodeId).toBe(0);
    expect(result.skipped).toHaveLength(2);
    expect(result.skipped.every((s) => s.includes('no certificate'))).toBe(true);
  });

  it('allows each node to choose a different certificate for the same TLS protocol', () => {
    const other: ManagedSniDomain = {
      name: 'france.example.org',
      source: 'remote',
      nodeId: 7,
      certFile: '/etc/france/fullchain.pem',
      keyFile: '/etc/france/privkey.pem',
    };
    const entries = plan([...sites, other], false, 'vpn.example.org', {
      7: 'france.example.org',
      9: 'auto',
    }).entries;
    expect(entries).toHaveLength(3);
    expect(entries[0].sni).toBe('vpn.example.org');
    expect(entries[1].sni).toBe('france.example.org');
    expect(entries[2].sni).toBe('vpn.example.org');
    const nodeCert = JSON.parse(entries[1].config?.streamSettings || '{}').tlsSettings
      .certificates[0];
    expect(nodeCert.certificateFile).toBe('/etc/france/fullchain.pem');
  });

  it('maps certificate files to each native TLS protocol without Xray-only fields', () => {
    for (const protocol of ['naive', 'anytls', 'masque', 'tuic', 'vetroff', 'trusttunnel']) {
      const config = createManagerPreset(protocol);
      expect(managerConfigNeedsTLSCertificate(config)).toBe(true);
      const filled = managerConfigWithTLSCertificate(config, local);
      const settings = JSON.parse(filled.settings);
      expect(filled.streamSettings).toBe(config.streamSettings);
      if (['naive', 'anytls', 'masque'].includes(protocol)) {
        expect(settings.tls).toMatchObject({
          enabled: true,
          serverName: local.name,
          certificatePath: local.certFile,
          keyPath: local.keyFile,
        });
      } else if (protocol === 'tuic') {
        expect(settings.server).toMatchObject({
          certificate: local.certFile,
          private_key: local.keyFile,
          sni: local.name,
        });
      } else {
        expect(settings).toMatchObject({
          hostname: local.name,
          certificate: local.certFile,
          privateKey: local.keyFile,
        });
      }
    }
  });

  it('classifies native TLS protocols into certificate-backed recipes', () => {
    for (const protocol of ['naive', 'anytls', 'masque', 'tuic', 'vetroff', 'trusttunnel']) {
      const built = buildManagerRecipes({
        presets: [protocol],
        nodeIds: [0],
        nodes: [],
        domains: [],
        sniByPreset: {},
        targetByPreset: {},
        verifiedSniByNode: {},
        managedCertificates: [local],
      });
      expect(built.entries).toHaveLength(1);
      expect(built.entries[0].sni).toBe(local.name);
    }
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
    expect(() =>
      managerConfigWithTLSCertificate(createManagerPreset('vless-tcp-reality'), local),
    ).toThrow('does not use Xray stream TLS');
  });
});
