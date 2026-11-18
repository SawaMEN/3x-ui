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
          { tag: 'v26.10.0-beta.1', prerelease: true },
        ]
      : { cores: [{ type: coreType, version: '26.8.0' }] },
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

it('shows stable releases as selectable rows and updates the chosen Xray version', async () => {
  open();
  await screen.findByRole('radio', { name: 'Выбрать v26.9.0' });
  expect(screen.queryByRole('radio', { name: 'Выбрать v26.10.0-beta.1' })).toBeNull();
  fireEvent.click(screen.getByRole('radio', { name: 'Выбрать v26.9.0' }));
  fireEvent.click(await screen.findByRole('button', { name: /^(Confirm|Подтвердить)$/ }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/cores/xray/update',
      { version: 'v26.9.0' },
      { timeout: 960000 },
    ),
  );
});

it('switches between stable and beta releases for upstream sing-box', async () => {
  coreType = 'sing-box-upstream';
  open();
  await screen.findByRole('radio', { name: 'Выбрать v26.9.0' });
  fireEvent.click(screen.getByText('Бета'));
  const beta = await screen.findByRole('radio', { name: 'Выбрать v26.10.0-beta.1' });
  expect(screen.queryByRole('radio', { name: 'Выбрать v26.9.0' })).toBeNull();
  fireEvent.click(beta);
  fireEvent.click(await screen.findByRole('button', { name: /^(Confirm|Подтвердить)$/ }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/cores/sing-box-upstream/update',
      { version: 'v26.10.0-beta.1' },
      { timeout: 960000 },
    ),
  );
});

it('does not show installation controls for the bundled sing-box core', async () => {
  coreType = 'sing-box';
  open();
  await waitFor(() => expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/setting/cores'));
  expect(screen.queryByRole('radio')).toBeNull();
  expect(
    screen.queryByText(
      'Последний стабильный выпуск. Перед заменой проверяется конфигурация; при неудачном запуске восстанавливается прежнее ядро.',
    ),
  ).toBeNull();
});
