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

it('offers an installation workflow for a missing DNSTT binary', async () => {
  render(<DNSTTSettings />);
  fireEvent.click(await screen.findByRole('button', { name: 'Установить DNSTT' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/dnstt/install', undefined, {
      silent: true,
      timeout: 180000,
    }),
  );
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

it('hides server controls when removed in another package view', async () => {
  await openInstalled();
  current = { ...original, publicKey: '' };
  act(() => window.dispatchEvent(new Event('dnstt:changed')));
  await screen.findByRole('button', { name: 'Установить DNSTT' });
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
