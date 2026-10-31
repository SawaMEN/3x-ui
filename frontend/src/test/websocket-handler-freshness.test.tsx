import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useWebSocket } from '@/hooks/useWebSocket';

const events = vi.hoisted(() => new Map<string, Set<(payload: unknown) => void>>());
const connect = vi.hoisted(() => vi.fn());
vi.mock('@/api/websocket', () => ({
  getSharedWebSocketClient: () => ({
    on: (event: string, callback: (payload: unknown) => void) => {
      if (!events.has(event)) events.set(event, new Set());
      events.get(event)!.add(callback);
    },
    off: (event: string, callback: (payload: unknown) => void) =>
      events.get(event)?.delete(callback),
    connect,
  }),
}));

afterEach(() => {
  events.clear();
  connect.mockClear();
});

describe('page websocket subscriptions', () => {
  it('uses the latest React callback after rerender without accumulating listeners', () => {
    const original = vi.fn();
    const current = vi.fn();
    const { rerender, unmount } = renderHook(({ handler }) => useWebSocket({ traffic: handler }), {
      initialProps: { handler: original },
    });

    expect(events.get('traffic')?.size).toBe(1);
    act(() => {
      for (const callback of events.get('traffic') || []) callback({ up: 1 });
    });
    expect(original).toHaveBeenCalledTimes(1);

    rerender({ handler: current });
    expect(events.get('traffic')?.size).toBe(1);
    act(() => {
      for (const callback of events.get('traffic') || []) callback({ up: 2 });
    });
    expect(original).toHaveBeenCalledTimes(1);
    expect(current).toHaveBeenCalledWith({ up: 2 });

    unmount();
    expect(events.get('traffic')?.size).toBe(0);
  });
});
