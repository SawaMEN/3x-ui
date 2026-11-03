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
  const status = {
    supported: true,
    backend: 'iptables',
    enabled: true,
    autoSync: false,
    syncInterval: 60,
    pingEnabled: true,
    rules: [],
    manualRules: [],
  };
  vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, msg: '', obj: status });
  vi.mocked(HttpUtil.post).mockResolvedValue({
    success: true,
    msg: '',
    obj: { ...status, syncInterval: 120 },
  });
  const view = render(<FirewallManager />);
  const input = await screen.findByRole('spinbutton', {
    name: /Rule check interval|Период проверки правил/,
  });
  fireEvent.change(input, { target: { value: '120' } });
  await act(async () => {
    fireEvent.blur(input);
  });
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/server/firewall/auto-sync',
    { enabled: false, syncInterval: 120 },
    { silentSuccess: true },
  );
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

it('shows unknown rules with delete only and sends their opaque ID', async () => {
  const rule = { id: 'a'.repeat(64), label: 'admin INPUT source 10.0.0.0/8 drop' };
  const status = {
    supported: true,
    backend: 'nftables',
    enabled: true,
    autoSync: false,
    pingEnabled: true,
    rules: [],
    manualRules: [],
    externalRules: [rule],
  };
  vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, msg: '', obj: status });
  vi.mocked(HttpUtil.post).mockResolvedValue({
    success: true,
    msg: '',
    obj: { ...status, externalRules: [] },
  });
  const view = render(<FirewallManager />);
  expect(await screen.findByText(/UNKNOWN|НЕИЗВЕСТНО/)).toBeTruthy();
  expect(screen.getByText(rule.label)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: /^(Delete|Удалить)$/ }));
  fireEvent.click(await screen.findByRole('button', { name: /^(OK|Да)$/ }));
  await act(async () => {});
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/server/firewall/rules/delete-external',
    { id: rule.id },
    { silentSuccess: true },
  );
  view.unmount();
});
