import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import OxiDNSSettings from '@/pages/singbox/OxiDNSSettings';
import { HttpUtil } from '@/utils';

const original = {
  installed: false,
  enabled: false,
  running: false,
  supported: true,
  domain: '',
  doh: false,
  http3: false,
  dot: false,
  doq: false,
  port: 8443,
  dotPort: 853,
  doqPort: 853,
  upstream: 'quad9',
  useSingBox: true,
  localAddress: '127.0.0.1:15353',
  installedVersion: '',
  latestVersion: '',
  updateAvailable: false,
  error: '',
};
let current = { ...original };

beforeEach(() => {
  current = { ...original, installed: true };
  vi.mocked(HttpUtil.get).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).includes('/logs')
      ? { logs: 'Started OxiDNS test service' }
      : String(path).includes('check=true')
        ? {
            ...current,
            latestVersion: 'v1.7.0',
            updateAvailable: current.installed,
          }
        : current,
  }));
  vi.mocked(HttpUtil.post).mockImplementation(async (path, body) => {
    const endpoint = String(path);
    if (endpoint.endsWith('/install')) {
      current = { ...current, installed: true };
    } else if (endpoint.endsWith('/setup')) {
      current = {
        ...current,
        ...(body as Partial<typeof original>),
        installed: true,
        enabled: true,
        running: true,
      };
    } else if (endpoint.endsWith('/disable')) {
      current = { ...current, enabled: false, running: false };
    } else if (endpoint.endsWith('/remove')) {
      current = { ...original };
    } else if (endpoint.endsWith('/update')) {
      current = { ...current, installedVersion: 'v1.7.0' };
    } else if (endpoint.endsWith('/check')) {
      return { success: true, msg: '', obj: { healthy: true } };
    }
    return { success: true, msg: '', obj: current };
  });
  vi.mocked(HttpUtil.get).mockClear();
  vi.mocked(HttpUtil.post).mockClear();
});

async function openDNS() {
  render(<OxiDNSSettings />);
  await screen.findByRole('combobox', { name: 'DNS-провайдер' });
}

function showAdvanced() {
  fireEvent.click(screen.getByText(/Дополнительные настройки: DoH/));
}

it('requires installation before activation and preserves settings', async () => {
  current = { ...original };
  await openDNS();
  expect(
    (screen.getByRole('button', { name: 'Сначала установите OxiDNS' }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.click(screen.getByRole('button', { name: 'Установить OxiDNS' }));
  await screen.findByRole('button', { name: 'Включить DNS' });
  expect(current.enabled).toBe(false);
  expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/oxidns/install', undefined, {
    silent: true,
    timeout: 180000,
  });
  expect(HttpUtil.post).not.toHaveBeenCalledWith(
    '/panel/api/oxidns/setup',
    expect.anything(),
    expect.anything(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Включить DNS' }));
  await screen.findByText(/DNS работает — запрос успешно проверен/);
});

it('keeps activation blocked after a failed installation', async () => {
  current = { ...original };
  vi.mocked(HttpUtil.post).mockResolvedValueOnce({
    success: false,
    msg: 'Ошибка скачивания',
    obj: null,
  });
  await openDNS();
  fireEvent.click(screen.getByRole('button', { name: 'Установить OxiDNS' }));
  await screen.findByText('Ошибка скачивания');
  expect(
    (screen.getByRole('button', { name: 'Сначала установите OxiDNS' }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(current.enabled).toBe(false);
});

it('keeps the initial flow simple and hides optional external modes', async () => {
  await openDNS();
  expect(screen.queryByRole('switch', { name: 'Включить публичный DoH' })).toBeNull();
  expect(screen.queryByRole('textbox', { name: 'Домен защищённого DNS' })).toBeNull();
  showAdvanced();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  expect(screen.getByRole('textbox', { name: 'Домен защищённого DNS' })).toBeTruthy();
  expect(screen.getByRole('spinbutton', { name: 'HTTPS-порт DoH' })).toBeTruthy();
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    true,
  );
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'dns.example.com' },
  });
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    false,
  );
});

it('rejects unused DNS and can be corrected without leaving the form', async () => {
  await openDNS();
  fireEvent.click(screen.getByRole('switch', { name: 'Подключить OxiDNS к sing-box' }));
  expect(screen.getByText('DNS не будет использоваться')).toBeTruthy();
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    true,
  );
});

it('supports applying and reverting advanced options', async () => {
  current = { ...original, installed: true, enabled: true, running: true };
  await openDNS();
  showAdvanced();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  expect(screen.getByText('Настройки изменены, но ещё не применены')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Сохранить и применить' })).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Отменить изменения' }));
  expect(screen.queryByRole('textbox', { name: 'Домен защищённого DNS' })).toBeNull();
});

it('allows DoT and DoQ on the same numeric port using different transports', async () => {
  await openDNS();
  showAdvanced();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить DoT' }));
  fireEvent.click(screen.getByRole('switch', { name: 'Включить DoQ' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'dns.example.com' },
  });
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    false,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Включить DNS' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/oxidns/setup',
      expect.objectContaining({ dot: true, doq: true, dotPort: 853, doqPort: 853 }),
      { silent: true, timeout: 180000 },
    ),
  );
});

it('prevents HTTP/3 and DoQ using the same UDP port', async () => {
  await openDNS();
  showAdvanced();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  fireEvent.click(screen.getByRole('switch', { name: 'Включить HTTP/3' }));
  fireEvent.click(screen.getByRole('switch', { name: 'Включить DoQ' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'dns.example.com' },
  });
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Порт DoQ' }), {
    target: { value: '8443' },
  });
  expect(screen.getByText(/Конфликт TCP\/UDP-портов/)).toBeTruthy();
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    true,
  );
});

it('shows the installer again after removal in another settings window', async () => {
  current = { ...original, installed: true };
  await openDNS();
  current = { ...original, installed: false };
  act(() => window.dispatchEvent(new Event('oxidns:changed')));
  await screen.findByRole('button', { name: 'Установить OxiDNS' });
  expect(
    (screen.getByRole('button', { name: 'Сначала установите OxiDNS' }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it('checks the official version and makes update available from this screen', async () => {
  current = { ...original, installed: true, installedVersion: 'v1.6.1' };
  await openDNS();
  fireEvent.click(screen.getByText(/Обслуживание и диагностика OxiDNS/));
  fireEvent.click(screen.getByRole('button', { name: 'Проверить обновления' }));
  await screen.findByRole('button', { name: 'Обновить OxiDNS' });
  fireEvent.click(screen.getByRole('button', { name: 'Обновить OxiDNS' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/oxidns/update', undefined, {
      silent: true,
      timeout: 180000,
    }),
  );
  expect(screen.getByText(/Установленная версия: v1.7.0/)).toBeTruthy();
});

it('shows bounded systemd logs through the authenticated API', async () => {
  current = { ...original, installed: true };
  await openDNS();
  fireEvent.click(screen.getByText(/Обслуживание и диагностика OxiDNS/));
  fireEvent.click(screen.getByRole('button', { name: 'Журнал службы' }));
  await screen.findByDisplayValue('Started OxiDNS test service');
  expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/oxidns/logs', undefined, {
    silent: true,
    timeout: 12000,
  });
});

it('disables setup on unsupported platforms but explains why', async () => {
  current = { ...original, installed: true, supported: false };
  await openDNS();
  expect(screen.getByText('Автоматическая установка недоступна')).toBeTruthy();
  expect((screen.getByRole('button', { name: 'Включить DNS' }) as HTMLButtonElement).disabled).toBe(
    true,
  );
});
