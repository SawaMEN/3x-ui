import { act, render } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import ProxyCoreTab from '@/pages/settings/ProxyCoreTab';
import { HttpUtil } from '@/utils';

vi.mock('@/pages/settings/CoreSelection', () => ({ default: () => null }));

afterEach(() => {
  vi.useRealTimers();
});

it('stops retries for a missing core and refreshes after settings are saved', async () => {
  vi.useFakeTimers();
  const get = vi
    .mocked(HttpUtil.get)
    .mockResolvedValue({ success: true, msg: '', obj: { installed: false, running: false } });
  get.mockClear();
  const view = render(<ProxyCoreTab active />);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(65000);
  });
  expect(get).toHaveBeenCalledTimes(2);
  await act(async () => {
    window.dispatchEvent(new Event('xui-core-settings-saved'));
  });
  expect(get).toHaveBeenCalledTimes(4);
  view.unmount();
});

it('bounds retries and pauses them when the core tab is hidden', async () => {
  vi.useFakeTimers();
  const get = vi
    .mocked(HttpUtil.get)
    .mockResolvedValue({ success: true, msg: '', obj: { installed: true, running: false } });
  get.mockClear();
  const view = render(<ProxyCoreTab active />);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(65000);
  });
  expect(get).toHaveBeenCalledTimes(24);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(65000);
  });
  expect(get).toHaveBeenCalledTimes(24);
  view.rerender(<ProxyCoreTab active={false} />);
  await act(async () => {
    window.dispatchEvent(new Event('xui-core-settings-saved'));
    await vi.advanceTimersByTimeAsync(65000);
  });
  expect(get).toHaveBeenCalledTimes(24);
  view.rerender(<ProxyCoreTab active />);
  await act(async () => {});
  expect(get).toHaveBeenCalledTimes(26);
  view.unmount();
});
