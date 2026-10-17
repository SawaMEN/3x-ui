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

it('shows compatibility warnings and can rank and ping the applied automatic route', async () => {
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: [
      {
        id: 7,
        remark: 'Mixed',
        url: 'https://provider.test/sub',
        enabled: true,
        autoBalance: true,
        outboundCount: 3,
        supportedCount: 2,
        lastWarning: 'Один протокол не поддерживается',
        selectedTag: 'best',
        appliedTag: 'best',
      },
    ],
  });
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.post).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).endsWith('/check')
      ? { delay: 42 }
      : path === '/panel/api/xray/'
        ? { subscriptionOutboundTags: ['sub-auto-7'] }
        : {},
  }));
  render(<OutboundSubscriptions open initialURL="" onClose={() => {}} onTagsChanged={() => {}} />);
  expect(await screen.findByText('Один протокол не поддерживается')).toBeTruthy();
  expect(screen.getByText('Применимо: 2 из 3')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Пинг' }));
  expect(await screen.findByText('42 мс')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Проверить и выбрать лучший' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/xray/outbound-subs/7/probe', undefined, {
      silent: true,
    }),
  );
});

it('groups incompatible subscription nodes instead of printing a wall of kernel errors', async () => {
  const problems = [
    'sub1-grpc-a: sing-box outbound "sub1-grpc-a" uses Xray gRPC authority "example.test" which sing-box V2Ray gRPC transport cannot represent',
    'sub1-grpc-b: sing-box outbound "sub1-grpc-b" uses Xray gRPC authority "example.test" which sing-box V2Ray gRPC transport cannot represent',
    'sub1-rawhttp: connection "sub1-rawhttp": TCP header "http" is not wire-compatible with sing-box',
  ];
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: [
      {
        id: 4,
        remark: 'Compatibility',
        url: 'https://provider.test/sub',
        enabled: true,
        autoBalance: true,
        outboundCount: 59,
        supportedCount: 56,
        lastWarning:
          '3 подключений не поддерживаются текущим ядром sing-box; сохранены в подписке, но не применены: ' +
          problems.join('; '),
        applyError: 'селектор sub-auto-4 ещё не загружен в ядро; требуется перезапуск sing-box',
        probeError: 'No healthy outbound; automatic route is blocked',
      },
    ],
  });
  render(<OutboundSubscriptions open initialURL="" onClose={() => {}} onTagsChanged={() => {}} />);
  expect(await screen.findByText('Применимо: 56 из 59')).toBeTruthy();
  expect(screen.getByText('Недоступно: 3')).toBeTruthy();
  expect(screen.getByText('gRPC: несовместимый authority: 2')).toBeTruthy();
  expect(screen.getByText('TCP: HTTP-заголовок Xray: 1')).toBeTruthy();
  expect(screen.getByText('Изменения ожидают перезапуска sing-box')).toBeTruthy();
  expect(
    screen.getByText('Нет доступных узлов: автоматический маршрут временно заблокирован'),
  ).toBeTruthy();
  fireEvent.click(screen.getByText('Показать причины по подключениям'));
  expect(await screen.findByText(problems[0])).toBeTruthy();
});
