import type {
  DomainFrontCertificate,
  DomainFrontCheck,
  DomainFrontStatus,
} from '@/generated/zod';
import type { DomainConfigForm, DomainForm } from '@/schemas/domain-front';

export type DomainHealth = 'disabled' | 'unsaved' | 'external' | 'error' | 'expiring' | 'healthy' | 'pending';

const WARNING_DAYS = 30;

export function getDomainHealth(
  domain: DomainForm,
  status: DomainFrontStatus | undefined,
  check: DomainFrontCheck | undefined,
  saved: boolean,
  now = Date.now(),
): DomainHealth {
  if (!domain.enabled) return 'disabled';
  if (!saved) return 'unsaved';
  if (check?.error) return 'error';
  if (check?.expiresAt && check.expiresAt * 1000 <= now) return 'error';
  const cert = status?.certificates.find((item) => item.name === domain.name);
  if (cert?.state === 'expired' || cert?.state === 'error' || cert?.error) return 'error';
  if (cert?.expiresAt && cert.expiresAt * 1000 <= now) return 'error';
  if (
    (cert?.expiresAt && cert.expiresAt * 1000 <= now + WARNING_DAYS * 86400000) ||
    (check?.expiresAt && check.expiresAt * 1000 <= now + WARNING_DAYS * 86400000)
  )
    return 'expiring';
  if (check?.expiresAt || cert?.state === 'valid') return 'healthy';
  if (domain.mode === 'shared_sni') return 'external';
  return 'pending';
}

export function certificateNeedsAttention(
  cert: DomainFrontCertificate,
  now = Date.now(),
): 'error' | 'expiring' | 'healthy' | 'pending' {
  if (cert.state === 'error' || cert.state === 'expired' || cert.error) return 'error';
  if (cert.expiresAt && cert.expiresAt * 1000 <= now) return 'error';
  if (cert.expiresAt && cert.expiresAt * 1000 <= now + WARNING_DAYS * 86400000)
    return 'expiring';
  if (cert.state === 'valid') return 'healthy';
  return 'pending';
}

export function getDomainSummary(
  draft: DomainConfigForm,
  status: DomainFrontStatus | undefined,
  checks: Record<string, DomainFrontCheck>,
  now = Date.now(),
) {
  const saved = status?.config.domains ?? [];
  const health = draft.domains.map((domain) =>
    getDomainHealth(
      domain,
      status,
      checks[domain.name],
      saved.some((item) => item.name === domain.name && JSON.stringify(item) === JSON.stringify(domain)),
      now,
    ),
  );
  const certificates = [...(status?.certificates ?? []), ...(status?.existingCertificates ?? [])];
  const certAlerts = certificates.filter((cert) => {
    const result = certificateNeedsAttention(cert, now);
    return result === 'error' || result === 'expiring';
  });
  return {
    total: draft.domains.length,
    active: draft.domains.filter((d) => d.enabled).length,
    healthy: health.filter((item) => item === 'healthy').length,
    attention: health.filter((item) => item === 'error' || item === 'expiring').length,
    pending: health.filter((item) => item === 'pending' || item === 'unsaved').length,
    certificateAlerts: certAlerts.length,
    expiringCertificates: certAlerts.filter((cert) => certificateNeedsAttention(cert, now) === 'expiring').length,
    failedCertificates: certAlerts.filter((cert) => certificateNeedsAttention(cert, now) === 'error').length,
  };
}
