import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import SystemUpdateModal from '@/pages/settings/SystemUpdateModal';
import { HttpUtil } from '@/utils';

let installedHysteria = false;

beforeEach(() => {
  installedHysteria = false;
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.get).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).includes('singbox')
      ? { installed: true, version: '1.13.0' }
      : String(path).includes('externalvpn')
        ? {
            pingtunnel: { installed: false, latestVersion: '2.10' },
            trusttunnel: { installed: false, latestVersion: '1.1.0' },
            hysteria2: {
              installed: installedHysteria,
              latestVersion: '2.13.0',
              updateAvailable: installedHysteria,
            },
          }
        : { installed: false, latestVersion: '1.0.0' },
  }));
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).includes('/system/update/')
      ? { supported: true, runningAsRoot: true, canUpdate: true, packages: [] }
      : {},
  }));
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
])('offers %s as a built-in component or an optional separate binary', async (name, endpoint) => {
  await openComponents();
  expect(
    (card(name).getByRole('button', { name: /Встроен в sing-box/ }) as HTMLButtonElement).disabled,
  ).toBe(true);
  fireEvent.mouseDown(screen.getByLabelText(`${name}: вариант компонента`));
  fireEvent.click(await screen.findByTitle('Отдельная установка бинарника'));
  expect(
    card(name).getByText(/Подключения панели продолжают использовать sing-box\./i),
  ).toBeTruthy();
  fireEvent.click(card(name).getByRole('button', { name: /Установить/ }));
  await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledWith(endpoint));
});

it('updates an installed standalone Hysteria even while the built-in view is selected', async () => {
  installedHysteria = true;
  await openComponents();
  fireEvent.click(screen.getByRole('button', { name: /Update panel packages & kernel/ }));
  fireEvent.click(await screen.findByRole('button', { name: 'OK' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/server/externalvpn/update/hysteria2'),
  );
});
