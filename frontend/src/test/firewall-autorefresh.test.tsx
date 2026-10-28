import { act, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { FirewallManager } from '@/pages/firewall/FirewallManager';
import { HttpUtil } from '@/utils';

vi.mock('@/utils', () => ({ HttpUtil: { get: vi.fn(), post: vi.fn() } }));

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
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
