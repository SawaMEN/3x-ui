import { MemoryRouter } from 'react-router';
import { AllSetting } from '@/models/setting';
import { HttpUtil, Msg } from '@/utils';
import SubscriptionDomains from '@/pages/settings/SubscriptionDomains';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setupHttp } from '@/api/http-init';
import DomainFrontTab from '@/pages/settings/DomainFrontTab';
import AdditionalSettingsTab from '@/pages/settings/AdditionalSettingsTab';
import { defaultDomainConfig, defaultDomain, DomainConfigFormSchema } from '@/schemas/domain-front';
import type { DomainConfigForm } from '@/schemas/domain-front';
import { server } from './msw/server';
import './setup.msw';
import { chooseSelectOption, renderWithProviders } from './test-utils';

function fixture(existing = false) {
  const config: DomainConfigForm = {
    ...defaultDomainConfig,
    enabled: true,
    acceptTerms: true,
    email: 'old@example.com',
    subscriptionDomain: 'sub.example.com',
    domains: [{ ...defaultDomain, name: 'sub.example.com', mode: 'sub_link_only' }],
  };
  const status = () => ({
    config,
    running: true,
    existingCertificates: existing
      ? [
          {
            name: 'panel',
            mode: 'auto',
            state: 'valid',
            autoRenew: true,
            source: 'panel',
            renewal: 'acme.sh',
            certFile: '/root/cert/existing/fullchain.pem',
            keyFile: '/root/cert/existing/privkey.pem',
            issuer: 'Installer issuer',
            expiresAt: Math.floor(Date.now() / 1000) + 40 * 86400,
            daysLeft: 40,
            error: '',
          },
        ]
      : [],
    error: '',
    certificates: config.domains.length
      ? [
          {
            name: config.domains[0].name,
            mode: 'auto',
            state: 'valid',
            autoRenew: true,
            issuer: 'Test issuer',
            expiresAt: Math.floor(Date.now() / 1000) + 45 * 86400,
            daysLeft: 45,
            error: '',
            source: 'panel',
            renewal: 'acme.sh',
            certFile: '/root/cert/sub.example.com/fullchain.pem',
            keyFile: '/root/cert/sub.example.com/privkey.pem',
          },
        ]
      : [],
  });
  let applied: DomainConfigForm | undefined;
  let reads = 0;
  const checked: string[] = [];
  server.use(
    http.get('*/panel/api/domains/certificates/events', () =>
      HttpResponse.json({ success: true, obj: [] }),
    ),
    http.get('*/panel/api/domains/certificates', () =>
      HttpResponse.json({
        success: true,
        obj: {
          items: status().certificates.map((cert) => ({
            id: cert.name,
            name: cert.name,
            names: [cert.name],
            usedBy: ['Subscription'],
            manager: cert.renewal,
            challenge: 'http-01',
            ports: [80],
            source: cert.source,
            state: cert.state,
            issuer: cert.issuer,
            expiresAt: cert.expiresAt,
            daysLeft: cert.daysLeft,
            autoRenew: cert.autoRenew,
            certFile: cert.certFile,
          })),
          firewallEnabled: true,
          firewallBackend: 'nftables',
        },
      }),
    ),
    http.get('*/panel/api/domains/status', () => {
      reads++;
      return HttpResponse.json({ success: true, obj: status() });
    }),
    http.get('*/panel/api/inbounds/options', () =>
      HttpResponse.json({
        success: true,
        obj: [
          {
            id: 12,
            enable: true,
            remark: 'WebSocket',
            network: 'ws',
            security: 'none',
            port: 15001,
            protocol: 'vless',
            tag: 'ws',
            tlsFlowCapable: false,
            ssMethod: '',
          },
        ],
      }),
    ),
    http.post('*/panel/api/domains/preflight', () =>
      HttpResponse.json({
        success: true,
        obj: { ready: true, checkedAt: 1, checks: [] },
      }),
    ),
    http.post('*/panel/api/domains/apply', async ({ request }) => {
      applied = DomainConfigFormSchema.parse(await request.json());
      return HttpResponse.json({ success: true, obj: { ...status(), config: applied } });
    }),
    http.post('*/panel/api/domains/check', async ({ request }) => {
      const body = (await request.json()) as { name: string };
      checked.push(body.name);
      return HttpResponse.json({
        success: true,
        obj: {
          name: body.name,
          addresses: ['203.0.113.8'],
          checkedAt: 1,
          expiresAt: Math.floor(Date.now() / 1000) + 80 * 86400,
          daysLeft: 80,
          issuer: 'CDN edge issuer',
          error: '',
        },
      });
    }),
  );
  return {
    config,
    checked,
    get applied() {
      return applied;
    },
    get reads() {
      return reads;
    },
  };
}

async function openDomainSection(name: 'Domains' | 'Certificates' | 'SNI and nodes' | 'Settings') {
  await screen.findByText('Domain control center');
  const navigation = document.querySelector('.domain-manager-nav');
  const option = Array.from(navigation?.querySelectorAll('label') ?? []).find(
    (element) => element.textContent?.trim() === name,
  );
  if (!option) throw new Error('Missing domain section: ' + name);
  fireEvent.click(option);
}

async function findDomainEmail() {
  await openDomainSection('Settings');
  return screen.findByDisplayValue('old@example.com');
}

describe('domain and TLS dashboard', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.removeItem('xui-domain-auto-check');
    window.X_UI_BASE_PATH = 'http://localhost';
    setupHttp();
  });
  afterEach(() => {
    delete window.X_UI_BASE_PATH;
    setupHttp();
  });
  it('opens the single TLS setup form from a contextual prompt for CertMagic domains', async () => {
    const f = fixture();
    f.config.email = '';
    f.config.acceptTerms = false;
    f.config.domains[0].tls = 'certmagic';
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Configure TLS' }));
    expect(screen.getAllByRole('textbox', { name: 'ACME email' })).toHaveLength(1);
    expect(screen.getAllByRole('checkbox', { name: /Let.s Encrypt/ })).toHaveLength(1);
    expect(screen.queryByRole('combobox', { name: 'Domain validation method' })).toBeNull();
    expect(f.applied).toBeUndefined();
  });

  it('adds a DNS-only certificate with the appropriate defaults and opens DNS setup', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    await openDomainSection('Certificates');
    fireEvent.click(screen.getByRole('button', { name: 'Add DNS-01 certificate' }));
    fireEvent.change(screen.getByPlaceholderText('sub.example.com'), {
      target: { value: '*.example.org' },
    });
    await waitFor(() =>
      expect(screen.queryByRole('checkbox', { name: 'Use in subscription links' })).toBeNull(),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Configure DNS-01' }));
    expect(await screen.findByRole('combobox', { name: 'Domain validation method' })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() =>
      expect(f.applied?.domains[1]).toMatchObject({
        name: '*.example.org',
        mode: 'cert_only',
        tls: 'certmagic',
        inboundIds: [],
        panel: false,
      }),
    );
    expect(f.applied?.subscriptionDomain).toBe('sub.example.com');
  });

  it('loads DNS credentials only when DNS settings are opened and preserves unsaved values', async () => {
    const f = fixture();
    f.config.dns01Provider = 'cloudflare';
    f.config.dns01Zone = 'example.com';
    let reads = 0;
    server.use(
      http.get('*/panel/api/domains/dns/credential', () => {
        reads++;
        return HttpResponse.json({ success: true, obj: { configured: true } });
      }),
    );
    renderWithProviders(<DomainFrontTab />);
    await openDomainSection('Settings');
    expect(reads).toBe(0);
    fireEvent.click(screen.getByText('Automatic DNS-01 validation'));
    const zone = await screen.findByRole('textbox', { name: 'DNS zone' });
    fireEvent.change(zone, { target: { value: 'edited.example.com' } });
    await waitFor(() => expect(reads).toBe(1));
    await openDomainSection('Domains');
    await openDomainSection('Settings');
    expect(await screen.findByDisplayValue('edited.example.com')).toBeTruthy();
    expect(f.applied).toBeUndefined();
  });

  it('offers one edit action per cover site and keeps its draft in the editor', async () => {
    const f = fixture();
    f.config.domains[0].mode = 'sni_only';
    f.config.subscriptionDomain = '';
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    expect(screen.queryByRole('button', { name: 'Change cover site' })).toBeNull();
    expect(await screen.findByRole('combobox', { name: 'Website template' })).toBeTruthy();
    expect(f.applied).toBeUndefined();
  });

  it.each(['Certificates', 'SNI and nodes'] as const)(
    'receives a cover-site certificate from another node through %s without changing the domain draft',
    async (section) => {
      const f = fixture();
      let nodeReads = 0;
      let enrollment: unknown;
      let fleet: unknown[] = [];
      server.use(
        http.get('*/panel/api/nodes/list', () => {
          nodeReads++;
          return HttpResponse.json({
            success: true,
            obj: [
              {
                id: 2,
                name: 'Germany',
                enable: true,
                scheme: 'https',
                tlsVerifyMode: 'verify',
                status: 'online',
              },
            ],
          });
        }),
        http.get('*/panel/api/domains/cover-sites', () =>
          HttpResponse.json({
            success: true,
            obj: [{ nodeId: 2, domain: 'web.example.org' }],
          }),
        ),
        http.get('*/panel/api/domains/certificates/fleet', () =>
          HttpResponse.json({ success: true, obj: fleet }),
        ),
        http.post('*/panel/api/domains/certificates/fleet', async ({ request }) => {
          enrollment = await request.json();
          fleet = [
            {
              sourceNodeId: 2,
              domain: 'web.example.org',
              targets: [{ nodeId: 0, state: 'pending', expiresAt: 0 }],
            },
          ];
          return HttpResponse.json({ success: true, obj: fleet });
        }),
      );
      renderWithProviders(<DomainFrontTab />);
      await openDomainSection(section);
      expect(nodeReads).toBe(0);
      fireEvent.click(screen.getByRole('button', { name: 'Use certificate from another node' }));
      const source = await screen.findByRole('combobox', { name: 'Certificate source site' });
      await waitFor(() => expect(source).toHaveProperty('disabled', false));
      fireEvent.mouseDown(source);
      fireEvent.click(await screen.findByText('web.example.org — Germany'));
      expect(enrollment).toBeUndefined();
      fireEvent.click(screen.getByRole('button', { name: 'Connect and update automatically' }));
      await waitFor(() =>
        expect(enrollment).toEqual({
          sourceNodeId: 2,
          domain: 'web.example.org',
          nodeIds: [0],
        }),
      );
      expect(await screen.findByText(/Automatic delivery connected/)).toBeTruthy();
      expect(f.applied).toBeUndefined();
      expect(nodeReads).toBe(1);
    },
  );

  it('shows certificate lifetime, preserves edited settings on refresh and applies the edited configuration', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    expect(await screen.findByText('sub.example.com')).toBeTruthy();
    await openDomainSection('Certificates');
    expect(await screen.findByText(/45 days/)).toBeTruthy();
    const email = await findDomainEmail();
    fireEvent.change(email, { target: { value: 'edited@example.com' } });
    const initialReads = f.reads;
    fireEvent.click(screen.getByRole('button', { name: /Refresh status/ }));
    await waitFor(() => expect(f.reads).toBeGreaterThan(initialReads));
    expect(screen.getByDisplayValue('edited@example.com')).toBeTruthy();
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() => expect(f.applied?.email).toBe('edited@example.com'));
    await waitFor(() =>
      expect(
        (screen.getByText('Save and apply').closest('button') as HTMLButtonElement).disabled,
      ).toBe(true),
    );
  });

  it('shares existing installer certificates and the managed subscription address across settings', async () => {
    const f = fixture(true);
    f.config.domains[0].mode = 'cdn';
    f.config.acceptTerms = false;
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <DomainFrontTab />
        <SubscriptionDomains
          allSetting={new AllSetting({ subPublicURL: '', subCDNEnable: false })}
          updateSetting={update}
        />
      </MemoryRouter>,
    );
    await openDomainSection('Certificates');
    fireEvent.click(await screen.findByText('Existing certificates'));
    expect(await screen.findByText(/Installer issuer/)).toBeTruthy();
    expect(screen.getByText('/root/cert/existing/fullchain.pem')).toBeTruthy();
    const address = (await screen.findByRole('textbox', {
      name: 'Public subscription address',
    })) as HTMLInputElement;
    await waitFor(() => expect(address.placeholder).toBe('https://sub.example.com'));
    expect(address.disabled).toBe(false);
    expect(
      screen.getByRole('switch', { name: 'Subscription through CDN' }).getAttribute('aria-checked'),
    ).toBe('true');
    expect(screen.queryByRole('textbox', { name: 'Caddy reverse proxy configuration' })).toBeNull();
    expect(update).not.toHaveBeenCalled();
  });

  it('follows the gateway HTTPS port unless a NAT mapping is explicitly enabled', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    await findDomainEmail();
    fireEvent.click(screen.getByText('Advanced network and testing settings'));
    const gateway = await screen.findByRole('spinbutton', { name: 'HTTPS gateway port' });
    expect(
      screen.queryByRole('spinbutton', { name: 'External HTTPS port (NAT / proxy)' }),
    ).toBeNull();
    fireEvent.change(gateway, { target: { value: '8443' } });
    fireEvent.click(
      screen.getByRole('checkbox', { name: 'Use a different external port (NAT / proxy)' }),
    );
    const external = screen.getByRole('spinbutton', { name: 'External HTTPS port (NAT / proxy)' });
    expect((external as HTMLInputElement).value).toBe('8443');
    fireEvent.change(external, { target: { value: '9443' } });
    fireEvent.change(gateway, { target: { value: '7443' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(f.applied).toMatchObject({ httpsPort: 7443, publicPort: 9443 }));
  });

  it.each([
    ['certmagic', 'Let’s Encrypt — issue and renew with CertMagic'],
    ['existing', 'Existing — panel, subscription or installer'],
  ])('allows explicitly selecting %s per domain', async (tlsMode, title) => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await screen.findByRole('dialog');
    fireEvent.click(screen.getByText('Advanced: TLS and status'));
    chooseSelectOption('tls', title);
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(
        document.querySelector('.domain-config-table .ant-table-tbody')?.textContent,
      ).toContain('sub.example.com'),
    );
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() => expect(f.applied?.domains[0].tls).toBe(tlsMode));
  });

  it('suggests a subscription-only domain, explains it and checks it automatically after apply', async () => {
    const f = fixture();
    f.config.domains = [];
    f.config.subscriptionDomain = '';
    renderWithProviders(<DomainFrontTab />);
    await findDomainEmail();
    await openDomainSection('Domains');
    fireEvent.click(screen.getByRole('button', { name: /Add domain/ }));
    const input = screen.getByPlaceholderText('sub.example.com');
    const help = document.querySelector('label[for="name"] .anticon-question-circle');
    expect(help).toBeTruthy();
    fireEvent.mouseEnter(help!);
    expect(await screen.findByText(/Domain name only, such as/)).toBeTruthy();
    fireEvent.mouseLeave(help!);
    expect(
      (screen.getByRole('checkbox', { name: 'Use in subscription links' }) as HTMLInputElement)
        .checked,
    ).toBe(true);
    fireEvent.change(input, { target: { value: 'new.example.com' } });
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(
        document.querySelector('.domain-config-table .ant-table-tbody')?.textContent,
      ).toContain('new.example.com'),
    );
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() => expect(f.applied?.subscriptionDomain).toBe('new.example.com'));
    expect(f.applied?.domains[0]).toMatchObject({
      mode: 'sub_link_only',
      tls: 'auto',
      panel: false,
      inboundIds: [],
    });
    await waitFor(() => expect(f.checked).toEqual(['new.example.com']));
  });

  it('audits saved domains in bulk and filters the domain table without modifying configuration', async () => {
    const f = fixture();
    f.config.domains.push({ ...defaultDomain, name: 'cdn.example.org', mode: 'cdn' });
    renderWithProviders(<DomainFrontTab />);
    await findDomainEmail();
    await openDomainSection('Domains');
    fireEvent.click(screen.getByRole('button', { name: /Audit all DNS\/TLS/ }));
    await waitFor(() =>
      expect(f.checked).toEqual(expect.arrayContaining(['sub.example.com', 'cdn.example.org'])),
    );
    const search = screen.getByRole('searchbox', { name: 'Search domains' });
    fireEvent.change(search, { target: { value: 'cdn.example.org' } });
    await waitFor(() => {
      const table = document.querySelector('.domain-config-table .ant-table-tbody');
      expect(table?.textContent).toContain('cdn.example.org');
      expect(table?.textContent).not.toContain('sub.example.com');
    });
    expect(f.applied).toBeUndefined();
  });

  it('checks the unsaved draft without applying it', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    await findDomainEmail();
    fireEvent.click(screen.getByRole('button', { name: 'Check readiness' }));
    expect(await screen.findByText('Configuration is ready to apply')).toBeTruthy();
    expect(f.applied).toBeUndefined();
  });

  it('blocks applying an invalid draft reported by the server preflight', async () => {
    const f = fixture();
    server.use(
      http.post('*/panel/api/domains/preflight', () =>
        HttpResponse.json({
          success: true,
          obj: {
            ready: false,
            checkedAt: 1,
            checks: [
              { code: 'telemt_port_conflict', severity: 'error', message: 'TCP 443 is in use' },
            ],
          },
        }),
      ),
    );
    renderWithProviders(<DomainFrontTab />);
    const email = await findDomainEmail();
    fireEvent.change(email, { target: { value: 'next@example.com' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    expect(
      (await screen.findAllByText('Configuration contains blocking errors')).length,
    ).toBeGreaterThan(0);
    expect(f.applied).toBeUndefined();
  });

  it('checks DNS automatically without an extra toggle', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    const email = await findDomainEmail();
    fireEvent.change(email, { target: { value: 'changed@example.com' } });
    expect(screen.queryByRole('switch', { name: 'Check DNS/TLS after apply' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(f.checked).toEqual(['sub.example.com']));
  });

  it('preserves a domain draft when navigating between additional settings tabs', async () => {
    const f = fixture();
    server.use(
      http.get('*/panel/api/setting/cores', () =>
        HttpResponse.json({
          success: true,
          obj: { cores: [{ type: 'sing-box', installed: true, running: true, selected: true }] },
        }),
      ),
      http.get('*/panel/api/server/status', () =>
        HttpResponse.json({ success: true, obj: { core: { running: 'sing-box' } } }),
      ),
    );
    renderWithProviders(
      <MemoryRouter initialEntries={['/settings#advanced/domains']}>
        <AdditionalSettingsTab
          allSetting={new AllSetting()}
          updateSetting={vi.fn()}
          settingsDirty={false}
          saveSettings={vi.fn().mockResolvedValue(true)}
        />
      </MemoryRouter>,
    );
    fireEvent.change(await findDomainEmail(), {
      target: { value: 'draft@example.com' },
    });
    fireEvent.click(screen.getByRole('tab', { name: /Proxy Core/ }));
    await screen.findByText('Running');
    fireEvent.click(screen.getByRole('tab', { name: /Domains and TLS/ }));
    await openDomainSection('Settings');
    expect(screen.getByDisplayValue('draft@example.com')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(f.applied?.email).toBe('draft@example.com'));
  });

  it('searches neighboring websites without enabling redirects or changing settings', async () => {
    fixture();
    // Isolate the result-rendering behavior from HTTP transport details.
    const recommend = vi.spyOn(HttpUtil, 'post').mockResolvedValueOnce(
      new Msg(true, '', {
        checkedAt: 123,
        recommended: 'https://ya.ru/',
        sites: [
          {
            url: 'https://ya.ru/',
            available: true,
            status: 200,
            pathStatus: 200,
            latencyMs: 42,
            sniLatencyMs: 19,
          },
          {
            url: 'https://www.wikipedia.org/',
            available: false,
            status: 0,
            pathStatus: 0,
            latencyMs: 0,
            sniLatencyMs: 0,
            error: 'unreachable',
          },
        ],
      }),
    );
    const update = vi.fn();
    const save = vi.fn().mockResolvedValue(true);
    renderWithProviders(
      <MemoryRouter>
        <DomainFrontTab
          allSetting={new AllSetting({ rootRedirectEnable: false, rootRedirectURL: '' })}
          updateSetting={update}
          saveSettings={save}
        />
      </MemoryRouter>,
    );
    await openDomainSection('Settings');
    fireEvent.click(screen.getByText('Domain website redirect'));
    fireEvent.click(await screen.findByRole('button', { name: 'Find neighboring websites' }));
    expect(await screen.findByText(/Fastest website: https:\/\/ya.ru\//)).toBeTruthy();
    expect(update).not.toHaveBeenCalled();
    expect(save).not.toHaveBeenCalled();
    const choose = screen.getAllByRole('button', { name: 'Choose' });
    expect((choose[0] as HTMLButtonElement).disabled).toBe(false);
    expect((choose[1] as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(choose[0]);
    expect(update).toHaveBeenCalledWith({ rootRedirectURL: 'https://ya.ru/' });
    expect(update).not.toHaveBeenCalledWith(expect.objectContaining({ rootRedirectEnable: true }));
    expect(save).not.toHaveBeenCalled();
    expect(recommend).toHaveBeenCalledWith(
      '/panel/api/domains/recommend',
      {},
      expect.objectContaining({ silent: true, timeout: 75_000 }),
    );
  });

  it('does not enable redirect if no destination is reachable', async () => {
    fixture();
    const recommend = vi.spyOn(HttpUtil, 'post').mockResolvedValueOnce(
      new Msg(true, '', {
        checkedAt: 123,
        recommended: '',
        sites: [
          {
            url: 'https://ya.ru/',
            available: false,
            status: 0,
            pathStatus: 0,
            latencyMs: 0,
            sniLatencyMs: 0,
          },
        ],
      }),
    );
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <DomainFrontTab
          allSetting={new AllSetting({ rootRedirectEnable: false, rootRedirectURL: '' })}
          updateSetting={update}
        />
      </MemoryRouter>,
    );
    await openDomainSection('Settings');
    fireEvent.click(screen.getByText('Domain website redirect'));
    fireEvent.click(await screen.findByRole('button', { name: 'Find neighboring websites' }));
    expect(
      (await screen.findAllByText(/No suitable neighboring websites were found/)).length,
    ).toBeGreaterThan(0);
    expect(update).not.toHaveBeenCalled();
    expect(recommend).toHaveBeenCalledWith(
      '/panel/api/domains/recommend',
      {},
      expect.objectContaining({ silent: true, timeout: 75_000 }),
    );
  });

  it('saves root redirect settings from the top of the domain panel', async () => {
    fixture();
    const save = vi.fn().mockResolvedValue(true);
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <DomainFrontTab
          allSetting={
            new AllSetting({ rootRedirectEnable: true, rootRedirectURL: 'https://example.com/' })
          }
          updateSetting={update}
          settingsDirty
          saveSettings={save}
        />
      </MemoryRouter>,
    );
    await findDomainEmail();
    fireEvent.click(screen.getByText('Domain website redirect'));
    const destination = await screen.findByDisplayValue('https://example.com/');
    expect(destination.closest('.domain-redirect-settings')).toBeTruthy();
    fireEvent.change(destination, { target: { value: 'https://new.example.com/' } });
    expect(update).toHaveBeenCalledWith({ rootRedirectURL: 'https://new.example.com/' });
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
  });

  it('does not apply domain changes if saving manual settings fails', async () => {
    const f = fixture();
    const save = vi.fn().mockResolvedValue(false);
    renderWithProviders(<DomainFrontTab settingsDirty saveSettings={save} />);
    fireEvent.change(await findDomainEmail(), {
      target: { value: 'changed@example.com' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
    expect(f.applied).toBeUndefined();
  });

  it('clears panel and VPN bindings when a domain becomes subscription-only', async () => {
    const f = fixture();
    f.config.domains[0] = {
      ...defaultDomain,
      name: 'sub.example.com',
      mode: 'direct',
      panel: true,
      inboundIds: [12],
    };
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await screen.findByRole('dialog');
    chooseSelectOption('mode', 'Subscription only');
    await waitFor(() =>
      expect(screen.queryByRole('checkbox', { name: 'Allow admin panel access' })).toBeNull(),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    await waitFor(() =>
      expect(
        document.querySelector('.domain-config-table .ant-table-tbody')?.textContent,
      ).toContain('Subscription only'),
    );
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() =>
      expect(f.applied?.domains[0]).toMatchObject({
        mode: 'sub_link_only',
        panel: false,
        inboundIds: [],
        origin: '',
        tls: 'auto',
      }),
    );
  });

  it('does not offer panel access through public gateway domains', async () => {
    const f = fixture();
    f.config.domains[0] = {
      ...defaultDomain,
      name: 'sub.example.com',
      mode: 'direct',
      panel: true, // legacy API payload
    };
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    await screen.findByRole('dialog');
    expect(screen.queryByRole('checkbox', { name: 'Allow admin panel access' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Done' }));
    fireEvent.change(await findDomainEmail(), {
      target: { value: 'private@example.com' },
    });
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() => expect(f.applied?.email).toBe('private@example.com'));
    expect(f.applied?.domains[0].panel).not.toBe(true);
  });

  it('shows external CDN TLS separately from the local origin certificate', async () => {
    fixture();
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'More actions' }));
    fireEvent.click(await screen.findByText('Check DNS/TLS'));
    expect(await screen.findByText(/Public TLS certificate: CDN edge issuer/)).toBeTruthy();
    expect(screen.getByText('DNS addresses: 203.0.113.8')).toBeTruthy();
    await openDomainSection('Certificates');
    expect(await screen.findByText('Test issuer')).toBeTruthy();
  });
});
