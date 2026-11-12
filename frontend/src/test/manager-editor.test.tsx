import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { HttpUtil } from '@/utils';
import SubscriptionEditor from '@/pages/manager/SubscriptionEditor';
import { newSubscription } from '@/pages/manager/schema';
vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: [{ id: 3, email: 'alice' }], isFetching: false }),
  useQueryClient: () => ({ invalidateQueries: vi.fn(), setQueryData: vi.fn() }),
}));
vi.mock('@/api/queries/useInboundOptions', () => ({
  useInboundOptions: () => ({
    data: [{ id: 1, remark: 'VLESS', tag: 'vless', protocol: 'vless', port: 443 }],
  }),
}));
vi.mock('@/api/queries/useNodesQuery', () => ({ useNodesQuery: () => ({ nodes: [] }) }));
beforeEach(() => {
  vi.spyOn(HttpUtil, 'post').mockResolvedValue({
    success: true,
    msg: '',
    obj: { ready: true, entries: [] },
  });
});
describe('manager editor', () => {
  it('does not send an invalid draft', async () => {
    const save = vi.fn();
    const close = vi.fn();
    render(
      <SubscriptionEditor
        value={newSubscription()}
        relays={[]}
        domains={[]}
        onSave={save}
        onClose={close}
        onTemplate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Save and generate' }));
    await waitFor(() => expect(screen.getAllByRole('alert').length).toBeGreaterThan(1));
    expect(save).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
  });
  it('preserves custom protocol links when saving and generating', async () => {
    const save = vi.fn().mockResolvedValue(true);
    const close = vi.fn();
    const value = {
      ...newSubscription(),
      name: 'Team',
      entries: [
        {
          templateId: 0,
          mode: 'custom' as const,
          port: '',
          sni: '',
          label: 'AnyTLS',
          flag: '🇫🇮',
          relayId: 0,
          customLink: 'anytls://secret@vpn.example:443#Team',
          activeInboundId: 0,
        },
      ],
    };
    render(
      <SubscriptionEditor
        value={value}
        relays={[]}
        domains={[]}
        onSave={save}
        onClose={close}
        onTemplate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Save and generate' }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'Team', entries: value.entries }),
        true,
      ),
    );
    await waitFor(() => expect(close).toHaveBeenCalledOnce());
  });
  it('retains a draft after a rejected save', async () => {
    const save = vi.fn().mockResolvedValue(false);
    const close = vi.fn();
    render(
      <SubscriptionEditor
        value={{ ...newSubscription(), name: 'Team' }}
        relays={[]}
        domains={[]}
        onSave={save}
        onClose={close}
        onTemplate={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Save and generate' }));
    await waitFor(() => expect(save).toHaveBeenCalledOnce());
    expect(close).not.toHaveBeenCalled();
  });
});

it('submits an automatic protocol recipe without a saved inbound', async () => {
  const onSave = vi.fn().mockResolvedValue(true);
  render(
    <SubscriptionEditor
      value={{ ...newSubscription(), name: 'Automatic' }}
      relays={[]}
      domains={['www.example.com']}
      onSave={onSave}
      onClose={vi.fn()}
      onTemplate={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Save and generate' }));
  await waitFor(() =>
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        entries: [
          expect.objectContaining({
            mode: 'generate',
            templateId: 0,
            nodeId: 0,
            config: expect.objectContaining({ protocol: 'vless' }),
          }),
        ],
      }),
      true,
    ),
  );
});

it('keeps the editor open when destination preflight fails', async () => {
  vi.spyOn(HttpUtil, 'post').mockResolvedValue({
    success: true,
    msg: '',
    obj: {
      ready: false,
      entries: [{ index: 1, ready: false, message: 'Certificate is missing on the selected node' }],
    },
  });
  const save = vi.fn();
  const close = vi.fn();
  render(
    <SubscriptionEditor
      value={{ ...newSubscription(), name: 'Preflight' }}
      relays={[]}
      domains={[]}
      onSave={save}
      onClose={close}
      onTemplate={vi.fn()}
    />,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Save and generate' }));
  await waitFor(() =>
    expect(screen.getByText('1: Certificate is missing on the selected node')).toBeTruthy(),
  );
  expect(save).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
});
