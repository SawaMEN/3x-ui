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
  certFile: '',
  keyFile: '',
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
    } else if (endpoint.endsWith('/validate')) {
      return { success: true, msg: '', obj: { valid: true } };
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

it('does not expose DNS configuration or package installation before OxiDNS is installed', async () => {
  current = { ...original };
  render(<OxiDNSSettings />);
  await screen.findByText('Для работы требуется установка OxiDNS');
  expect(screen.getByText(/Системные пакеты и ядро Linux/)).toBeTruthy();
  expect(screen.queryByRole('combobox', { name: 'DNS-провайдер' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Установить OxiDNS' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Создать и включить DNS' })).toBeNull();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});

it('automatically selects the VPN preset and enables the service with one action', async () => {
  await openDNS();
  expect(screen.getByText(/Домен, сертификат и открытые порты не требуются/)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Создать и включить DNS' }));
  await screen.findByText(/DNS работает — запрос успешно проверен/);
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/oxidns/setup',
    expect.objectContaining({
      useSingBox: true,
      doh: false,
      dot: false,
      doq: false,
      upstream: 'quad9',
    }),
    { silent: true, timeout: 180000 },
  );
});

it('automatically prepares DoH mode and explains external prerequisites', async () => {
  await openDNS();
  fireEvent.click(screen.getByRole('button', { name: 'Для телефона и браузера (DoH)' }));
  expect(screen.getByRole('textbox', { name: 'Домен защищённого DNS' })).toBeTruthy();
  expect(screen.getByText(/не регистрирует домен, не выпускает сертификат/)).toBeTruthy();
  expect(screen.getByText(/Потребуется домен, действующий сертификат/)).toBeTruthy();
});

it('allows a different DNS subdomain and guides its DNS records and TLS setup', async () => {
  await openDNS();
  fireEvent.click(screen.getByRole('button', { name: 'Для телефона и браузера (DoH)' }));
  expect(screen.getByText(/даже если адрес панели другой/)).toBeTruthy();
  expect(screen.getByText(/публичный IPv4 вашего сервера/)).toBeTruthy();
  expect(screen.getByText(/DNS only в Cloudflare/)).toBeTruthy();
  expect(screen.getByText(/TCP\/8443 \(DoH\)/)).toBeTruthy();

  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'resolver.another-domain.net' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Проверить DNS-домен и сертификат' }));
  await screen.findByText('Сертификат подходит выбранному DNS-поддомену');
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/oxidns/validate',
    expect.objectContaining({ domain: 'resolver.another-domain.net' }),
    { silent: true, timeout: 12000 },
  );
  expect(HttpUtil.post).not.toHaveBeenCalledWith(
    '/panel/api/oxidns/setup',
    expect.anything(),
    expect.anything(),
  );
  expect(screen.getByText(/A\/AAAA-записи и доступность портов из интернета не проверялись/)).toBeTruthy();
});

it('accepts independent certificate paths and sends them during setup', async () => {
  await openDNS();
  fireEvent.click(screen.getByRole('button', { name: 'Для телефона и браузера (DoH)' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'dns.another-domain.net' },
  });
  fireEvent.change(screen.getByRole('textbox', { name: 'Путь к сертификату DNS' }), {
    target: { value: '/etc/letsencrypt/live/dns.another-domain.net/fullchain.pem' },
  });
  expect((screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(screen.getByRole('textbox', { name: 'Путь к ключу DNS' }), {
    target: { value: '/etc/letsencrypt/live/dns.another-domain.net/privkey.pem' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Создать и включить DNS' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/oxidns/setup',
      expect.objectContaining({
        domain: 'dns.another-domain.net',
        certFile: '/etc/letsencrypt/live/dns.another-domain.net/fullchain.pem',
        keyFile: '/etc/letsencrypt/live/dns.another-domain.net/privkey.pem',
      }),
      { silent: true, timeout: 180000 },
    ),
  );
});

it('keeps the initial flow simple and hides optional external modes', async () => {
  await openDNS();
  expect(screen.queryByRole('switch', { name: 'Включить публичный DoH' })).toBeNull();
  expect(screen.queryByRole('textbox', { name: 'Домен защищённого DNS' })).toBeNull();
  showAdvanced();
  fireEvent.click(screen.getByRole('switch', { name: 'Включить публичный DoH' }));
  expect(screen.getByRole('textbox', { name: 'Домен защищённого DNS' })).toBeTruthy();
  expect(screen.getByRole('spinbutton', { name: 'HTTPS-порт DoH' })).toBeTruthy();
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(true);
  fireEvent.change(screen.getByRole('textbox', { name: 'Домен защищённого DNS' }), {
    target: { value: 'dns.example.com' },
  });
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(false);
});

it('rejects unused DNS and can be corrected without leaving the form', async () => {
  await openDNS();
  fireEvent.click(screen.getByRole('switch', { name: 'Подключить OxiDNS к sing-box' }));
  expect(screen.getByText('DNS не будет использоваться')).toBeTruthy();
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(true);
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
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(false);
  fireEvent.click(screen.getByRole('button', { name: 'Создать и включить DNS' }));
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
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(true);
});

it('hides all settings after the package was removed elsewhere', async () => {
  current = { ...original, installed: true };
  await openDNS();
  current = { ...original, installed: false };
  act(() => window.dispatchEvent(new Event('oxidns:changed')));
  await screen.findByText('Для работы требуется установка OxiDNS');
  expect(screen.queryByRole('combobox', { name: 'DNS-провайдер' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Установить OxiDNS' })).toBeNull();
});

it('shows diagnostics without lifecycle operations', async () => {
  current = { ...original, installed: true, installedVersion: 'v1.6.1' };
  await openDNS();
  fireEvent.click(screen.getByText('Диагностика OxiDNS'));
  expect(screen.getByText(/Версия OxiDNS: v1.6.1/)).toBeTruthy();
  expect(screen.getByText(/Для обновления или удаления/)).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Проверить обновления' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Обновить OxiDNS' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Удалить OxiDNS' })).toBeNull();
});

it('shows bounded systemd logs through the authenticated API', async () => {
  current = { ...original, installed: true };
  await openDNS();
  fireEvent.click(screen.getByText('Диагностика OxiDNS'));
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
  expect(screen.getByText('OxiDNS не поддерживается на этой системе')).toBeTruthy();
  expect(
    (screen.getByRole('button', { name: 'Создать и включить DNS' }) as HTMLButtonElement).disabled,
  ).toBe(true);
});
