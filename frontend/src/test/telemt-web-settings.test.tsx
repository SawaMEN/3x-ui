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
              webProxy: { ...webProxy, frontend },
            }
          : url.endsWith('/proxy')
            ? []
            : url.endsWith('/inbounds/options')
              ? [
                  {
                    id: 42,
                    remark: 'Naive test',
                    protocol: 'naive',
                    enable: true,
                    network: 'tcp',
                    listen: '127.0.0.1',
                    port: 9443,
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
      if (frontend === 'haproxy') {
        fireEvent.click(screen.getByRole('switch', { name: 'Общий HTTPS-вход на порту 443' }));
        fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Входящий NaiveProxy' }));
        fireEvent.click(await screen.findByText('Naive test · 9443'));
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
            site: undefined,
            frontend,
            ...(frontend === 'haproxy' && { sharedIngress: { enabled: true, naiveInboundId: 42 } }),
            carrier: 'https',
            carriers: ['websocket', 'websocket-lanes', 'https'],
          },
          expect.any(Object),
        ),
      );
    },
  );
});
