import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router';

import TemplatesPage from '@/pages/templates/TemplatesPage';
import { HttpUtil, Msg } from '@/utils';

import { renderWithProviders } from './test-utils';

vi.mock('@/layouts/AppSidebar', () => ({ default: () => null }));

afterEach(() => vi.restoreAllMocks());

it('keeps the latest template search when an older request finishes later', async () => {
  let finishOld!: (value: Msg<unknown>) => void;
  const get = vi.spyOn(HttpUtil, 'get');
  get.mockImplementationOnce(() => new Promise((resolve) => (finishOld = resolve)));
  get.mockResolvedValue(
    new Msg(true, '', {
      items: [{ id: 2, kind: 'inbound', title: 'Latest result', tags: [], sizeBytes: 20 }],
    }),
  );

  const { unmount } = renderWithProviders(
    <MemoryRouter>
      <TemplatesPage />
    </MemoryRouter>,
  );
  await waitFor(() => expect(get).toHaveBeenCalledTimes(1));
  const oldSignal = get.mock.calls[0][2]?.signal;
  fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'latest' } });
  await screen.findByText('Latest result');

  await act(async () => {
    finishOld(
      new Msg(true, '', {
        items: [{ id: 1, kind: 'inbound', title: 'Stale result', tags: [], sizeBytes: 20 }],
      }),
    );
  });

  expect(screen.queryByText('Stale result')).toBeNull();
  expect(screen.getByText('Latest result')).toBeTruthy();
  expect(oldSignal?.aborted).toBe(true);
  const currentSignal = get.mock.calls[1][2]?.signal;
  unmount();
  expect(currentSignal?.aborted).toBe(true);
});
