import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { expect, it, vi } from 'vitest';
import { ThemeProvider } from '@/hooks/useTheme';
import ClientFormModal from '@/pages/clients/ClientFormModal';
import type { ClientRecord, InboundOption } from '@/schemas/client';

function open(limit?: ClientRecord['speedLimit']) {
  const save = vi.fn().mockResolvedValue({ success: true });
  const query = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <ThemeProvider>
      <QueryClientProvider client={query}>
        <ClientFormModal
          open
          mode="edit"
          client={{
            email: 'limited',
            subId: 'limitedsub',
            uuid: '11111111-1111-4111-8111-111111111111',
            enable: true,
            speedLimit: limit,
          }}
          inbounds={[
            { id: 1, protocol: 'vless', tag: 'in', port: 443, enable: true } as InboundOption,
          ]}
          attachedIds={[1]}
          save={save}
          onOpenChange={() => {}}
        />
      </QueryClientProvider>
    </ThemeProvider>,
  );
  return save;
}

it('keeps existing rates when saving the client and explicitly disables them', async () => {
  const save = open({ enabled: true, uploadMbps: 5, downloadMbps: 10 });
  expect(await screen.findByRole('spinbutton', { name: 'Download, Mbps' })).toHaveProperty(
    'value',
    '10',
  );
  fireEvent.click(screen.getByRole('switch', { name: 'Limit traffic speed' }));
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].speedLimit).toEqual({
    enabled: false,
    uploadMbps: 5,
    downloadMbps: 10,
  });
});

it('rejects enabled zero rates and saves a real directional cap', async () => {
  const save = open();
  fireEvent.click(await screen.findByRole('switch', { name: 'Limit traffic speed' }));
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await screen.findByText('Set a positive upload or download speed.');
  expect(save).not.toHaveBeenCalled();
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Download, Mbps' }), {
    target: { value: '20' },
  });
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].speedLimit).toEqual({
    enabled: true,
    uploadMbps: 0,
    downloadMbps: 20,
  });
}, 15000);
