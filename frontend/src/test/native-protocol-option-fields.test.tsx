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
      expect(fields).toHaveProperty(type);
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
  it('blocks saving invalid protocol JSON and saves corrected headers without losing other settings', () => {
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
          quic: true,
        }}
        existingTags={[]}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByText('Дополнительные параметры'));
    fireEvent.change(screen.getByLabelText('Дополнительные HTTP-заголовки'), {
      target: { value: '{' },
    });
    const save = screen.getByRole('button', { name: 'Сохранить изменения' }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.click(save);
    expect(onSave).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('Дополнительные HTTP-заголовки'), {
      target: { value: '{"Test":"new"}' },
    });
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ extra_headers: { Test: 'new' }, quic: true }),
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
    fireEvent.click(screen.getByText('Дополнительные параметры'));
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
    fireEvent.click(screen.getByText('Дополнительные параметры'));
    expect(screen.queryByLabelText('Тайм-аут подключения')).toBeNull();
  });
});
