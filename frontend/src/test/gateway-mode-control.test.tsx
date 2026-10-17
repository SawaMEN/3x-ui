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
      ? { ...disconnected, configured: true, forwarding: true, nat: true,
          persistent: true, listenerReady: true, rpFilter: true, policyRoute: true, nftables: true }
      : disconnected,
  };
}

beforeEach(() => {
  vi.mocked(HttpUtil.get).mockReset();
  vi.mocked(HttpUtil.post).mockReset();
});

describe('gateway settings in sing-box', () => {
  it('automatically discovers and preflights a safe LAN without enabling firewall rules', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? {
            interfaces: [
              { name: 'br-lan', defaultRoute: false, addresses: [{ ip: '192.168.10.1', prefix: 24, private: true }] },
              { name: 'eth0', defaultRoute: true, addresses: [{ ip: '203.0.113.1', prefix: 24, private: false }] },
            ],
            defaultWAN: 'eth0',
            suggested: network,
            note: 'Найден один безопасный LAN',
          }
        : status(),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: { ready: true, problems: [], notes: [] } });

    render(<GatewayModeControl />);
    expect(await screen.findByText('br-lan · 192.168.10.1/24')).toBeTruthy();
    await waitFor(() => {
      expect(screen.getByText('Готов к включению')).toBeTruthy();
      expect((screen.getByRole('button', { name: 'Включить шлюз' }) as HTMLButtonElement).disabled).toBe(false);
    }, { timeout: 5000 });
    expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(false);

    fireEvent.click(screen.getByRole('button', { name: 'Включить шлюз' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Включить', exact: true }));
    await waitFor(() => expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(true));
  });

  it('never guesses an unsafe LAN and exposes manual settings if discovery is ambiguous', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Выберите LAN вручную' }
        : status(),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, obj: { ready: false, problems: [], notes: [] } });

    render(<GatewayModeControl />);
    await screen.findByText('Выберите LAN вручную');
    expect((screen.getByRole('button', { name: 'Включить шлюз' }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole('button', { name: 'Настроить сеть вручную' }));
    expect(await screen.findByPlaceholderText('br-lan')).toBeTruthy();
    expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/enable'))).toBe(false);
  });

  it('shows only the relevant off action when the gateway is healthy', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true, msg: '',
      obj: url.endsWith('/interfaces')
        ? { interfaces: [], defaultWAN: 'eth0', note: 'Настроено вручную' }
        : status(true),
    }));
    vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: status(false) });

    render(<GatewayModeControl />);
    await screen.findByText('Работает');
    expect(screen.queryByRole('button', { name: 'Включить шлюз' })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Выключить шлюз' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Выключить', exact: true }));
    await waitFor(() => expect(vi.mocked(HttpUtil.post).mock.calls.some(([url]) => url.endsWith('/disable'))).toBe(true));
  });
});
