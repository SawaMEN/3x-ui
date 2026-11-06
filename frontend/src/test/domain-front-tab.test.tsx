import { MemoryRouter } from 'react-router';
import { AllSetting } from '@/models/setting';
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
          },
        ]
      : [],
  });
  let applied: DomainConfigForm | undefined;
  let reads = 0;
  const checked: string[] = [];
  server.use(
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
  it('shows certificate lifetime, preserves edited settings on refresh and applies the edited configuration', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    const email = await screen.findByDisplayValue('old@example.com');
    expect(await screen.findByText('Test issuer')).toBeTruthy();
    expect(screen.getByText('Days remaining: 45 days')).toBeTruthy();
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
    await screen.findByDisplayValue('old@example.com');
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
    chooseSelectOption('tls', title);
    fireEvent.click(screen.getByRole('button', { name: 'OK' }));
    await waitFor(() =>
      expect(document.querySelector('.ant-table-tbody')?.textContent).toContain(title),
    );
    fireEvent.click(screen.getByText('Save and apply').closest('button')!);
    await waitFor(() => expect(f.applied?.domains[0].tls).toBe(tlsMode));
  });

  it('suggests a subscription-only domain, explains it and checks it automatically after apply', async () => {
    const f = fixture();
    f.config.domains = [];
    f.config.subscriptionDomain = '';
    renderWithProviders(<DomainFrontTab />);
    await screen.findByDisplayValue('old@example.com');
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
    fireEvent.click(screen.getByRole('button', { name: 'OK' }));
    await waitFor(() =>
      expect(document.querySelector('.ant-table-tbody')?.textContent).toContain('new.example.com'),
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

  it('checks DNS automatically without an extra toggle', async () => {
    const f = fixture();
    renderWithProviders(<DomainFrontTab />);
    const email = await screen.findByDisplayValue('old@example.com');
    fireEvent.change(email, { target: { value: 'changed@example.com' } });
    expect(screen.queryByRole('switch', { name: 'Check DNS/TLS after apply' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(f.checked).toEqual(['sub.example.com']));
  });

  it('preserves a domain draft when navigating between additional settings tabs', async () => {
    const f = fixture();
    server.use(
      http.get('*/panel/api/setting/singbox/status', () =>
        HttpResponse.json({ success: true, obj: { installed: true, running: true } }),
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
    fireEvent.change(await screen.findByDisplayValue('old@example.com'), {
      target: { value: 'draft@example.com' },
    });
    fireEvent.click(screen.getByRole('tab', { name: /Proxy Core/ }));
    await screen.findByText('Running');
    fireEvent.click(screen.getByRole('tab', { name: /Domains and TLS/ }));
    expect(screen.getByDisplayValue('draft@example.com')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Save and apply' }));
    await waitFor(() => expect(f.applied?.email).toBe('draft@example.com'));
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
    await screen.findByDisplayValue('old@example.com');
    const destination = screen.getByDisplayValue('https://example.com/');
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
    fireEvent.change(await screen.findByDisplayValue('old@example.com'), {
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
    fireEvent.click(screen.getByRole('button', { name: 'OK' }));
    await waitFor(() =>
      expect(document.querySelector('.ant-table-tbody')?.textContent).toContain(
        'Subscription only',
      ),
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

  it('shows external CDN TLS separately from the local origin certificate', async () => {
    fixture();
    renderWithProviders(<DomainFrontTab />);
    fireEvent.click(await screen.findByRole('button', { name: 'Check DNS/TLS' }));
    expect(await screen.findByText(/Public TLS certificate: CDN edge issuer/)).toBeTruthy();
    expect(screen.getByText('DNS addresses: 203.0.113.8')).toBeTruthy();
    expect(screen.getByText('Test issuer')).toBeTruthy();
  });
});
