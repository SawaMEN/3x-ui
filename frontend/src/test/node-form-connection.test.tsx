import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import NodeFormModal from '@/pages/nodes/NodeFormModal';
import { Msg } from '@/utils';
import type { ProbeResult } from '@/schemas/node';
import { chooseSelectOption, renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useOutboundTags', () => ({
  useOutboundTagGroups: () => ({ data: { outbounds: ['via-vpn'], balancers: [] } }),
}));

function form() {
  const props = {
    open: true,
    mode: 'add' as const,
    node: null,
    testConnection: vi.fn().mockResolvedValue(
      new Msg<ProbeResult>(true, '', {
        status: 'offline',
        error: 'save the node before probing',
      }),
    ),
    fetchFingerprint: vi.fn(),
    fetchInbounds: vi.fn(),
    save: vi.fn().mockResolvedValue(new Msg(true, '', { id: 7 })),
    onOpenChange: vi.fn(),
  };
  renderWithProviders(<NodeFormModal {...props} />);
  fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), {
    target: { value: 'new-node' },
  });
  fireEvent.change(screen.getByRole('textbox', { name: 'Address' }), {
    target: { value: 'node.example.org' },
  });
  fireEvent.change(screen.getByLabelText('API Token'), { target: { value: 'test-token' } });
  return props;
}

describe('new node connection', () => {
  it('saves direct nodes through server validation without an extra probe', async () => {
    const props = form();
    fireEvent.click(screen.getByRole('button', { name: /^Save$/ }));
    await waitFor(() => expect(props.save).toHaveBeenCalledOnce());
    expect(props.save).toHaveBeenCalledWith(
      expect.objectContaining({
        address: 'node.example.org',
        apiToken: 'test-token',
        outboundTag: '',
      }),
    );
    expect(props.testConnection).not.toHaveBeenCalled();
    await waitFor(() => expect(props.onOpenChange).toHaveBeenCalledWith(false));
  });

  it('allows a new routed node to be saved and explains its pending pre-save check', async () => {
    const props = form();
    const picker = screen.getByRole('combobox', { name: 'Connection outbound' });
    chooseSelectOption(picker.id, 'via-vpn');
    fireEvent.click(screen.getByRole('button', { name: 'Test Connection' }));
    await screen.findByText('Save the node to apply its connection route and check connectivity.');
    expect(props.testConnection).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /^Save$/ }));
    await waitFor(() =>
      expect(props.save).toHaveBeenCalledWith(expect.objectContaining({ outboundTag: 'via-vpn' })),
    );
    await waitFor(() => expect(props.onOpenChange).toHaveBeenCalledWith(false));
  });

  it('keeps the form open and shows the server connection failure', async () => {
    const props = form();
    props.save.mockResolvedValueOnce(new Msg(false, 'HTTP 403 from remote panel', null));
    fireEvent.click(screen.getByRole('button', { name: /^Save$/ }));
    await screen.findByText('HTTP 403 from remote panel');
    expect(props.onOpenChange).not.toHaveBeenCalled();
  });

  it('still performs an explicit direct connection test', async () => {
    const props = form();
    props.testConnection.mockResolvedValueOnce(
      new Msg<ProbeResult>(true, '', { status: 'online', latencyMs: 12 }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Test Connection' }));
    await waitFor(() => expect(props.testConnection).toHaveBeenCalledOnce());
    expect(props.save).not.toHaveBeenCalled();
  });
});
