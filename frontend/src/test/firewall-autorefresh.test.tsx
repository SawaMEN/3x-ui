import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { FirewallManager } from '@/pages/firewall/FirewallManager';
import { HttpUtil } from '@/utils';

vi.mock('@/utils', () => ({ HttpUtil: { get: vi.fn(), post: vi.fn() } }));

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

it('persists a configurable reconciliation interval without changing the auto-sync toggle', async () => {
  const status = { supported: true, backend: 'iptables', enabled: true, autoSync: false, syncInterval: 60, pingEnabled: true, rules: [], manualRules: [] };
  vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, msg: '', obj: status });
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: { ...status, syncInterval: 120 } });
  const view = render(<FirewallManager />);
  const input = await screen.findByRole('spinbutton', { name: /Rule check interval|Период проверки правил/ });
  fireEvent.change(input, { target: { value: '120' } });
  await act(async () => { fireEvent.blur(input); });
  expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/server/firewall/auto-sync', { enabled: false, syncInterval: 120 }, { silentSuccess: true });
  view.unmount();
});

it('refreshes firewall state automatically and cancels polling on unmount', async () => {
  vi.useFakeTimers();
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false);
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: {
      supported: true,
      backend: 'iptables',
      enabled: true,
      autoSync: true,
      pingEnabled: true,
      rules: [],
      manualRules: [],
    },
  });
  const view = render(<FirewallManager />);
  await act(async () => {});
  expect(HttpUtil.get).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole('button', { name: /Sync now|Синхронизировать сейчас/ })).toBeNull();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(15000);
  });
  expect(HttpUtil.get).toHaveBeenCalledTimes(2);
  view.unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(30000);
  });
  expect(HttpUtil.get).toHaveBeenCalledTimes(2);
});
