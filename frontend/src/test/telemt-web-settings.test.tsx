import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import TelemtPage from '@/pages/telemt/TelemtPage';
import { HttpUtil } from '@/utils';

vi.mock('@/layouts/AppSidebar', () => ({ default: () => null }));
vi.mock('@/hooks/useTheme', () => ({ useTheme: () => ({ antdThemeConfig: {} }) }));
vi.setConfig({ testTimeout: 15000 });

const webProxy = {
  enabled: true,
  supported: true,
  domain: 'web.example.com',
  defaultDomain: 'web.example.com',
  link: 'tg://webproxy?server=web.example.com',
  frontend: 'caddy',
  frontendActive: true,
  carrier: 'https',
  carriers: ['websocket-lanes', 'websocket', 'https'],
  carrierNegotiationSupported: true,
  nginxInstalled: false,
  nginxActive: false,
  certificateReady: true,
  certificateFile: '/cert.pem',
  listenPort: 15080,
  port443Available: true,
  port443Owner: 'caddy',
  error: '',
};

describe('Telemt WEB settings', () => {
  it.each(['caddy', 'haproxy'] as const)(
    'preserves %s settings and reordered carriers',
    async (frontend) => {
      vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
        success: true,
        msg: '',
        obj: url.endsWith('/status')
          ? {
              installed: true,
              active: true,
              enabled: true,
              configured: true,
              version: '3.5.14',
              latestVersion: '3.5.14',
              updateAvailable: false,
              mekoEnabled: false,
              webProxy: { ...webProxy, frontend: 'nginx', port443Owner: 'nginx' },
            }
          : url.endsWith('/proxy')
            ? []
            : url.endsWith('/inbounds/options')
              ? [
                  {
                    id: 42,
                    remark: 'VetrOFF test',
                    sharedIngressSni: 'vpn.example.com',
                    protocol: 'vetroff',
                    security: 'tls',
                    enable: true,
                    network: 'tcp',
                    listen: '127.0.0.1',
                    port: 9443,
                  },
                  {
                    id: 43,
                    remark: 'AnyTLS test',
                    sharedIngressSni: 'anytls.example.com',
                    protocol: 'anytls',
                    security: 'tls',
                    enable: true,
                    network: 'tcp',
                    listen: '127.0.0.1',
                    port: 9444,
                  },
                  {
                    id: 44,
                    remark: 'WEB conflict',
                    protocol: 'anytls',
                    sharedIngressSni: 'web.example.com',
                    enable: true,
                    port: 9445,
                  },
                  {
                    id: 45,
                    remark: 'Duplicate one',
                    protocol: 'vless',
                    sharedIngressSni: 'duplicate.example.com',
                    enable: true,
                    port: 9446,
                  },
                  {
                    id: 46,
                    remark: 'Duplicate two',
                    protocol: 'vless',
                    sharedIngressSni: 'duplicate.example.com',
                    enable: true,
                    port: 9447,
                  },
                  {
                    id: 47,
                    remark: 'Hysteria UDP',
                    protocol: 'hysteria',
                    udp443: true,
                    enable: true,
                    port: 443,
                  },
                ]
              : {},
      }));
      vi.mocked(HttpUtil.post).mockImplementation(async (url) => ({
        success: true,
        msg: '',
        obj: url.endsWith('/preflight')
          ? { ready: true, domain: 'web.example.com', frontend, checks: [], warnings: [] }
          : {},
      }));
      render(<TelemtPage />);
      fireEvent.click(
        await screen.findByRole('button', { name: /Настроить Web Proxy/ }, { timeout: 5000 }),
      );
      expect(screen.getByLabelText('Домен')).toHaveProperty('value', 'web.example.com');
      fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Сервер HTTPS (frontend)' }));
      fireEvent.click(
        await screen.findByText(frontend, { selector: '.ant-select-item-option-content' }),
      );
      expect(screen.getByText('nginx — запущен')).toBeTruthy();
      expect(screen.getByText(`${frontend} — переключение после применения`)).toBeTruthy();
      {
        fireEvent.click(screen.getByRole('switch', { name: 'Общий HTTPS-вход на порту 443' }));
        const automatic = await screen.findByRole('button', {
          name: 'Select compatible automatically',
        });
        await waitFor(() => expect(automatic).toHaveProperty('disabled', false));
        fireEvent.click(automatic);
      }
      fireEvent.click(screen.getByRole('button', { name: 'Выше: websocket' }));
      await waitFor(() =>
        expect(screen.getByRole('button', { name: 'Применить' })).toHaveProperty('disabled', false),
      );
      fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
      await waitFor(() =>
        expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
          '/panel/api/telemt/webproxy/enable',
          {
            domain: 'web.example.com',
            aliases: [],
            basePath: '',
            site: { mode: 'auto' },
            retryJitter: { enabled: false, minMs: 0, maxMs: 250 },
            frontend,
            sharedIngress: { enabled: true, inboundIds: [42, 43] },
            carrier: 'https',
            carriers: ['websocket', 'websocket-lanes', 'https'],
          },
          expect.any(Object),
        ),
      );
    },
  );
});
