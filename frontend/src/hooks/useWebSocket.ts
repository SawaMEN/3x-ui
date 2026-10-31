import { useEffect, useRef } from 'react';
import { getSharedWebSocketClient } from '@/api/websocket';

type Handler = (payload: unknown) => void;

// Event subscriptions stay stable across renders, but callbacks must always
// use current props/state rather than the closures from the first render.
export function useWebSocket(handlers: Record<string, Handler>) {
  const latestHandlers = useRef(handlers);
  useEffect(() => {
    latestHandlers.current = handlers;
  }, [handlers]);
  const eventNames = Object.keys(handlers).sort().join('\u0000');

  useEffect(() => {
    const client = getSharedWebSocketClient();
    const entries = eventNames
      ? eventNames.split('\u0000').map((event) => {
          const handler = (payload: unknown) => latestHandlers.current[event]?.(payload);
          client.on(event, handler);
          return [event, handler] as const;
        })
      : [];
    client.connect();
    return () => {
      for (const [event, handler] of entries) {
        client.off(event, handler);
      }
    };
  }, [eventNames]);
}
