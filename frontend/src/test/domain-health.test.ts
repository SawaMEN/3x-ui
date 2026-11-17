import { describe, expect, it } from 'vitest';
import type { DomainFrontStatus } from '@/generated/zod';
import { defaultDomain, defaultDomainConfig } from '@/schemas/domain-front';
import {
  certificateNeedsAttention,
  getDomainHealth,
  getDomainSummary,
} from '@/pages/settings/domain-health';

const NOW = Date.UTC(2026, 10, 10);
const DAY = 86400;
const domain = { ...defaultDomain, name: 'sub.example.org', mode: 'sub_link_only' as const };
const cert = {
  name: domain.name,
  state: 'valid',
  issuer: 'Test CA',
  source: 'panel',
  renewal: 'certmagic',
  mode: 'auto',
  autoRenew: true,
  certFile: '/certs/fullchain.pem',
  keyFile: '/certs/privkey.pem',
  error: '',
  expiresAt: Math.floor(NOW / 1000) + 45 * DAY,
  daysLeft: 45,
};
function status(): DomainFrontStatus {
  return {
    running: true,
    error: '',
    config: { ...defaultDomainConfig, enabled: true, domains: [domain] },
    certificates: [cert],
    existingCertificates: [],
  };
}

describe('domain workspace health', () => {
  it('does not treat remote SNI or a disabled domain as a failed local certificate', () => {
    const remote = { ...domain, mode: 'shared_sni' as const };
    expect(getDomainHealth(remote, status(), undefined, true, NOW)).toBe('external');
    expect(getDomainHealth({ ...domain, enabled: false }, status(), undefined, true, NOW)).toBe(
      'disabled',
    );
  });

  it('reports a valid certificate and raises a 30-day renewal warning', () => {
    expect(getDomainHealth(domain, status(), undefined, true, NOW)).toBe('healthy');
    expect(certificateNeedsAttention(cert, NOW)).toBe('healthy');
    const nearExpiry = { ...cert, expiresAt: Math.floor(NOW / 1000) + 10 * DAY };
    expect(certificateNeedsAttention(nearExpiry, NOW)).toBe('expiring');
    expect(
      getDomainHealth(domain, { ...status(), certificates: [nearExpiry] }, undefined, true, NOW),
    ).toBe('expiring');
  });

  it('prioritizes public TLS errors over a valid local certificate', () => {
    expect(
      getDomainHealth(
        domain,
        status(),
        {
          name: domain.name,
          addresses: [],
          issuer: '',
          daysLeft: 0,
          expiresAt: 0,
          checkedAt: Math.floor(NOW / 1000),
          error: 'connection refused',
        },
        true,
        NOW,
      ),
    ).toBe('error');
  });

  it('keeps uncommitted domain edits separate from the server health', () => {
    const draft = {
      ...defaultDomainConfig,
      enabled: true,
      domains: [{ ...domain, tls: 'manual' as const }],
    };
    expect(getDomainSummary(draft, status(), {}, NOW)).toMatchObject({
      total: 1,
      active: 1,
      healthy: 0,
      attention: 0,
      pending: 1,
    });
  });

  it('includes inherited certificates in expiry alert counts', () => {
    const inherited = {
      ...cert,
      name: 'panel',
      source: 'installer',
      expiresAt: Math.floor(NOW / 1000) + 15 * DAY,
    };
    expect(
      getDomainSummary(
        { ...defaultDomainConfig, enabled: true, domains: [domain] },
        { ...status(), existingCertificates: [inherited] },
        {},
        NOW,
      ),
    ).toMatchObject({
      expiringCertificates: 1,
      certificateAlerts: 1,
    });
  });
  it('uses local certificate health for DNS-only domains after a failed public probe', () => {
    expect(
      getDomainHealth(
        { ...domain, mode: 'cert_only' },
        status(),
        {
          name: domain.name,
          addresses: [],
          issuer: '',
          daysLeft: 0,
          expiresAt: 0,
          checkedAt: 0,
          error: 'connection refused',
        },
        true,
        NOW,
      ),
    ).toBe('healthy');
  });
});
