import { useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';

import { getSharedWebSocketClient } from '@/api/websocket';
import { keys } from '@/api/queryKeys';
import { isRecentLocalInvalidate } from '@/api/invalidationTracker';

type Handler = (payload: unknown) => void;

let invalidateTimer: number | null = null;
const pendingInvalidations = new Set<'inbounds' | 'clients'>();

export function useWebSocketBridge() {
  const queryClient = useQueryClient();

  useEffect(() => {
    const client = getSharedWebSocketClient();

    const onInvalidate: Handler = (payload) => {
      const p = payload as { type?: string } | undefined;
      if (!p || (p.type !== 'inbounds' && p.type !== 'clients')) return;
      pendingInvalidations.add(p.type);
      if (invalidateTimer != null) clearTimeout(invalidateTimer);
      invalidateTimer = window.setTimeout(() => {
        invalidateTimer = null;
        if (isRecentLocalInvalidate()) {
          pendingInvalidations.clear();
          return;
        }
        const pending = new Set(pendingInvalidations);
        pendingInvalidations.clear();
        for (const type of pending) {
          void queryClient.invalidateQueries({ queryKey: [type] });
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

    client.on('invalidate', onInvalidate);
    client.on('outbounds', onOutbounds);
    client.on('nodes', onNodes);
    client.on('inbounds', onInbounds);
    client.connect();

    return () => {
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
