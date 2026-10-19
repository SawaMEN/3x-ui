import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

import OlcRTCClientsModal from '@/pages/olcrtc/OlcRTCClientsModal';
import { ClipboardManager, HttpUtil } from '@/utils';
import { renderWithProviders } from './test-utils';

beforeEach(() => {
  vi.spyOn(HttpUtil, 'get').mockImplementation(async (url) => {
    if (String(url).endsWith('/live')) throw new Error('metrics unavailable');
    return {
      success: true,
      obj: [
        {
          id: 'alice',
          locations: [],
          simpleEditable: false,
          link: 'olcrtc://test-link',
        },
      ],
    } as never;
  });
});

afterEach(() => vi.restoreAllMocks());

test('keeps clients available when live metrics cannot be fetched', async () => {
  renderWithProviders(<OlcRTCClientsModal open onClose={() => {}} />);
  expect(await screen.findByText('alice')).toBeTruthy();
  expect(screen.queryByText('metrics unavailable')).toBeNull();
});

test('copies a connection through the shared HTTP-compatible clipboard helper', async () => {
  const copy = vi.spyOn(ClipboardManager, 'copyText').mockResolvedValue(true);
  renderWithProviders(<OlcRTCClientsModal open onClose={() => {}} />);
  await screen.findByText('alice');
  fireEvent.click(screen.getByRole('button', { name: /Ссылка/ }));
  await waitFor(() => expect(copy).toHaveBeenCalledWith('olcrtc://test-link'));
});
