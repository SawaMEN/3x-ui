import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { getSharedWebSocketClient } from '@/api/websocket';
import { keys } from '@/api/queryKeys';

type Handler = (payload: unknown) => void;

export function useWebSocketBridge() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const client = getSharedWebSocketClient();
    const invalidationKeys = new Map<string, readonly string[]>([
      ['inbounds', keys.inbounds.root()],
      ['clients', keys.clients.root()],
      ['nodes', keys.nodes.list()],
      ['outbounds', keys.xray.outboundsTraffic()],
    ]);
    let invalidateTimer: number | null = null;
    const pendingInvalidations = new Set<string>();

    const onInvalidate: Handler = (payload) => {
      const p = payload as { type?: string } | undefined;
      if (!p?.type || !invalidationKeys.has(p.type)) return;
      pendingInvalidations.add(p.type);
      if (invalidateTimer != null) return;
      invalidateTimer = window.setTimeout(() => {
        invalidateTimer = null;
        const pending = new Set(pendingInvalidations);
        pendingInvalidations.clear();
        for (const type of pending) {
          void queryClient.invalidateQueries({ queryKey: invalidationKeys.get(type) });
        }
      }, 200);
    };

    const onOutbounds: Handler = (payload) => {
      if (!Array.isArray(payload)) return;
      queryClient.setQueryData(keys.xray.outboundsTraffic(), payload);
    };

    const onNodes: Handler = (payload) => {
      if (!Array.isArray(payload)) return;
      queryClient.setQueryData(keys.nodes.list(), payload);
    };

    const onInbounds: Handler = (payload) => {
      if (!Array.isArray(payload)) return;
      queryClient.setQueryData(keys.inbounds.slim(), payload);
    };

    const onConnected: Handler = () => {
      for (const queryKey of invalidationKeys.values()) {
        void queryClient.invalidateQueries({ queryKey });
      }
    };

    client.on('connected', onConnected);
    client.on('invalidate', onInvalidate);
    client.on('outbounds', onOutbounds);
    client.on('nodes', onNodes);
    client.on('inbounds', onInbounds);
    client.connect();

    return () => {
      client.off('connected', onConnected);
      client.off('invalidate', onInvalidate);
      client.off('outbounds', onOutbounds);
      client.off('nodes', onNodes);
      client.off('inbounds', onInbounds);
      if (invalidateTimer != null) {
        clearTimeout(invalidateTimer);
        invalidateTimer = null;
      }
      pendingInvalidations.clear();
    };
  }, [queryClient]);
}
