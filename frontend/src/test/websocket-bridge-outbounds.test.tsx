import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import { getSharedWebSocketClient } from '@/api/websocket';
import { useWebSocketBridge } from '@/api/websocketBridge';
import { keys } from '@/api/queryKeys';

type ListenerMap = { listeners: Map<string, Set<(payload: unknown) => void>> };

describe('websocket bridge outbounds handler', () => {
  let originalWS: typeof WebSocket;

  beforeEach(() => {
    originalWS = globalThis.WebSocket;
    class FakeWebSocket {
      static readonly CONNECTING = 0;
      static readonly OPEN = 1;
      static readonly CLOSING = 2;
      static readonly CLOSED = 3;
      readyState = FakeWebSocket.CONNECTING;
      addEventListener() {}
      removeEventListener() {}
      close() {}
      send() {}
    }
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket;
  });

  afterEach(() => {
    globalThis.WebSocket = originalWS;
    vi.useRealTimers();
  });

  it('ignores a non-array outbounds push instead of poisoning the cache', () => {
    const queryClient = new QueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    renderHook(() => useWebSocketBridge(), { wrapper });

    const client = getSharedWebSocketClient() as unknown as ListenerMap;
    const handlers = client.listeners.get('outbounds');
    expect(handlers && handlers.size).toBeGreaterThan(0);

    for (const handler of handlers ?? []) handler({ not: 'an array' });

    expect(queryClient.getQueryData(keys.xray.outboundsTraffic())).toBeUndefined();
  });

  it('refetches oversized node and outbound pushes without postponing under a burst', () => {
    vi.useFakeTimers();
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { unmount } = renderHook(() => useWebSocketBridge(), { wrapper });
    const client = getSharedWebSocketClient() as unknown as ListenerMap;
    const emit = (type: string) => {
      for (const handler of client.listeners.get('invalidate') ?? []) handler({ type });
    };

    act(() => {
      emit('nodes');
      vi.advanceTimersByTime(100);
      emit('outbounds');
      emit('nodes');
      vi.advanceTimersByTime(100);
    });
    expect(invalidate).toHaveBeenCalledTimes(2);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.nodes.list() });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.xray.outboundsTraffic() });

    act(() => emit('nodes'));
    unmount();
    act(() => vi.advanceTimersByTime(200));
    expect(invalidate).toHaveBeenCalledTimes(2);
  });

  it('refreshes clients during continuous invalidations', () => {
    vi.useFakeTimers();
    const queryClient = new QueryClient();
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { unmount } = renderHook(() => useWebSocketBridge(), { wrapper });
    const client = getSharedWebSocketClient() as unknown as ListenerMap;
    act(() => {
      for (let i = 0; i < 4; i++) {
        for (const handler of client.listeners.get('invalidate') ?? [])
          handler({ type: 'clients' });
        vi.advanceTimersByTime(100);
      }
    });
    expect(invalidate).toHaveBeenCalledTimes(2);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: keys.clients.root() });
    unmount();
  });
});
