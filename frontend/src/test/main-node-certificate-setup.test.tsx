import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, screen, waitFor, within } from '@testing-library/react';
import { useState } from 'react';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import MainNodeDNS01Setup from '@/components/MainNodeDNS01Setup';
import LocalCertificateAssistant from '@/pages/inbounds/form/security/LocalCertificateAssistant';
import { defaultDomainConfig, defaultDomain } from '@/schemas/domain-front';
import { HttpUtil, Msg } from '@/utils';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

const mocks = vi.hoisted(() => ({ status: vi.fn(), sites: vi.fn() }));
vi.mock('@/api/queries/useDomainFrontStatus', () => ({ useDomainFrontStatus: mocks.status }));
vi.mock('@/api/queries/useManagedSniDomains', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api/queries/useManagedSniDomains')>()),
  useManagedSniDomains: mocks.sites,
}));
const configured = {
  ...defaultDomainConfig,
  dns01Provider: 'cloudflare',
  dns01Zone: 'example.org',
  dns01ZoneId: 'a'.repeat(32),
};
let preparationState = 'not_requested';
const validCertificate = {
  domain: 'vpn.example.org',
  nodeId: 5,
  state: 'valid',
  certFile: '/node-5/current/fullchain.pem',
  keyFile: '/node-5/current/privkey.pem',
  expiresAt: 2000000000,
};
beforeEach(() => {
  preparationState = 'not_requested';
  mocks.status.mockReturnValue({
    data: { config: configured },
    refetch: vi.fn().mockResolvedValue({ data: { config: configured } }),
  });
  mocks.sites.mockReturnValue({ data: [], refetch: vi.fn().mockResolvedValue({ data: [] }) });
  vi.mocked(HttpUtil.get).mockImplementation(async (url, params) =>
    url.endsWith('/preparation')
      ? new Msg(true, '', {
          ...validCertificate,
          domain: (params as { domain: string }).domain,
          nodeId: (params as { nodeId: number }).nodeId,
          state: preparationState,
        })
      : new Msg(true, '', { configured: true }),
  );
  vi.mocked(HttpUtil.post).mockReset();
  vi.mocked(HttpUtil.post).mockImplementation(async () => {
    preparationState = 'pending';
    return new Msg(true, '', { domain: 'vpn.example.org', nodeId: 5, state: 'pending' });
  });
});
function RemoteForm({ custom = false }: { custom?: boolean }) {
  const form = useForm({
    defaultValues: {
      protocol: 'trojan',
      nodeId: 5,
      streamSettings: {
        security: 'tls',
        tlsSettings: {
          certificates: custom
            ? [{ useFile: true, certificateFile: '/custom/cert.pem', keyFile: '/custom/key.pem' }]
            : [],
        },
      },
    },
  });
  const values = useWatch({ control: form.control });
  const [sni, setSni] = useState('vpn.example.org');
  return (
    <FormProvider {...form}>
      <input aria-label="SNI" value={sni} onChange={(e) => setSni(e.target.value)} />
      <button onClick={() => form.setValue('nodeId', 6)}>Switch node</button>
      <LocalCertificateAssistant sni={sni} />
      <output data-testid="inbound-values">{JSON.stringify(values)}</output>
    </FormProvider>
  );
}
async function requestIssuance() {
  const button = await screen.findByRole('button', { name: 'Get certificate (DNS-01)' });
  await waitFor(() => expect(button).toHaveProperty('disabled', false));
  fireEvent.click(button);
  const modal = await screen.findByRole('dialog');
  fireEvent.change(within(modal).getByRole('textbox', { name: 'ACME email' }), {
    target: { value: 'admin@example.org' },
  });
  fireEvent.click(within(modal).getByRole('checkbox'));
  const issue = within(modal).getByRole('button', { name: 'Issue and connect' });
  await waitFor(() => expect(issue).toHaveProperty('disabled', false));
  fireEvent.click(issue);
  return modal;
}

describe('inbound DNS-01 preparation', () => {
  it('checks typed SNI without starting issuance or sending keys', async () => {
    renderWithProviders(<RemoteForm />);
    await waitFor(() =>
      expect(HttpUtil.get).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/preparation',
        { nodeId: 5, domain: 'vpn.example.org', rsaOnly: false },
        expect.objectContaining({ silent: true }),
      ),
    );
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });
  it('issues and delivers through one request and automatically fills node-local paths', async () => {
    const client = makeTestQueryClient();
    renderWithProviders(<RemoteForm />, { queryClient: client });
    await requestIssuance();
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/prepare',
        {
          nodeId: 5,
          domain: 'vpn.example.org',
          rsaOnly: false,
          email: 'admin@example.org',
          acceptTerms: true,
        },
        expect.anything(),
      ),
    );
    expect(await screen.findByText(/This form updates automatically/)).toBeTruthy();
    preparationState = 'valid';
    await act(async () => {
      await client.invalidateQueries({ queryKey: ['inbound-certificate-preparation'] });
    });
    await screen.findByText(/Certificate ready on the selected node/);
    await waitFor(() =>
      expect(screen.getByTestId('inbound-values').textContent).toContain(validCertificate.certFile),
    );
    expect(HttpUtil.post).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('button', { name: /Check issuance/ })).toBeNull();
  });
  it('reuses saved DNS-01 contact and consent in one click', async () => {
    mocks.status.mockReturnValue({
      data: { config: { ...configured, email: 'saved@example.org', acceptTerms: true } },
    });
    renderWithProviders(<RemoteForm />);
    const get = await screen.findByRole('button', { name: 'Get certificate (DNS-01)' });
    await waitFor(() => expect(get).toHaveProperty('disabled', false));
    fireEvent.click(get);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/prepare',
        { nodeId: 5, domain: 'vpn.example.org', rsaOnly: false },
        expect.anything(),
      ),
    );
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('uses an available main certificate without DNS setup or reissuing it', async () => {
    preparationState = 'available';
    renderWithProviders(<RemoteForm />);
    const use = await screen.findByRole('button', { name: 'Use certificate on this node' });
    await waitFor(() => expect(use).toHaveProperty('disabled', false));
    fireEvent.click(use);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/prepare',
        { nodeId: 5, domain: 'vpn.example.org', rsaOnly: false },
        expect.anything(),
      ),
    );
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('preserves custom paths even when a matching managed certificate appears', async () => {
    preparationState = 'valid';
    renderWithProviders(<RemoteForm custom />);
    await screen.findByText(/Custom certificate files or PEM are preserved/);
    await waitFor(() =>
      expect(HttpUtil.get).toHaveBeenCalledWith(
        '/panel/api/domains/certificates/preparation',
        expect.anything(),
        expect.anything(),
      ),
    );
    expect(screen.getByTestId('inbound-values').textContent).toContain('/custom/cert.pem');
    expect(screen.getByTestId('inbound-values').textContent).not.toContain(
      validCertificate.certFile,
    );
  });
  it('clears automatically filled paths when the selected node changes', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url, params) =>
      url.endsWith('/preparation')
        ? new Msg(true, '', {
            ...validCertificate,
            nodeId: (params as { nodeId: number }).nodeId,
            state: (params as { nodeId: number }).nodeId === 5 ? 'valid' : 'not_requested',
          })
        : new Msg(true, '', { configured: true }),
    );
    renderWithProviders(<RemoteForm />);
    await screen.findByText(/Certificate ready on the selected node/);
    await waitFor(() =>
      expect(screen.getByTestId('inbound-values').textContent).toContain(validCertificate.certFile),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Switch node' }));
    await waitFor(() =>
      expect(screen.getByTestId('inbound-values').textContent).not.toContain(
        validCertificate.certFile,
      ),
    );
  });
  it('ignores a late preparation response after switching nodes', async () => {
    preparationState = 'available';
    let complete!: (result: Msg) => void;
    vi.mocked(HttpUtil.post).mockImplementation(
      () =>
        new Promise((resolve) => {
          complete = resolve;
        }),
    );
    renderWithProviders(<RemoteForm />);
    const use = await screen.findByRole('button', { name: 'Use certificate on this node' });
    await waitFor(() => expect(use).toHaveProperty('disabled', false));
    fireEvent.click(use);
    await waitFor(() => expect(complete).toBeTypeOf('function'));
    fireEvent.click(screen.getByRole('button', { name: 'Switch node' }));
    await act(async () => {
      complete(new Msg(true, '', validCertificate));
    });
    expect(screen.getByTestId('inbound-values').textContent).not.toContain(
      validCertificate.certFile,
    );
    expect(screen.getByTestId('inbound-values').textContent).toContain('"nodeId":6');
  });
  it('blocks issuance before DNS-01 setup and resumes after credentials are saved', async () => {
    mocks.status.mockReturnValue({
      data: { config: defaultDomainConfig },
      refetch: vi.fn().mockResolvedValue({ data: { config: defaultDomainConfig } }),
    });
    let tokenSaved = false;
    vi.mocked(HttpUtil.get).mockImplementation(async (url, params) =>
      url.endsWith('/preparation')
        ? new Msg(true, '', {
            domain: (params as { domain: string }).domain,
            nodeId: 5,
            state: preparationState,
          })
        : new Msg(true, '', { configured: tokenSaved }),
    );
    vi.mocked(HttpUtil.post).mockImplementation(async (url) => {
      if (url === '/panel/api/domains/dns/credential') tokenSaved = true;
      if (url === '/panel/api/domains/certificates/prepare') {
        preparationState = 'pending';
        return new Msg(true, '', { domain: 'vpn.example.org', nodeId: 5, state: 'pending' });
      }
      return new Msg(true, '', {});
    });
    renderWithProviders(<RemoteForm />);
    const get = await screen.findByRole('button', { name: 'Get certificate (DNS-01)' });
    await waitFor(() => expect(get).toHaveProperty('disabled', false));
    fireEvent.click(get);
    const issuance = await screen.findByRole('dialog');
    fireEvent.change(within(issuance).getByRole('textbox', { name: 'ACME email' }), {
      target: { value: 'admin@example.org' },
    });
    fireEvent.click(within(issuance).getByRole('checkbox'));
    expect(within(issuance).getByRole('button', { name: 'Issue and connect' })).toHaveProperty(
      'disabled',
      true,
    );
    fireEvent.click(within(issuance).getByRole('button', { name: 'Configure DNS-01' }));
    const zone = await screen.findByRole('textbox', { name: 'DNS zone' });
    const setup = zone.closest('[role="dialog"]') as HTMLElement;
    expect(zone).toHaveProperty('value', 'example.org');
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
        '/panel/api/domains/certificates/prepare',
        {
          nodeId: 5,
          domain: 'vpn.example.org',
          rsaOnly: false,
          email: 'admin@example.org',
          acceptTerms: true,
        },
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
