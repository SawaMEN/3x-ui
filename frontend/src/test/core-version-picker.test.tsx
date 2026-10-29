import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { beforeEach, expect, it, vi } from 'vitest';
import VersionModal from '@/pages/index/VersionModal';
import { HttpUtil } from '@/utils';
import type { Status } from '@/models/status';

let coreType = 'xray';
vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: () => ({ allSetting: { coreType } }),
}));

beforeEach(() => {
  coreType = 'xray';
  vi.mocked(HttpUtil.get).mockImplementation(async (path) => ({
    success: true,
    msg: '',
    obj: String(path).endsWith('/versions')
      ? [
          { tag: 'v26.9.0', prerelease: false },
          { tag: 'v26.10.0', prerelease: true },
        ]
      : { cores: [{ type: 'xray', version: '26.8.0' }] },
  }));
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: {} });
});

function open() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <VersionModal open status={{} as Status} onClose={vi.fn()} onBusy={vi.fn()} />
    </QueryClientProvider>,
  );
}

it('sends the chosen release to the active core update endpoint', async () => {
  open();
  await screen.findByText('26.8.0');
  const select = screen.getByRole('combobox', { name: 'Версия ядра' });
  fireEvent.mouseDown(select.closest('.ant-select')!.querySelector('.ant-select-content')!);
  fireEvent.click(await screen.findAllByText('v26.9.0').then((items) => items[items.length - 1]));
  fireEvent.click(screen.getByRole('button', { name: 'Update' }));
  fireEvent.click(await screen.findByRole('button', { name: 'OK' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/cores/xray/update',
      { version: 'v26.9.0' },
      { timeout: 330000 },
    ),
  );
});

it('keeps the bundled core on panel updates', async () => {
  coreType = 'sing-box';
  open();
  await waitFor(() => expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/setting/cores'));
  expect(screen.queryByRole('combobox', { name: 'Версия ядра' })).toBeNull();
});
