import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import CertificateInventoryPanel from '@/pages/settings/CertificateInventoryPanel';
import { HttpUtil, Msg } from '@/utils';
import { defaultDomainConfig } from '@/schemas/domain-front';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

vi.mock('@/pages/settings/CertificateAuditPanel', () => ({ default: () => null }));
vi.mock('@/api/queries/useDomainFrontStatus', () => ({
  useDomainFrontStatus: () => ({
    data: {
      config: {
        ...defaultDomainConfig,
        dns01Provider: 'cloudflare',
        dns01Zone: 'site.ru',
        dns01ZoneId: 'a'.repeat(32),
        email: 'owner@site.ru',
        acceptTerms: true,
      },
    },
  }),
}));
vi.mock('@/api/queries/useManagedSniDomains', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/queries/useManagedSniDomains')>()),
  useManagedSniDomains: () => ({ data: [], refetch: vi.fn() }),
}));
let state = 'not_requested';
const item = {
  id: 'node-site',
  name: 'web.site.ru',
  names: ['web.site.ru'],
  usedBy: ['node'],
  nodeId: 3,
  nodeName: 'Finland',
  source: 'remote',
  manager: 'certmagic',
  challenge: 'dns-01',
  ports: [],
  state: 'pending',
  issuer: '',
  expiresAt: 0,
  daysLeft: 0,
  autoRenew: true,
  coverSite: true,
  canObtain: true,
};
beforeEach(() => {
  state = 'not_requested';
  vi.mocked(HttpUtil.post).mockReset();
  vi.mocked(HttpUtil.get).mockImplementation(async (url) => {
    if (url === '/panel/api/domains/certificates')
      return new Msg(true, '', {
        items: [{ ...item, state: state === 'valid' ? 'valid' : 'pending' }],
        firewallEnabled: true,
        firewallBackend: 'nft',
      });
    if (url.endsWith('/preparation'))
      return new Msg(true, '', {
        domain: item.name,
        nodeId: 3,
        state,
        certFile: state === 'valid' ? '/node/current/fullchain.pem' : undefined,
        keyFile: state === 'valid' ? '/node/current/privkey.pem' : undefined,
        expiresAt: 2000000000,
      });
    return new Msg(true, '', { configured: true });
  });
  vi.mocked(HttpUtil.post).mockImplementation(async () => {
    state = 'pending';
    return new Msg(true, '', { domain: item.name, nodeId: 3, state });
  });
});
it('obtains and applies a missing cover-site certificate from its registry row in one click', async () => {
  const client = makeTestQueryClient();
  renderWithProviders(<CertificateInventoryPanel />, { queryClient: client });
  fireEvent.click(await screen.findByRole('button', { name: 'Obtain certificate' }));
  await waitFor(
    () =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/prepare',
        { nodeId: 3, domain: 'web.site.ru', rsaOnly: false, coverSite: true },
        expect.anything(),
      ),
    { timeout: 3000 },
  );
  expect(await screen.findByText(/This form updates automatically/)).toBeTruthy();
  state = 'valid';
  await act(async () => {
    await client.invalidateQueries({ queryKey: ['inbound-certificate-preparation'] });
  });
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  await waitFor(() =>
    expect(screen.queryByRole('button', { name: 'Obtain certificate' })).toBeNull(),
  );
  expect(HttpUtil.post).toHaveBeenCalledTimes(1);
});
it('keeps failures visible and allows another attempt', async () => {
  vi.mocked(HttpUtil.post).mockResolvedValue(new Msg(false, 'DNS provider is unavailable', null));
  renderWithProviders(<CertificateInventoryPanel />);
  fireEvent.click(await screen.findByRole('button', { name: 'Obtain certificate' }));
  await screen.findByText('DNS provider is unavailable');
  expect(screen.getByRole('dialog')).toBeTruthy();
  const retry = screen.getByRole('button', { name: 'Get certificate (DNS-01)' });
  await waitFor(() => expect(retry).toHaveProperty('disabled', false));
  fireEvent.click(retry);
  await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledTimes(2));
});
