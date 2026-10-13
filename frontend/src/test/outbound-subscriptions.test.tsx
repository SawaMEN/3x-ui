import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import OutboundSubscriptions from '@/pages/singbox/OutboundSubscriptions';
import { HttpUtil } from '@/utils';

it('saves and fetches a subscription and exposes its automatic cascade tag', async () => {
  const onTagsChanged = vi.fn();
  vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, msg: '', obj: [] });
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj:
      path === '/panel/api/xray/outbound-subs'
        ? { id: 7 }
        : path === '/panel/api/xray/'
          ? { subscriptionOutboundTags: ['sub-auto-7', 'node'] }
          : [],
  }));
  render(
    <OutboundSubscriptions
      open
      initialURL="https://provider.test/sub"
      onClose={() => {}}
      onTagsChanged={onTagsChanged}
    />,
  );
  await screen.findByDisplayValue('https://provider.test/sub');
  fireEvent.click(screen.getByRole('button', { name: 'Добавить и загрузить' }));
  await waitFor(() => expect(onTagsChanged).toHaveBeenCalledWith(['sub-auto-7', 'node']));
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/xray/outbound-subs',
    expect.objectContaining({
      url: 'https://provider.test/sub',
      autoBalance: true,
      updateInterval: 600,
    }),
    { silent: true },
  );
  expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/xray/outbound-subs/7/refresh', undefined, {
    silent: true,
  });
});

it('keeps a saved subscription visible for retry when the first download fails', async () => {
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: [
      {
        id: 7,
        remark: 'Saved',
        url: 'https://provider.test/sub',
        enabled: true,
        autoBalance: true,
        outboundCount: 0,
      },
    ],
  });
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => ({
    success: !String(path).endsWith('/refresh'),
    msg: String(path).endsWith('/refresh') ? 'Download failed' : '',
    obj: path === '/panel/api/xray/outbound-subs' ? { id: 7 } : { subscriptionOutboundTags: [] },
  }));
  render(
    <OutboundSubscriptions
      open
      initialURL="https://provider.test/sub"
      onClose={() => {}}
      onTagsChanged={() => {}}
    />,
  );
  await screen.findByDisplayValue('https://provider.test/sub');
  fireEvent.click(screen.getByRole('button', { name: 'Добавить и загрузить' }));
  await screen.findByText('Download failed');
  expect(screen.getByText('Saved')).toBeTruthy();
  await waitFor(() =>
    expect((screen.getByLabelText('URL подписки') as HTMLInputElement).value).toBe(''),
  );
  expect(
    vi
      .mocked(HttpUtil.post)
      .mock.calls.filter(([path]) => path === '/panel/api/xray/outbound-subs'),
  ).toHaveLength(1);
});
