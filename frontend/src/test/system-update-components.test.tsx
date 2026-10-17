import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import SystemUpdateModal from '@/pages/settings/SystemUpdateModal';
import { HttpUtil } from '@/utils';

let installedHysteria = false;
let installedOxiDNS = false;
let naiveSupported = true;
let installedNaive = false;
let hysteriaFallback = '';

beforeEach(() => {
  installedHysteria = false;
  installedOxiDNS = false;
  hysteriaFallback = '';
  naiveSupported = true;
  installedNaive = false;
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.get).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).includes('singbox')
      ? { installed: true, version: '1.13.0' }
      : String(path).includes('/oxidns/status')
        ? {
            installed: installedOxiDNS,
            installedVersion: installedOxiDNS ? 'v1.6.1' : '',
            latestVersion: 'v1.6.1',
            supported: true,
            updateAvailable: false,
          }
        : String(path).includes('externalvpn')
          ? {
              pingtunnel: { installed: false, latestVersion: '2.10' },
              trusttunnel: { installed: false, latestVersion: '1.1.0' },
              hysteria2: {
                installed: installedHysteria,
                source: !installedHysteria || hysteriaFallback ? 'sing-box' : 'standalone',
                fallbackReason: hysteriaFallback,
                latestVersion: '2.13.0',
                updateAvailable: installedHysteria,
              },
            }
          : String(path).includes('naiveproxy')
            ? {
                installed: installedNaive,
                source: installedNaive ? 'standalone' : 'sing-box',
                supported: naiveSupported,
                unsupportedReason: naiveSupported
                  ? undefined
                  : 'Caddy-Naive is not published for linux/arm64',
                latestVersion: naiveSupported ? '1.0.0' : '',
              }
            : { installed: false, latestVersion: '1.0.0' },
  }));
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => {
    if (String(path).endsWith('/externalvpn/update/hysteria2')) installedHysteria = true;
    if (String(path).endsWith('/naiveproxy/update')) installedNaive = true;
    if (String(path).endsWith('/oxidns/install')) installedOxiDNS = true;
    if (String(path).endsWith('/oxidns/remove')) installedOxiDNS = false;
    if (String(path).endsWith('/components/remove/naiveproxy')) installedNaive = false;
    if (String(path).endsWith('/components/remove/hysteria2')) installedHysteria = false;
    return {
      success: true,
      msg: '',
      obj: String(path).includes('/system/update/')
        ? { supported: true, runningAsRoot: true, canUpdate: true, packages: [] }
        : {},
    };
  });
});

async function openComponents() {
  render(<SystemUpdateModal open onClose={() => {}} />);
  await screen.findByText('Pingtunnel');
  await waitFor(() =>
    expect(
      (card('Sudoku').getByRole('button', { name: /Установить/ }) as HTMLButtonElement).disabled,
    ).toBe(false),
  );
}

function card(name: string) {
  const node = screen.getByText(name).closest('.system-component-card');
  if (!(node instanceof HTMLElement)) throw new Error(`Missing card ${name}`);
  return within(node);
}

it('keeps missing optional components out of the general update', async () => {
  await openComponents();
  fireEvent.click(screen.getByRole('button', { name: /Update panel packages & kernel/ }));
  fireEvent.click(await screen.findByRole('button', { name: 'OK' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/system/update/apply?allPackages=false',
    ),
  );
  await screen.findByText('System packages were updated successfully.');
  expect(
    vi
      .mocked(HttpUtil.post)
      .mock.calls.some(([path]) =>
        /\/(sudoku|naiveproxy)\/update|externalvpn\/update/.test(String(path)),
      ),
  ).toBe(false);
});

it('installs Sudoku only through its own install button', async () => {
  await openComponents();
  fireEvent.click(card('Sudoku').getByRole('button', { name: /Установить/ }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/setting/sudoku/update'),
  );
});

it.each([
  ['Hysteria2', '/panel/api/server/externalvpn/update/hysteria2'],
  ['NaiveProxy', '/panel/api/naiveproxy/update'],
])('automatically uses %s after installing its separate binary', async (name, endpoint) => {
  await openComponents();
  expect(card(name).getByText(/Используется встроенный sing-box/)).toBeTruthy();
  fireEvent.click(card(name).getByRole('button', { name: /Установить отдельное ядро/ }));
  await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledWith(endpoint));
  await waitFor(() => expect(card(name).getByText(/Используется отдельное ядро/)).toBeTruthy());
});

it('hides the install button and explains an unsupported architecture', async () => {
  naiveSupported = false;
  await openComponents();
  expect(card('NaiveProxy').getByText('Caddy-Naive is not published for linux/arm64')).toBeTruthy();
  expect(card('NaiveProxy').queryByRole('button', { name: /Установить/ })).toBeNull();
  expect(card('NaiveProxy').getByText(/Используется встроенный sing-box/)).toBeTruthy();
});

it('updates an installed standalone Hysteria in the general update', async () => {
  installedHysteria = true;
  await openComponents();
  fireEvent.click(screen.getByRole('button', { name: /Update panel packages & kernel/ }));
  fireEvent.click(await screen.findByRole('button', { name: 'OK' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/server/externalvpn/update/hysteria2'),
  );
});

it('explains native fallback for a preinstalled server with incompatible routing', async () => {
  installedHysteria = true;
  hysteriaFallback = 'standalone Hysteria cannot preserve routing by authenticated user';
  await openComponents();
  expect(card('Hysteria2').getByText(/Используется встроенный sing-box/)).toBeTruthy();
  expect(card('Hysteria2').getByText(hysteriaFallback)).toBeTruthy();
});

it.each(['NaiveProxy', 'Hysteria2'])(
  'removes %s through a compact trash action and refreshes native status',
  async (name) => {
    installedNaive = name === 'NaiveProxy';
    installedHysteria = name === 'Hysteria2';
    const key = name === 'NaiveProxy' ? 'naiveproxy' : 'hysteria2';
    await openComponents();
    const button = card(name).getByRole('button', { name: `Удалить ${name}` });
    expect(button.textContent).toBe('');
    fireEvent.click(button);
    expect(HttpUtil.post).not.toHaveBeenCalledWith(`/panel/api/server/components/remove/${key}`);
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(`/panel/api/server/components/remove/${key}`),
    );
    await waitFor(() =>
      expect(card(name).queryByRole('button', { name: `Удалить ${name}` })).toBeNull(),
    );
    expect(card(name).getByText(/Используется встроенный sing-box/)).toBeTruthy();
  },
);

it('does not offer removal for missing optional binaries', async () => {
  await openComponents();
  expect(screen.queryByRole('button', { name: /^Удалить / })).toBeNull();
});

it('updates an installed Hysteria server while incompatible inbounds use sing-box', async () => {
  installedHysteria = true;
  hysteriaFallback = 'Cannot preserve per-user routing';
  await openComponents();
  fireEvent.click(card('Hysteria2').getByRole('button', { name: 'Update' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/server/externalvpn/update/hysteria2'),
  );
});

it('does not treat restored connectivity as confirmation that system updates succeeded', async () => {
  installedHysteria = true;
  await openComponents();
  const originalPost = vi.mocked(HttpUtil.post).getMockImplementation()!;
  vi.mocked(HttpUtil.post).mockImplementation(async (path, ...args) => {
    if (String(path).includes('/system/update/apply'))
      return { success: false, msg: 'Failed to fetch', obj: {} };
    return originalPost(path, ...args);
  });
  fireEvent.click(screen.getByRole('button', { name: /Update panel packages & kernel/ }));
  fireEvent.click(await screen.findByRole('button', { name: 'OK' }));
  await screen.findByText(/Соединение с панелью восстановлено/);
  await waitFor(() =>
    expect(
      screen
        .getByRole('button', { name: /Update panel packages & kernel/ })
        .classList.contains('ant-btn-loading'),
    ).toBe(false),
  );
  expect(HttpUtil.post).not.toHaveBeenCalledWith('/panel/api/server/externalvpn/update/hysteria2');
  expect(screen.queryByText('System packages were updated successfully.')).toBeNull();
});

it('manages OxiDNS only as a package; setup is in sing-box DNS', async () => {
  await openComponents();
  expect(card('OxiDNS').getByText(/После установки в «sing-box → DNS»/)).toBeTruthy();
  expect(card('OxiDNS').queryByRole('link')).toBeNull();
  fireEvent.click(card('OxiDNS').getByRole('button', { name: 'Установить OxiDNS' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/oxidns/install', undefined, {
      timeout: 180000,
    }),
  );
  await waitFor(() =>
    expect(card('OxiDNS').getByText(/Создание и настройка DNS доступны/)).toBeTruthy(),
  );
  expect(card('OxiDNS').queryByRole('link')).toBeNull();
  fireEvent.click(card('OxiDNS').getByRole('button', { name: 'Удалить OxiDNS' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/oxidns/remove'));
  expect(card('OxiDNS').queryByRole('link')).toBeNull();
  expect(card('DNSTT').queryByRole('link')).toBeNull();
});
