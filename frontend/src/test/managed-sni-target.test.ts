import { expect, it } from 'vitest';
import { managedRealityTarget } from '../lib/managed-sni-target';
import type { ManagedSniDomain } from '../api/queries/useManagedSniDomains';

it('uses the advertised HTTPS port and requires explicit endpoint metadata', () => {
  const site: ManagedSniDomain = {
    name: 'vpn.example.org',
    source: 'gateway',
    certFile: '/cert',
    keyFile: '/key',
  };
  expect(managedRealityTarget(site)).toBe('');
  expect(managedRealityTarget({ ...site, realityTarget: 'vpn.example.org:8443' })).toBe(
    'vpn.example.org:8443',
  );
  expect(
    managedRealityTarget({ ...site, source: 'remote', realityTarget: 'vpn.example.org:9443' }),
  ).toBe('vpn.example.org:9443');
  for (const target of [
    'other.example.org:443',
    'vpn.example.org:0',
    'vpn.example.org:65536',
    'https://vpn.example.org:443',
    'vpn.example.org:443/path',
  ]) {
    expect(managedRealityTarget({ ...site, realityTarget: target })).toBe('');
  }
  expect(
    managedRealityTarget({ ...site, name: '*.example.org', realityTarget: '*.example.org:443' }),
  ).toBe('');
});
