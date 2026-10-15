import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SingBoxOutboundModal from '@/pages/singbox/SingBoxOutboundModal';
import fields from '@/pages/singbox/native-outbound-fields.json';
import { nativeOutboundTypes, nativeEndpointTypes } from '@/pages/singbox/native-import';
vi.mock('@/components/form/JsonEditor', () => ({
  default: ({ value }: { value: string }) => (
    <textarea aria-label="JSON editor" readOnly value={value} />
  ),
}));
describe('protocol-specific outbound options', () => {
  it('covers every selectable native outbound and endpoint', () => {
    for (const type of [...nativeOutboundTypes, ...nativeEndpointTypes])
      expect(fields.protocols).toHaveProperty(type);
  });
  it('saves Naive concurrency while preserving imported settings', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{
          type: 'naive',
          tag: 'naive',
          server: 'example.com',
          server_port: 443,
          username: 'user',
          password: 'secret',
          tls: { enabled: true },
          extra_headers: { Test: 'keep' },
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.change(screen.getByLabelText('Одновременные соединения'), { target: { value: '8' } });
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить изменения' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ insecure_concurrency: 8, extra_headers: { Test: 'keep' } }),
      [],
    );
  });
  it('edits headers without JSON and rejects duplicate names', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{
          type: 'naive',
          tag: 'naive',
          server: 'example.com',
          server_port: 443,
          username: 'user',
          password: 'secret',
          tls: { enabled: true },
          extra_headers: { Test: 'old' },
          quic: true,
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByText('Дополнительные параметры naive'));
    fireEvent.change(screen.getByLabelText('Дополнительные HTTP-заголовки: значение 1'), {
      target: { value: 'new' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Добавить заголовок' }));
    fireEvent.change(screen.getByLabelText('Дополнительные HTTP-заголовки: имя 2'), {
      target: { value: 'test' },
    });
    const save = screen.getByRole('button', { name: 'Сохранить изменения' }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.click(screen.getByLabelText('Дополнительные HTTP-заголовки: удалить 2'));
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ extra_headers: { Test: ['new'] }, quic: true }),
      [],
    );
  });
  it('edits a WireGuard peer without changing keys or unknown imported options', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{
          type: 'wireguard',
          tag: 'wg',
          private_key: 'private',
          address: ['10.0.0.2/32'],
          peers: [{ address: 'old.test', port: 51820, public_key: 'public', custom: 'keep' }],
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.change(screen.getByLabelText('Узлы 1: Адрес'), { target: { value: 'new.test' } });
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить изменения' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        private_key: 'private',
        peers: [{ address: 'new.test', port: 51820, public_key: 'public', custom: 'keep' }],
      }),
      [],
    );
  });
  it('edits Mieru port bindings and removes a conflicting legacy port', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{
          type: 'mieru',
          tag: 'mieru',
          server: 'example.com',
          server_port: 443,
          username: 'user',
          password: 'secret',
          portBindings: [{ protocol: 'TCP', port: 443 }],
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.change(screen.getByLabelText('Порты Mieru 1: Порт'), { target: { value: '8443' } });
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить изменения' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ portBindings: [{ protocol: 'TCP', port: 8443 }] }),
      [],
    );
    expect(onSave.mock.calls[0][0]).not.toHaveProperty('server_port');
  });
  it('fills required defaults and SNI from the server with one click', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{
          type: 'naive',
          tag: 'naive',
          server: 'example.com',
          username: 'user',
          password: 'secret',
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Настроить автоматически' }));
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить изменения' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        server_port: 443,
        tls: { enabled: true, server_name: 'example.com' },
        username: 'user',
        password: 'secret',
      }),
      [],
    );
  });
  it('creates missing credentials from the normal connection screen', () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{ type: 'naive', tag: 'naive', server: 'example.com' }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Настроить автоматически' }));
    fireEvent.click(screen.getByRole('button', { name: 'Сохранить изменения' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        username: expect.stringMatching(/^vpn_/),
        password: expect.stringMatching(/^[0-9a-f]{48}$/),
      }),
      [],
    );
  });
  it('exposes group-specific settings without adding dialer fields to block', () => {
    const { unmount } = render(
      <SingBoxOutboundModal
        open
        value={{ type: 'balancer', tag: 'balance', outbounds: ['proxy'] }}
        existingTags={['proxy']}
        onCancel={() => {}}
        onSave={() => {}}
      />,
    );
    expect(screen.getByLabelText('Стратегия балансировки')).toBeTruthy();
    unmount();
    render(
      <SingBoxOutboundModal
        open
        value={{ type: 'block', tag: 'block' }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={() => {}}
      />,
    );
    expect(screen.queryByLabelText('Тайм-аут подключения')).toBeNull();
  });
});
