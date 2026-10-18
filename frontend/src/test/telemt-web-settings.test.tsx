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
  it('preserves the selected frontend and fallback and saves the reordered carrier list', async () => {
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
            webProxy,
          }
        : url.endsWith('/proxy')
          ? []
          : {},
    }));
    vi.mocked(HttpUtil.post).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/preflight')
        ? { ready: true, domain: 'web.example.com', frontend: 'caddy', checks: [], warnings: [] }
        : {},
    }));
    render(<TelemtPage />);
    fireEvent.click(
      await screen.findByRole('button', { name: /Настроить Web Proxy/ }, { timeout: 5000 }),
    );
    expect(screen.getByLabelText('Домен')).toHaveProperty('value', 'web.example.com');
    fireEvent.click(screen.getByRole('button', { name: 'Выше: websocket' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Применить' })).toBeEnabled());
    fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
        '/panel/api/telemt/webproxy/enable',
        {
          domain: 'web.example.com',
          frontend: 'caddy',
          carrier: 'https',
          carriers: ['websocket', 'websocket-lanes', 'https'],
        },
        expect.any(Object),
      ),
    );
  });
});
