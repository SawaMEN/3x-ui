import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import MainNodeDNS01Setup from '@/components/MainNodeDNS01Setup';
import LocalCertificateAssistant from '@/pages/inbounds/form/security/LocalCertificateAssistant';
import { defaultDomainConfig, defaultDomain } from '@/schemas/domain-front';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

const mocks = vi.hoisted(() => ({ status: vi.fn(), sites: vi.fn() }));
vi.mock('@/api/queries/useDomainFrontStatus', () => ({ useDomainFrontStatus: mocks.status }));
vi.mock('@/api/queries/useManagedSniDomains', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/queries/useManagedSniDomains')>()),
  useManagedSniDomains: mocks.sites,
}));
beforeEach(() => {
  const configured = {
    ...defaultDomainConfig,
    dns01Provider: 'cloudflare',
    dns01Zone: 'example.org',
    dns01ZoneId: 'a'.repeat(32),
  };
  mocks.status.mockReturnValue({
    data: { config: configured },
    refetch: vi.fn().mockResolvedValue({ data: { config: configured } }),
  });
  mocks.sites.mockReturnValue({ data: [], refetch: vi.fn() });
  vi.mocked(HttpUtil.get).mockResolvedValue(new Msg(true, '', { configured: true }));
  vi.mocked(HttpUtil.post).mockClear();
});
function RemoteForm() {
  const form = useForm({
    defaultValues: { protocol: 'trojan', nodeId: 5, streamSettings: { security: 'tls' } },
  });
  return (
    <FormProvider {...form}>
      <LocalCertificateAssistant sni="vpn.example.org" />
    </FormProvider>
  );
}
async function requestIssuance() {
  fireEvent.click(screen.getByRole('button', { name: 'Issue certificate (DNS-01)' }));
  const modal = await screen.findByRole('dialog');
  fireEvent.change(within(modal).getByRole('textbox'), { target: { value: 'admin@example.org' } });
  fireEvent.click(within(modal).getByRole('checkbox'));
  const issue = within(modal).getByRole('button', { name: 'Issue via DNS-01' });
  await waitFor(() => expect(issue).toHaveProperty('disabled', false));
  fireEvent.click(issue);
  return modal;
}
describe('main-node certificate setup', () => {
  it('offers issuance from a remote inbound and sends the main-node request with consent', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValue(
      new Msg(true, '', { domain: 'vpn.example.org', state: 'pending' }),
    );
    renderWithProviders(<RemoteForm />);
    expect(
      screen.getByRole('button', { name: 'Transfer certificate from main node' }),
    ).toHaveProperty('disabled', true);
    await requestIssuance();
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/manager/domains/local-certificate',
        { domain: 'vpn.example.org', email: 'admin@example.org', acceptTerms: true },
        expect.anything(),
      ),
    );
    expect(await screen.findByRole('button', { name: 'Check issuance (pending)' })).toHaveProperty(
      'disabled',
      false,
    );
  });
  it('enables explicit transfer when the main-node certificate becomes available', async () => {
    mocks.sites.mockReturnValue({
      data: [
        {
          name: 'vpn.example.org',
          source: 'gateway',
          certFile: '/main/cert.pem',
          keyFile: '/main/key.pem',
        },
      ],
      refetch: vi.fn(),
    });
    vi.mocked(HttpUtil.post).mockImplementation(async (url) =>
      url.endsWith('/distribute')
        ? new Msg(true, '', {
            domain: 'vpn.example.org',
            certFile: '/node/cert.pem',
            keyFile: '/node/key.pem',
            expiresAt: 2000000000,
          })
        : new Msg(false, '', null),
    );
    renderWithProviders(<RemoteForm />);
    const transfer = screen.getByRole('button', { name: 'Transfer certificate from main node' });
    expect(transfer).toHaveProperty('disabled', false);
    expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/distribute'))).toBe(
      false,
    );
    fireEvent.click(transfer);
    expect(
      await screen.findByText('Certificate installed on the selected node. Save the inbound.'),
    ).toBeTruthy();
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/domains/certificates/distribute',
      { nodeId: 5, domain: 'vpn.example.org', rsaOnly: false },
      expect.anything(),
    );
  });
  it('blocks issuance before DNS-01 is configured instead of showing a backend error', async () => {
    mocks.status.mockReturnValue({
      data: { config: defaultDomainConfig },
      refetch: vi.fn().mockResolvedValue({ data: { config: defaultDomainConfig } }),
    });
    vi.mocked(HttpUtil.get).mockResolvedValue(new Msg(true, '', { configured: false }));
    renderWithProviders(<RemoteForm />);
    fireEvent.click(screen.getByRole('button', { name: 'Issue certificate (DNS-01)' }));
    const modal = await screen.findByRole('dialog');
    fireEvent.change(within(modal).getByRole('textbox'), {
      target: { value: 'admin@example.org' },
    });
    fireEvent.click(within(modal).getByRole('checkbox'));
    expect(within(modal).getByRole('button', { name: 'Issue via DNS-01' })).toHaveProperty(
      'disabled',
      true,
    );
    expect(within(modal).getByRole('button', { name: 'Configure DNS-01' })).toHaveProperty(
      'disabled',
      false,
    );
    expect(
      vi
        .mocked(HttpUtil.post)
        .mock.calls.some(([url]) => url === '/panel/api/manager/domains/local-certificate'),
    ).toBe(false);
  });
  it('suggests the DNS zone and resumes issuance automatically after saving credentials', async () => {
    mocks.status.mockReturnValue({
      data: { config: defaultDomainConfig },
      refetch: vi.fn().mockResolvedValue({ data: { config: defaultDomainConfig } }),
    });
    let tokenSaved = false;
    vi.mocked(HttpUtil.get).mockImplementation(
      async () => new Msg(true, '', { configured: tokenSaved }),
    );
    vi.mocked(HttpUtil.post).mockImplementation(async (url) => {
      if (url === '/panel/api/domains/dns/credential') tokenSaved = true;
      if (url === '/panel/api/manager/domains/local-certificate')
        return new Msg(true, '', { domain: 'vpn.example.org', state: 'pending' });
      return new Msg(true, '', {});
    });
    renderWithProviders(<RemoteForm />);
    fireEvent.click(screen.getByRole('button', { name: 'Issue certificate (DNS-01)' }));
    const issuance = await screen.findByRole('dialog');
    fireEvent.change(within(issuance).getByRole('textbox'), {
      target: { value: 'admin@example.org' },
    });
    fireEvent.click(within(issuance).getByRole('checkbox'));
    fireEvent.click(within(issuance).getByRole('button', { name: 'Configure DNS-01' }));
    const setup = await screen.findByRole('dialog', { name: 'Configure DNS-01 on main server' });
    expect(within(setup).getByRole('textbox', { name: 'DNS zone' })).toHaveProperty(
      'value',
      'example.org',
    );
    expect(within(setup).getByLabelText('DNS zone help')).toBeTruthy();
    fireEvent.change(within(setup).getByRole('textbox', { name: 'Cloudflare Zone ID' }), {
      target: { value: 'a'.repeat(32) },
    });
    fireEvent.change(within(setup).getByLabelText('DNS API token'), {
      target: { value: 'secret-token-for-dns-only' },
    });
    const save = within(setup).getByRole('button', { name: 'Save DNS-01' });
    await waitFor(() => expect(save).toHaveProperty('disabled', false));
    fireEvent.click(save);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/manager/domains/local-certificate',
        { domain: 'vpn.example.org', email: 'admin@example.org', acceptTerms: true },
        expect.anything(),
      ),
    );
  });
  it('preserves freshly loaded domains and stores the token separately from the configuration', async () => {
    const latest = {
      ...defaultDomainConfig,
      domains: [{ ...defaultDomain, name: 'new.example.org' }],
      dns01Propagation: 30,
    };
    mocks.status.mockReturnValue({
      data: { config: defaultDomainConfig },
      refetch: vi.fn().mockResolvedValue({ data: { config: latest } }),
    });
    let configured = false;
    vi.mocked(HttpUtil.get).mockImplementation(async () => new Msg(true, '', { configured }));
    vi.mocked(HttpUtil.post).mockImplementation(async (url) => {
      if (url === '/panel/api/domains/dns/credential') configured = true;
      return new Msg(true, '', {});
    });
    renderWithProviders(<MainNodeDNS01Setup />);
    fireEvent.click(screen.getByRole('button', { name: 'Configure DNS-01' }));
    const modal = await screen.findByRole('dialog');
    fireEvent.change(within(modal).getByRole('textbox', { name: 'DNS zone' }), {
      target: { value: 'example.org' },
    });
    fireEvent.change(within(modal).getByRole('textbox', { name: 'Cloudflare Zone ID' }), {
      target: { value: 'a'.repeat(32) },
    });
    fireEvent.change(within(modal).getByLabelText('DNS API token'), {
      target: { value: 'secret-token-for-dns-only' },
    });
    const save = within(modal).getByRole('button', { name: 'Save DNS-01' });
    await waitFor(() => expect(save).toHaveProperty('disabled', false));
    fireEvent.click(save);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/domains/apply',
        {
          ...latest,
          dns01Provider: 'cloudflare',
          dns01Zone: 'example.org',
          dns01ZoneId: 'a'.repeat(32),
        },
        expect.anything(),
      ),
    );
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/domains/dns/credential',
      { provider: 'cloudflare', token: 'secret-token-for-dns-only' },
      expect.anything(),
    );
  });
});
