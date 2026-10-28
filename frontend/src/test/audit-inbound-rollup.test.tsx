import type { ReactNode } from 'react';
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import { expect, it } from 'vitest';
import { keys } from '@/api/queryKeys';
import { useInbounds } from '@/pages/inbounds/useInbounds';
import { makeTestQueryClient } from './test-utils';

it.each(['mieru', 'masque', 'naive', 'anytls', 'shadowtls', 'vk-turn-proxy'])(
  'counts clients and depleted accounts on %s listeners',
  async (protocol) => {
    const client = makeTestQueryClient();
    client.setQueryData(keys.inbounds.slim(), [
      {
        id: 1,
        protocol,
        enable: true,
        tag: 'audit',
        settings: JSON.stringify({
          clients: [
            { email: 'active', enable: true },
            { email: 'depleted', enable: true },
          ],
        }),
        clientStats: [
          { email: 'active', up: 1, down: 1, total: 100, expiryTime: 0, enable: true },
          { email: 'depleted', up: 100, down: 1, total: 100, expiryTime: 0, enable: false },
        ],
      },
    ]);
    client.setQueryData(keys.clients.onlines(), []);
    client.setQueryData(keys.clients.onlinesByGuid(), {});
    client.setQueryData(keys.clients.activeInbounds(), {});
    client.setQueryData(keys.clients.lastOnline(), {});
    client.setQueryData(keys.settings.defaults(), {});
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result, unmount } = renderHook(useInbounds, { wrapper });
    await waitFor(() => expect(result.current.dbInbounds).toHaveLength(1));
    expect(result.current.clientCount[1]).toMatchObject({
      clients: 2,
      active: ['active'],
      depleted: ['depleted'],
    });
    unmount();
  },
);
