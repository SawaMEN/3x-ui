import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import GatewayModeControl from '@/pages/settings/GatewayModeControl';
import { HttpUtil } from '@/utils';

vi.setConfig({ testTimeout: 15000 });

const network = {
  lanInterface: 'br-lan',
  lanIP: '192.168.10.1',
  lanPrefix: 24,
  wanInterface: 'eth0',
};

const disconnected = {
  configured: false,
  config: network,
  forwarding: false,
  nat: false,
  persistent: false,
  listenerReady: false,
  rpFilter: false,
  policyRoute: false,
  nftables: false,
};

function status(enabled = false) {
  return {
    enabled,
    configured: enabled,
    recoveryBackup: false,
    canEnable: true,
    coreType: 'hiddify-core',
    gatewayCoreType: enabled ? 'hiddify-core' : '',
    coreRunning: true,
    port: 52345,
    network: enabled
      ? {
          ...disconnected,
          configured: true,
          forwarding: true,
          nat: true,
          persistent: true,
          listenerReady: true,
          rpFilter: true,
          policyRoute: true,
          nftables: true,
        }
      : disconnected,
  };
}

beforeEach(() => {
  vi.mocked(HttpUtil.get).mockReset();
  vi.mocked(HttpUtil.post).mockReset();
});

// Read-only discovery must never enable traffic interception without confirmation.
describe('gateway settings in sing-box', () => {
  it('automatically discovers and preflights a safe LAN without enabling firewall rules', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? {
            interfaces: [
              {
                name: 'br-lan',
                defaultRoute: false,
                addresses: [{ ip: '192.168.10.1', prefix: 24, private: true }],
              },
              {
                name: 'eth0',
                defaultRoute: true,
                addresses: [{ ip: '203.0.113.1', prefix: 24, private: false }],
              },
            ],
            defaultWAN: 'eth0',
            suggested: network,
            note: 'Найден один безопасный LAN',
          }
        : status(),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: true,
      msg: '',
      obj: { ready: true, problems: [], notes: [] },
    });

    render(<GatewayModeControl />);
    expect(await screen.findByText('br-lan · 192.168.10.1/24')).toBeTruthy();
    await waitFor(
      () => {
        expect(screen.getByText('Сеть готова. Можно включать шлюз.')).toBeTruthy();
        expect(
          (screen.getByRole('button', { name: 'Включить шлюз' }) as HTMLButtonElement).disabled,
        ).toBe(false);
      },
      { timeout: 5000 },
    );
    expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(
      false,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Включить шлюз' }));
    fireEvent.click(await screen.findByRole('button', { name: /^Включить$/ }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(
        true,
      ),
    );
  });

  it('never guesses an unsafe LAN and exposes manual settings if discovery is ambiguous', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Выберите LAN вручную' }
        : status(),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: true,
      msg: '',
      obj: { ready: false, problems: [], notes: [] },
    });

    render(<GatewayModeControl />);
    await screen.findByText('Выберите LAN вручную');
    expect(
      (screen.getByRole('button', { name: 'Включить шлюз' }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: 'Выбрать сеть' }));
    expect(await screen.findByPlaceholderText('br-lan')).toBeTruthy();
    expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(
      false,
    );
  });

  it('shows only the relevant off action when the gateway is healthy', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Настроено вручную' }
        : status(true),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: status(false) });

    render(<GatewayModeControl />);
    await screen.findByText('Работает');
    expect(screen.queryByRole('button', { name: 'Включить шлюз' })).toBeNull();
    expect(screen.queryByPlaceholderText('br-lan')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Выключить шлюз' }));
    fireEvent.click(await screen.findByRole('button', { name: /^Выключить$/ }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/disable'))).toBe(
        true,
      ),
    );
  });
  it('ignores an outdated preflight result after the LAN configuration changes', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? {
            interfaces: [
              {
                name: 'br-lan',
                defaultRoute: false,
                addresses: [{ ip: '192.168.10.1', prefix: 24, private: true }],
              },
              {
                name: 'eth0',
                defaultRoute: true,
                addresses: [{ ip: '203.0.113.1', prefix: 24, private: false }],
              },
            ],
            defaultWAN: 'eth0',
            suggested: network,
            note: 'Автоматическая настройка',
          }
        : status(),
    }));

    let releaseFirst:
      | ((value: {
          success: boolean;
          msg: string;
          obj: { ready: boolean; problems: string[]; notes: string[] };
        }) => void)
      | undefined;
    let checks = 0;
    vi.mocked(HttpUtil.post).mockImplementation(async (url) => {
      if (!url.endsWith('/preflight')) return { success: true, msg: '', obj: status(true) };
      checks += 1;
      if (checks === 1) {
        return new Promise<{
          success: boolean;
          msg: string;
          obj: { ready: boolean; problems: string[]; notes: string[] };
        }>((resolve) => {
          releaseFirst = resolve;
        });
      }
      return { success: true, msg: '', obj: { ready: true, problems: [], notes: [] } };
    });

    render(<GatewayModeControl />);
    await waitFor(() => expect(checks).toBe(1), { timeout: 5000 });
    fireEvent.click(screen.getByText('Параметры сети'));
    fireEvent.change(await screen.findByPlaceholderText('192.168.50.1'), {
      target: { value: '192.168.10.2' },
    });
    await waitFor(
      () => {
        expect(checks).toBe(2);
        expect(screen.getByText('Сеть готова. Можно включать шлюз.')).toBeTruthy();
      },
      { timeout: 5000 },
    );

    releaseFirst?.({
      success: true,
      msg: '',
      obj: { ready: false, problems: ['Устаревшая проверка'], notes: [] },
    });
    await waitFor(() => {
      expect(screen.getByText('Сеть готова. Можно включать шлюз.')).toBeTruthy();
      expect(screen.queryByText('Устаревшая проверка')).toBeNull();
    });
  });

  it('does not report an offline core as a working gateway', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Нет отдельного LAN' }
        : { ...status(true), coreRunning: false, canEnable: false },
    }));
    render(<GatewayModeControl />);
    expect(await screen.findByText('Требует внимания')).toBeTruthy();
    expect(screen.queryByText('Работает')).toBeNull();
  });
  it('does not show a warning for a disabled gateway just because the core is stopped', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Выберите сеть вручную' }
        : { ...status(), coreRunning: false, canEnable: false },
    }));

    render(<GatewayModeControl />);
    expect(await screen.findByText('Для включения сначала запустите sing-box.')).toBeTruthy();
    expect(screen.queryByText('Ядро остановлено')).toBeNull();
    expect(screen.queryByText('Требует внимания')).toBeNull();
    fireEvent.click(screen.getByText('Диагностика'));
    expect(screen.getByText('Проверка сетевых служб появится после включения шлюза.')).toBeTruthy();
    expect(screen.queryByText('IPv4 forwarding')).toBeNull();
  });

  it('shows a failed readiness check once, including when diagnostics are opened', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? {
            interfaces: [
              {
                name: 'br-lan',
                defaultRoute: false,
                addresses: [{ ip: '192.168.10.1', prefix: 24, private: true }],
              },
              {
                name: 'eth0',
                defaultRoute: true,
                addresses: [{ ip: '203.0.113.1', prefix: 24, private: false }],
              },
            ],
            defaultWAN: 'eth0',
            suggested: network,
            note: 'Автоматическая настройка',
          }
        : status(),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: true,
      msg: '',
      obj: { ready: false, problems: ['Не найден модуль TPROXY'], notes: [] },
    });

    render(<GatewayModeControl />);
    await screen.findByText('Нужно исправить настройки');
    expect(screen.getAllByText('Не найден модуль TPROXY')).toHaveLength(1);
    expect(
      (screen.getByRole('button', { name: 'Включить шлюз' }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByText('Диагностика'));
    expect(screen.getAllByText('Не найден модуль TPROXY')).toHaveLength(1);
  });

  it('shows the actual network error in a single gateway alert', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: '' }
        : {
            ...status(true),
            network: {
              ...status(true).network,
              listenerReady: false,
              error: 'Не удалось запустить TPROXY',
            },
          },
    }));

    render(<GatewayModeControl />);
    expect(await screen.findByText('Не удалось запустить TPROXY')).toBeTruthy();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    expect(screen.getByText('Шлюз требует восстановления')).toBeTruthy();
  });
  it('keeps a corrupted stored network error visible even when the gateway is off', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: '' }
        : {
            ...status(),
            network: {
              ...disconnected,
              configured: true,
              error: 'Файл сетевых настроек повреждён',
            },
          },
    }));

    render(<GatewayModeControl />);
    expect(await screen.findByText('Ошибка сетевых настроек')).toBeTruthy();
    expect(screen.getByText('Файл сетевых настроек повреждён')).toBeTruthy();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
  });
});
