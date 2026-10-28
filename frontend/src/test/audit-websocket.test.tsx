import { afterEach, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useWebSocketBridge } from '@/api/websocketBridge';
import { keys } from '@/api/queryKeys';
import { useClients } from '@/hooks/useClients';
import { HttpUtil, Msg } from '@/utils';

const listeners = vi.hoisted(() => new Map<string, (...args: unknown[]) => void>());
vi.mock('@/api/websocket', () => ({
  getSharedWebSocketClient: () => ({
    on: (event: string, fn: (...args: unknown[]) => void) => listeners.set(event, fn),
    off: (event: string) => listeners.delete(event),
    connect: () => {},
  }),
}));
afterEach(() => {
  vi.useRealTimers();
  listeners.clear();
  vi.restoreAllMocks();
});

function mountBridge() {
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, 'invalidateQueries');
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', {}));
  const hook = renderHook(
    () => {
      useWebSocketBridge();
      return useClients({ list: false });
    },
    { wrapper },
  );
  return { invalidate, ...hook };
}

it('does not lose a different admin mutation immediately after a local save', async () => {
  vi.useFakeTimers();
  const { invalidate, unmount, result } = mountBridge();
  await act(async () => {
    await result.current.refresh();
  });
  invalidate.mockClear();
  act(() => {
    listeners.get('invalidate')?.({ type: 'inbounds' });
    vi.advanceTimersByTime(200);
  });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.inbounds.root() });
  unmount();
});

it('refreshes cached data after a websocket outage', () => {
  const { invalidate, unmount } = mountBridge();
  act(() => listeners.get('connected')?.());
  expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.inbounds.root() });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.clients.root() });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.nodes.list() });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.xray.outboundsTraffic() });
  unmount();
});
