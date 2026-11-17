import { describe, expect, it } from 'vitest';

import {
  canApplyManagedSniCertificate,
  type ManagedSniDomain,
} from '@/api/queries/useManagedSniDomains';

const localSite: ManagedSniDomain = {
  name: 'local.example.com',
  source: 'telemt',
  certFile: '/root/cert/local/fullchain.pem',
  keyFile: '/root/cert/local/privkey.pem',
};

const usSite: ManagedSniDomain = {
  name: 'edge.example.com',
  source: 'remote',
  nodeId: 7,
  nodeName: 'USA',
  certFile: '/etc/letsencrypt/live/edge.example.com/fullchain.pem',
  keyFile: '/etc/letsencrypt/live/edge.example.com/privkey.pem',
};

describe('managed SNI certificate ownership', () => {
  it('reuses a main-panel certificate only on that panel', () => {
    expect(canApplyManagedSniCertificate(localSite, null)).toBe(true);
    expect(canApplyManagedSniCertificate(localSite, undefined)).toBe(true);
    expect(canApplyManagedSniCertificate(localSite, 7)).toBe(false);
  });

  it('treats existing panel and subscription certificates as local-only files', () => {
    for (const source of ['panel', 'subscription'] as const) {
      const site: ManagedSniDomain = { ...localSite, source };
      expect(canApplyManagedSniCertificate(site, null)).toBe(true);
      expect(canApplyManagedSniCertificate(site, undefined)).toBe(true);
      expect(canApplyManagedSniCertificate(site, 7)).toBe(false);
    }
  });

  it('reuses a foreign certificate only on its owning node', () => {
    expect(canApplyManagedSniCertificate(usSite, 7)).toBe(true);
    expect(canApplyManagedSniCertificate(usSite, 8)).toBe(false);
    expect(canApplyManagedSniCertificate(usSite, undefined)).toBe(false);
  });

  it('never substitutes missing or incomplete file paths', () => {
    const unknown: ManagedSniDomain = { ...usSite, certFile: '', keyFile: '' };
    expect(canApplyManagedSniCertificate(unknown, 7)).toBe(false);
    expect(canApplyManagedSniCertificate({ ...localSite, keyFile: '' }, null)).toBe(false);
  });

  it('cannot infer certificate ownership from manually entered foreign SNI', () => {
    const manual: ManagedSniDomain = {
      name: 'remote.example.com',
      source: 'remote',
      certFile: '',
      keyFile: '',
    };
    expect(canApplyManagedSniCertificate(manual, 7)).toBe(false);
  });
});
