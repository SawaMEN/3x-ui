import { act, screen, fireEvent, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import LogModal from '@/pages/index/LogModal';
import XrayLogModal from '@/pages/index/XrayLogModal';
import AmneziaWGLogModal from '@/pages/index/AmneziaWGLogModal';
import VkTurnProxyLogModal from '@/pages/index/VkTurnProxyLogModal';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';
afterEach(() => vi.restoreAllMocks());
it.each([
  { name: 'panel', Viewer: LogModal },
  { name: 'core', Viewer: XrayLogModal },
  { name: 'AmneziaWG', Viewer: AmneziaWGLogModal },
  { name: 'VK TURN', Viewer: VkTurnProxyLogModal },
])('cancels a pending request when $name is unmounted', async ({ Viewer }) => {
  let complete!: (value: Msg) => void;
  const post = vi.spyOn(HttpUtil, 'post').mockImplementation((url) => {
    if (!url.includes('/server/')) return Promise.resolve(new Msg(false));
    return new Promise((resolve) => {
      complete = resolve;
    });
  });
  const { unmount } = renderWithProviders(<Viewer open onClose={() => {}} />);
  await waitFor(() => expect(post.mock.calls.some(([url]) => url.includes('/server/'))).toBe(true));
  const signal = post.mock.calls.find(([url]) => url.includes('/server/'))?.[2]?.signal;
  expect(signal).toBeInstanceOf(AbortSignal);
  unmount();
  expect(signal?.aborted).toBe(true);
  await act(async () => complete(new Msg(false)));
});
it('does not let a slow previous response replace a manually refreshed log', async () => {
  const complete: Array<(value: Msg) => void> = [];
  vi.spyOn(HttpUtil, 'post').mockImplementation(
    () =>
      new Promise((resolve) => {
        complete.push(resolve);
      }),
  );
  renderWithProviders(<LogModal open onClose={() => {}} />);
  await waitFor(() => expect(complete).toHaveLength(1));
  fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
  await waitFor(() => expect(complete).toHaveLength(2));
  await act(async () =>
    complete[1](
      new Msg(true, '', {
        entries: [{ source: 'panel', level: 'INFO', raw: 'latest record' }],
        warnings: [],
      }),
    ),
  );
  expect(await screen.findByText('latest record')).toBeTruthy();
  await act(async () =>
    complete[0](
      new Msg(true, '', {
        entries: [{ source: 'panel', level: 'INFO', raw: 'stale record' }],
        warnings: [],
      }),
    ),
  );
  expect(screen.queryByText('stale record')).toBeNull();
  expect(screen.getByText('latest record')).toBeTruthy();
});
