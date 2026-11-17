import { act, render, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import TelemtPage from '@/pages/telemt/TelemtPage';
import { HttpUtil, Msg } from '@/utils';
vi.mock('@/layouts/AppSidebar', () => ({ default: () => null }));
vi.mock('@/hooks/useTheme', () => ({ useTheme: () => ({ antdThemeConfig: {} }) }));
afterEach(() => vi.restoreAllMocks());
it('aborts status reads and does not start config/proxy requests after disposal', async () => {
  let complete!: (value: Msg) => void;
  const get = vi.spyOn(HttpUtil, 'get').mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  const { unmount } = render(<TelemtPage />);
  await waitFor(() => expect(get).toHaveBeenCalled());
  const call = get.mock.calls.find(([url]) => url === '/panel/api/telemt/status');
  expect(call?.[2]?.signal).toBeInstanceOf(AbortSignal);
  unmount();
  expect(call?.[2]?.signal?.aborted).toBe(true);
  await act(async () => complete(new Msg(true, '', { installed: true })));
  expect(
    get.mock.calls.some(
      ([url]) => url === '/panel/api/telemt/config' || url === '/panel/api/telemt/proxy',
    ),
  ).toBe(false);
});
