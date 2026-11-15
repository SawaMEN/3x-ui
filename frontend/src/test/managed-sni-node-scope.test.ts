import { expect, it } from 'vitest';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import { managedSniForNode } from '@/lib/managed-sni-target';

const local: ManagedSniDomain = {
  name: 'local.example.org',
  source: 'gateway',
  certFile: '/cert',
  keyFile: '/key',
  realityTarget: 'local.example.org:8443',
  expiresAt: 4102444800,
};
const remote: ManagedSniDomain = {
  name: 'node.example.org',
  source: 'remote',
  nodeId: 7,
  certFile: '',
  keyFile: '',
  realityTarget: 'node.example.org:9443',
  expiresAt: 4102444800,
};
const sites: ManagedSniDomain[] = [
  local,
  remote,
  { ...remote, nodeId: 9 },
  { ...local, name: 'expired.example.org', expiresAt: 1 },
  { ...local, name: 'cert.example.org', realityTarget: undefined },
  { ...local, name: '*.example.org', realityTarget: '*.example.org:443' },
  { name: 'reference.example.org', source: 'remote', certFile: '', keyFile: '' },
];

it.each([null, undefined, 0])(
  'treats node %s as the main panel and hides other nodes',
  (nodeId) => {
    expect(managedSniForNode(sites, nodeId, 'tls').map((site) => site.name)).toEqual([
      'local.example.org',
      'cert.example.org',
    ]);
  },
);

it('uses verified node certificate summaries without leaking or requiring private paths', () => {
  expect(managedSniForNode(sites, 7, 'tls')).toEqual([remote]);
  expect(managedSniForNode(sites, 8, 'tls')).toEqual([]);
  expect(managedSniForNode([{ ...remote, expiresAt: undefined }], 7, 'tls')).toEqual([]);
  expect(managedSniForNode([{ ...local, keyFile: '' }], 0, 'tls')).toEqual([]);
});

it.each(['reality', 'shadowtls', 'fakeTls'] as const)(
  'requires a site on the selected node for %s',
  (purpose) => {
    expect(managedSniForNode(sites, 0, purpose)).toEqual([local]);
    expect(managedSniForNode(sites, 7, purpose)).toEqual([remote]);
    expect(managedSniForNode(sites, 8, purpose)).toEqual([]);
  },
);
