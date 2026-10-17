import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import DNSTTSettings from '@/pages/singbox/DNSTTSettings';
import { HttpUtil } from '@/utils';

const original = {
  installed: false,
  running: false,
  enabled: false,
  supported: true,
  domain: '',
  serverIP: '',
  listenPort: 53,
  targetPort: 22,
  updateAvailable: false,
  latestVersion: '2026-05-01-0c5c52a',
};
let current = { ...original, publicKey: '' };

beforeEach(() => {
  current = { ...original, publicKey: '' };
  vi.mocked(HttpUtil.get).mockImplementation(async () => ({
    success: true,
    msg: '',
    obj: current,
  }));
  vi.mocked(HttpUtil.post).mockImplementation(async (path, data) => {
    if (String(path).endsWith('/install')) {
      current = { ...current, installed: true };
    } else if (String(path).endsWith('/setup')) {
      const form = data as {
        domain: string;
        serverIP: string;
        listenPort: number;
        targetPort: number;
      };
      current = {
        ...current,
        ...form,
        enabled: true,
        running: true,
        publicKey: '1234567890abcdef'.repeat(4),
      };
    } else if (String(path).endsWith('/disable')) {
      current = { ...current, enabled: false, running: false };
    } else if (String(path).endsWith('/remove')) {
      current = { ...original, publicKey: '' };
    }
    return { success: true, msg: '', obj: current };
  });
  vi.mocked(HttpUtil.get).mockClear();
  vi.mocked(HttpUtil.post).mockClear();
});

async function openInstalled() {
  current = { ...current, installed: true };
  render(<DNSTTSettings />);
  await screen.findByRole('textbox', { name: 'Домен DNSTT' });
}

it('does not display DNSTT before package installation', async () => {
  render(<DNSTTSettings />);
  await waitFor(() =>
    expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/dnstt/status', undefined, {
      silent: true,
    }),
  );
  await waitFor(() => expect(screen.queryByText('DNS-туннель — DNSTT')).toBeNull());
  expect(screen.queryByRole('button', { name: 'Установить DNSTT' })).toBeNull();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});

it('shows DNSTT when installed through package updates', async () => {
  render(<DNSTTSettings />);
  await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
  current = { ...current, installed: true };
  act(() => window.dispatchEvent(new Event('dnstt:changed')));
  await screen.findByRole('textbox', { name: 'Домен DNSTT' });
});

it('shows NS and A instructions and creates an authenticated local target', async () => {
  await openInstalled();
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен DNSTT' }), {
    target: { value: 't.example.com' },
  });
  fireEvent.change(screen.getByRole('textbox', { name: 'IPv4 DNSTT' }), {
    target: { value: '203.0.113.2' },
  });
  const records = screen.getByText('Добавьте записи у DNS-провайдера').closest('[role="alert"]');
  expect(records?.textContent).toContain('t.example.com');
  expect(records?.textContent).toContain('ns.example.com');
  expect(records?.textContent).toContain('203.0.113.2');
  fireEvent.click(screen.getByRole('button', { name: 'Создать и включить DNSTT' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/dnstt/setup',
      {
        domain: 't.example.com',
        serverIP: '203.0.113.2',
        listenPort: 53,
        targetPort: 22,
        enabled: false,
      },
      { silent: true, timeout: 180000 },
    ),
  );
  await screen.findByText('Публичный ключ сервера');
  expect(screen.getByText(/dnstt-client -doh https:\/\/cloudflare-dns.com/)).toBeTruthy();
});

it('verifies DNS delegation without changing tunnel configuration', async () => {
  current = {
    ...current,
    installed: true,
    running: true,
    enabled: true,
    domain: 't.example.com',
    serverIP: '203.0.113.2',
    publicKey: '1234567890abcdef'.repeat(4),
  };
  const originalGet = vi.mocked(HttpUtil.get).getMockImplementation();
  vi.mocked(HttpUtil.get).mockImplementation(async (path, ...args) => {
    if (String(path).endsWith('/check-delegation')) {
      return {
        success: true,
        msg: '',
        obj: {
          zone: 't.example.com',
          nameServer: 'ns.example.com',
          expectedIP: '203.0.113.2',
          addresses: ['203.0.113.2'],
          nsFound: true,
          ipMatches: true,
          ready: true,
          details: 'DNS-делегирование найдено.',
        },
      };
    }
    return originalGet!(path, ...args);
  });
  render(<DNSTTSettings />);
  fireEvent.click(await screen.findByRole('button', { name: 'Проверить NS-делегирование домена' }));
  await screen.findByText('DNS-делегирование найдено');
  expect(screen.getByText('ns.example.com')).toBeTruthy();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});

it('switches client resolver transport without modifying server settings', async () => {
  current = {
    ...current,
    installed: true,
    running: true,
    enabled: true,
    domain: 't.example.com',
    publicKey: '1234567890abcdef'.repeat(4),
  };
  render(<DNSTTSettings />);
  await screen.findByText('4. Подключение клиента DNSTT');
  expect(screen.getByText(/dnstt-client -doh /)).toBeTruthy();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});

it('supports custom DoH/DoT resolvers and rejects unsafe client command arguments', async () => {
  current = {
    ...current,
    installed: true,
    enabled: true,
    running: true,
    domain: 't.example.com',
    publicKey: '1234567890abcdef'.repeat(4),
  };
  render(<DNSTTSettings />);
  await screen.findByRole('textbox', { name: 'DNS-резолвер клиента' });
  fireEvent.change(screen.getByRole('textbox', { name: 'DNS-резолвер клиента' }), {
    target: { value: 'https://dns.example.com/dns-query' },
  });
  expect(screen.getByText(/dnstt-client -doh https:\/\/dns.example.com\/dns-query/)).toBeTruthy();
  fireEvent.change(screen.getByRole('textbox', { name: 'DNS-резолвер клиента' }), {
    target: { value: 'https://dns.example.com/;rm -rf' },
  });
  expect(screen.getByText(/Проверьте адрес DNS-резолвера клиента/)).toBeTruthy();
  expect(screen.getByText(/dnstt-client -doh <DNS_RESOLVER>/)).toBeTruthy();
});

it('hides server controls when removed in another package view', async () => {
  await openInstalled();
  current = { ...original, publicKey: '' };
  act(() => window.dispatchEvent(new Event('dnstt:changed')));
  await waitFor(() => expect(screen.queryByText('DNS-туннель — DNSTT')).toBeNull());
  expect(screen.queryByRole('textbox', { name: 'Домен DNSTT' })).toBeNull();
});

it('removes DNSTT with key destruction warning', async () => {
  await openInstalled();
  const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
  fireEvent.click(screen.getByRole('button', { name: 'Удалить DNSTT и ключи' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/dnstt/remove', undefined, {
      silent: true,
      timeout: 180000,
    }),
  );
  confirm.mockRestore();
});
