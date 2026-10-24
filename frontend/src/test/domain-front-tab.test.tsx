import { fireEvent, screen, waitFor } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { setupHttp } from '@/api/http-init';
import DomainFrontTab from '@/pages/settings/DomainFrontTab';
import { defaultDomainConfig, defaultDomain, DomainConfigFormSchema } from '@/schemas/domain-front';
import type { DomainConfigForm } from '@/schemas/domain-front';
import { server } from './msw/server';
import './setup.msw';
import { chooseSelectOption, renderWithProviders } from './test-utils';

function fixture() {
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
    error: '',
    certificates: [
      {
        name: config.domains[0].name,
        mode: 'auto',
        state: 'valid',
        autoRenew: true,
        issuer: 'Test issuer',
        expiresAt: Math.floor(Date.now() / 1000) + 45 * 86400,
        daysLeft: 45,
        error: '',
      },
    ],
  });
  let applied: DomainConfigForm | undefined;
  let reads = 0;
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
    http.post('*/panel/api/domains/check', () =>
      HttpResponse.json({
        success: true,
        obj: {
          name: 'sub.example.com',
          addresses: ['203.0.113.8'],
          checkedAt: 1,
          expiresAt: Math.floor(Date.now() / 1000) + 80 * 86400,
          daysLeft: 80,
          issuer: 'CDN edge issuer',
          error: '',
        },
      }),
    ),
  );
  return {
    config,
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
