import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import OxiDNSSettings from '@/pages/singbox/OxiDNSSettings';
import { HttpUtil } from '@/utils';

const original = {
  installed: true,
  enabled: false,
  running: false,
  supported: true,
  domain: '',
  doh: false,
  port: 8443,
  upstream: 'quad9',
  useSingBox: true,
  localAddress: '127.0.0.1:15353',
};
let current = { ...original };

beforeEach(() => {
  current = { ...original };
  vi.mocked(HttpUtil.get).mockImplementation(async () => ({ success: true, obj: current }));
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => {
    if (String(path).endsWith('/setup')) {
      current = { ...current, enabled: true, running: true };
    }
    if (String(path).endsWith('/disable')) {
      current = { ...current, enabled: false, running: false };
    }
    return { success: true, obj: current };
  });
  vi.mocked(HttpUtil.get).mockClear();
  vi.mocked(HttpUtil.post).mockClear();
});

async function openInstalled() {
  render(<OxiDNSSettings />);
  await screen.findByText('1. Выберите DNS-провайдера');
}

it('hides the DNS wizard when OxiDNS has been removed', async () => {
  current = { ...original, installed: false };
  render(<OxiDNSSettings />);
  await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
  await waitFor(() => expect(screen.queryByText('Собственный DNS — OxiDNS')).toBeNull());
});

it('offers one-click local DNS without domain or HTTPS settings', async () => {
  await openInstalled();
  expect(screen.getByText('Quad9 — защита от вредоносных доменов')).toBeTruthy();
  expect(screen.queryByRole('textbox', { name: 'Домен для DoH' })).toBeNull();
  expect(screen.queryByRole('spinbutton', { name: 'HTTPS-порт DoH' })).toBeNull();

  fireEvent.click(screen.getByRole('button', { name: /Создать и включить DNS/ }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/oxidns/setup',
      { domain: '', doh: false, port: 8443, upstream: 'quad9', useSingBox: true },
      { silent: true, timeout: 180000 },
    ),
  );
  await screen.findByText('DNS проверен и работает');
  expect(screen.getByText('127.0.0.1:15353')).toBeTruthy();
});

it('requires a domain only when public DoH is enabled', async () => {
  await openInstalled();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  expect(screen.getByRole('textbox', { name: 'Домен для DoH' })).toBeTruthy();
  expect(screen.getByRole('spinbutton', { name: 'HTTPS-порт DoH' })).toBeTruthy();
  expect(
    (screen.getByRole('button', { name: /Создать и включить DNS/ }) as HTMLButtonElement).disabled,
  ).toBe(true);
  expect(HttpUtil.post).not.toHaveBeenCalled();

  fireEvent.change(screen.getByRole('textbox', { name: 'Домен для DoH' }), {
    target: { value: 'dns.example.com' },
  });
  expect(
    (screen.getByRole('button', { name: /Создать и включить DNS/ }) as HTMLButtonElement).disabled,
  ).toBe(false);
});

it('warns when the resolver has no consumers', async () => {
  await openInstalled();
  fireEvent.click(screen.getByRole('switch', { name: 'Использовать OxiDNS в sing-box' }));
  expect(screen.getByText('DNS пока не подключён ни к одному клиенту')).toBeTruthy();
  expect(
    (screen.getByRole('button', { name: /Создать и включить DNS/ }) as HTMLButtonElement).disabled,
  ).toBe(true);
});

it('shows unapplied changes and allows resetting the provider', async () => {
  current = { ...original, enabled: true, running: true };
  await openInstalled();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  expect(screen.getByText('Есть неприменённые изменения')).toBeTruthy();
  expect(screen.getByRole('button', { name: /Сохранить и применить/ })).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Отменить изменения' }));
  expect(screen.queryByRole('textbox', { name: 'Домен для DoH' })).toBeNull();
  expect(screen.queryByText('Есть неприменённые изменения')).toBeNull();
});

it('refreshes the screen when the package is removed elsewhere', async () => {
  await openInstalled();
  current = { ...original, installed: false };
  act(() => window.dispatchEvent(new Event('oxidns:changed')));
  await waitFor(() => expect(screen.queryByText('Собственный DNS — OxiDNS')).toBeNull());
});
