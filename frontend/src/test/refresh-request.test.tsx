import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { useRefreshRequest } from '@/hooks/useRefreshRequest';
function deferred() {
  let resolve!: (value: string | undefined) => void;
  const promise = new Promise<string | undefined>((yes) => {
    resolve = yes;
  });
  return { promise, resolve };
}
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});
it('ignores a stale response even when the server ignores abort', async () => {
  const first = deferred(),
    second = deferred();
  const load = vi
    .fn<(signal: AbortSignal) => Promise<string | undefined>>()
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const { result } = renderHook(() => useRefreshRequest(true, load, 'initial'));
  await waitFor(() => expect(load).toHaveBeenCalledTimes(1));
  act(() => result.current.refresh());
  expect(load.mock.calls[0][0].aborted).toBe(true);
  await act(async () => second.resolve('new'));
  await act(async () => first.resolve('old'));
  expect(result.current.data).toBe('new');
  expect(result.current.loading).toBe(false);
});
it('cancels closed viewers and ignores responses from a previous opening', async () => {
  const first = deferred(),
    second = deferred();
  const load = vi
    .fn<(signal: AbortSignal) => Promise<string | undefined>>()
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const { result, rerender } = renderHook(({ open }) => useRefreshRequest(open, load, 'initial'), {
    initialProps: { open: true },
  });
  await waitFor(() => expect(load).toHaveBeenCalledTimes(1));
  rerender({ open: false });
  expect(load.mock.calls[0][0].aborted).toBe(true);
  expect(result.current.loading).toBe(false);
  rerender({ open: true });
  await waitFor(() => expect(load).toHaveBeenCalledTimes(2));
  await act(async () => first.resolve('old'));
  expect(result.current.data).toBe('initial');
  expect(result.current.loading).toBe(true);
  await act(async () => second.resolve('current'));
  expect(result.current.data).toBe('current');
});
it('cancels the previous filter and aborts pending work on unmount', async () => {
  const first = deferred(),
    second = deferred();
  const old = vi.fn((_signal: AbortSignal) => first.promise),
    next = vi.fn((_signal: AbortSignal) => second.promise);
  const { result, rerender, unmount } = renderHook(
    ({ load }) => useRefreshRequest(true, load, 'initial'),
    { initialProps: { load: old } },
  );
  await waitFor(() => expect(old).toHaveBeenCalledOnce());
  rerender({ load: next });
  await waitFor(() => expect(next).toHaveBeenCalledOnce());
  await act(async () => first.resolve('stale filter'));
  expect(result.current.data).toBe('initial');
  expect(old.mock.calls[0][0].aborted).toBe(true);
  unmount();
  expect(next.mock.calls[0][0].aborted).toBe(true);
  await act(async () => second.resolve('late'));
});
it('keeps good data on a rejected refresh', async () => {
  const load = vi
    .fn<(signal: AbortSignal) => Promise<string | undefined>>()
    .mockResolvedValueOnce('good')
    .mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useRefreshRequest(true, load, 'initial'));
  await waitFor(() => expect(result.current.data).toBe('good'));
  act(() => result.current.refresh());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.data).toBe('good');
});
it('avoids overlapping polls and pauses in a hidden tab', async () => {
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] });
  const first = deferred(),
    second = deferred();
  const load = vi
    .fn<(signal: AbortSignal) => Promise<string | undefined>>()
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const { result } = renderHook(() => useRefreshRequest(true, load, 'initial', 50));
  await act(async () => {
    await Promise.resolve();
  });
  expect(load).toHaveBeenCalledTimes(1);
  await act(async () => vi.advanceTimersByTimeAsync(250));
  expect(load).toHaveBeenCalledTimes(1);
  await act(async () => first.resolve('ready'));
  const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true);
  await act(async () => vi.advanceTimersByTimeAsync(250));
  expect(load).toHaveBeenCalledTimes(1);
  hidden.mockReturnValue(false);
  await act(async () => vi.advanceTimersByTimeAsync(50));
  expect(load).toHaveBeenCalledTimes(2);
  await act(async () => second.resolve('latest'));
  expect(result.current.data).toBe('latest');
});
