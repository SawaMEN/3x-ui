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

test('refreshes service state until a background restart completes', async () => {
  vi.useFakeTimers();
  try {
    const { act } = await import('@testing-library/react');
    const { default: OlcRTCManagement } = await import('@/pages/olcrtc/OlcRTCManagement');
    const get = vi
      .spyOn(HttpUtil, 'get')
      .mockResolvedValueOnce({
        success: true,
        obj: { installed: true, running: false, busy: true },
      } as never)
      .mockResolvedValueOnce({
        success: true,
        obj: { installed: true, running: true, busy: false },
      } as never);
    renderWithProviders(<OlcRTCManagement />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('Служба остановлена')).toBeTruthy();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
    });
    expect(screen.getByText('Служба работает')).toBeTruthy();
    const calls = get.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(get.mock.calls.length).toBe(calls);
  } finally {
    vi.useRealTimers();
  }
});
