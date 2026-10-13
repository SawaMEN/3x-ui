import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SingBoxOutboundModal from '@/pages/singbox/SingBoxOutboundModal';
import { importNativeConnections } from '@/pages/singbox/native-import';
vi.mock('@/components/form/JsonEditor', () => ({
  default: ({ value, onChange }: { value: string; onChange: (v: string) => void }) => (
    <textarea aria-label="JSON editor" value={value} onChange={(e) => onChange(e.target.value)} />
  ),
}));
vi.mock('@/pages/singbox/native-import', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/pages/singbox/native-import')>()),
  importNativeConnections: vi.fn(),
}));
describe('native outbound modal', () => {
  it('imports a link inside JSON and saves the same native data with a unique tag', async () => {
    const onSave = vi.fn();
    vi.mocked(importNativeConnections).mockResolvedValue([
      {
        type: 'anytls',
        tag: 'proxy',
        server: 'host.test',
        server_port: 443,
        password: 'secret',
        tls: { enabled: true },
        idle_session_timeout: '30s',
      },
    ]);
    render(
      <SingBoxOutboundModal
        open
        value={null}
        existingTags={['proxy']}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole('tab', { name: /JSON/ }));
    fireEvent.change(screen.getByLabelText('Ссылка подключения'), {
      target: { value: 'anytls://secret@host.test' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Импортировать/ }));
    await waitFor(() =>
      expect(
        screen.getByLabelText('JSON editor').getAttribute('value') ||
          (screen.getByLabelText('JSON editor') as HTMLTextAreaElement).value,
      ).toContain('idle_session_timeout'),
    );
    fireEvent.click(screen.getByRole('tab', { name: /Подключение/ }));
    expect(screen.getByDisplayValue('host.test')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Добавить подключение' }));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'anytls', tag: 'proxy-2', idle_session_timeout: '30s' }),
      [],
    );
  });
  it('does not save the last valid draft while JSON has a syntax error', async () => {
    const onSave = vi.fn();
    render(
      <SingBoxOutboundModal
        open
        value={{ type: 'direct', tag: 'direct' }}
        existingTags={['direct']}
        onCancel={() => {}}
        onSave={onSave}
      />,
    );
    fireEvent.click(screen.getByRole('tab', { name: /JSON/ }));
    fireEvent.change(screen.getByLabelText('JSON editor'), { target: { value: '{"type":' } });
    const save = screen.getByRole('button', { name: 'Сохранить изменения' }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.click(save);
    expect(onSave).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText('JSON editor'), {
      target: { value: '{"type":"direct","tag":"direct","tcp_fast_open":true}' },
    });
    fireEvent.click(save);
    expect(onSave).toHaveBeenCalledWith({ type: 'direct', tag: 'direct', tcp_fast_open: true }, []);
  });
});
