import { fireEvent, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import ClientFormModal from '@/pages/clients/ClientFormModal';
import { ClientFormSchema } from '@/schemas/client';
import type { ClientRecord } from '@/schemas/client';
import { renderWithProviders } from './test-utils';

function open(mode?: ClientRecord['connectionNameMode']) {
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
        connectionNameMode: mode,
      }}
      inbounds={[{ id: 1, protocol: 'vless', tag: 'in', port: 443 }]}
      attachedIds={[1]}
      save={save}
      onOpenChange={() => {}}
    />,
  );
  return save;
}

it.each(['remark', 'remark-client', 'client'] as const)(
  'preserves selected mode %s on unrelated edits',
  async (mode) => {
    const save = open(mode);
    await screen.findByRole('combobox', { name: 'Connection naming' });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));
    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0][0].connectionNameMode).toBe(mode);
  },
);

it('offers exactly the three requested choices and saves a different selection', async () => {
  const save = open('remark');
  const select = await screen.findByRole('combobox', { name: 'Connection naming' });
  fireEvent.mouseDown(
    select.closest('.ant-select')!.querySelector('.ant-select-selector') || select,
  );
  const labels = ['Connection name (remark)', 'Name-client', 'Client'];
  await waitFor(() =>
    expect(
      document.querySelectorAll(
        '.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option',
      ).length,
    ).toBe(3),
  );
  const options = Array.from(
    document.querySelectorAll(
      '.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option',
    ),
  );
  expect(options.map((option) => option.getAttribute('title'))).toEqual(labels);
  fireEvent.click(options[1]);
  fireEvent.click(screen.getByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].connectionNameMode).toBe('remark-client');
});

it('defaults to the connection remark and rejects unsupported modes', async () => {
  const save = open();
  fireEvent.click(await screen.findByRole('button', { name: /save/i }));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].connectionNameMode).toBe('remark');
  expect(ClientFormSchema.shape.connectionNameMode.safeParse('arbitrary name').success).toBe(false);
});
