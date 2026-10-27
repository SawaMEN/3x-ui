import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import ClientFormModal from '@/pages/clients/ClientFormModal';
import { ClientFormSchema } from '@/schemas/client';
import { renderWithProviders } from './test-utils';

function open(connectionName?: string) {
  const save = vi.fn().mockResolvedValue({ success: true });
  renderWithProviders(
    <ClientFormModal
      open
      mode="edit"
      client={{
        email: 'named',
        subId: 'namedsub',
        uuid: '11111111-1111-4111-8111-111111111111',
        enable: true,
        connectionName,
      }}
      inbounds={[{ id: 1, protocol: 'vless', tag: 'in', port: 443 }]}
      attachedIds={[1]}
      save={save}
      onOpenChange={() => {}}
    />,
  );
  return save;
}

it('saves one client connection name and retains it on unrelated edits', async () => {
  const save = open('Мой VPN / #1');
  expect(await screen.findByRole('textbox', { name: 'Connection name' })).toHaveProperty(
    'value',
    'Мой VPN / #1',
  );
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].connectionName).toBe('Мой VPN / #1');
});

it('explicitly clears the override to restore the subscription naming template', async () => {
  const save = open('Old name');
  fireEvent.change(await screen.findByRole('textbox', { name: 'Connection name' }), {
    target: { value: '' },
  });
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].connectionName).toBe('');
});

it('adds and trims a name for a client created before the option existed', async () => {
  const save = open();
  fireEvent.change(await screen.findByRole('textbox', { name: 'Connection name' }), {
    target: { value: '  Общий VPN  ' },
  });
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].connectionName).toBe('Общий VPN');
});

it('rejects oversized names and control characters in the form schema', () => {
  const name = ClientFormSchema.shape.connectionName;
  expect(name.safeParse('a'.repeat(257)).success).toBe(false);
  expect(name.safeParse('A\nB').success).toBe(false);
  expect(name.safeParse('A\u0085B').success).toBe(false);
  expect(name.safeParse('VPN #1').success).toBe(true);
});
