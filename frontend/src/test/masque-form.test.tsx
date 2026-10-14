import { useForm, FormProvider, useWatch } from 'react-hook-form';
import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import MasqueFields from '@/pages/inbounds/form/protocols/masque';
import { MasqueInboundSettingsSchema } from '@/schemas/protocols/inbound/masque';
import { renderWithProviders } from './test-utils';

function Form() {
  const methods = useForm({
    defaultValues: {
      settings: MasqueInboundSettingsSchema.parse({
        clients: [{ email: 'alice', password: 'secret' }],
        advanced: { initial_packet_size: 1200, stream_receive_window: '64 MB' },
        tls: { certificatePath: '/cert.pem', keyPath: '/key.pem', serverName: 'vpn.example.com' },
      }),
    },
  });
  const settings = useWatch({ control: methods.control, name: 'settings' });
  return (
    <FormProvider {...methods}>
      <MasqueFields />
      <output data-testid="settings">{JSON.stringify(settings)}</output>
    </FormProvider>
  );
}

describe('MASQUE automatic settings', () => {
  it('switches transports and resets tunnel defaults while keeping users and TLS', () => {
    renderWithProviders(<Form />);
    fireEvent.click(screen.getByRole('button', { name: 'TCP / HTTP/2 compatibility' }));
    let settings = JSON.parse(screen.getByTestId('settings').textContent || '{}');
    expect(settings.version).toEqual([2, 1]);
    expect(settings.clients[0].password).toBe('secret');
    fireEvent.click(screen.getByRole('button', { name: 'Automatic settings' }));
    settings = JSON.parse(screen.getByTestId('settings').textContent || '{}');
    expect(settings.version).toEqual([3, 2, 1]);
    expect(settings.tls.certificatePath).toBe('/cert.pem');
    expect(settings.tls.serverName).toBe('vpn.example.com');
  });
  it('clears the certificate pair together when automatic certificate selection is requested', () => {
    renderWithProviders(<Form />);
    fireEvent.click(screen.getByRole('button', { name: 'Select certificate automatically' }));
    const settings = JSON.parse(screen.getByTestId('settings').textContent || '{}');
    expect(settings.tls.certificatePath).toBe('');
    expect(settings.tls.keyPath).toBe('');
    expect(settings.clients[0].password).toBe('secret');
  });
  it('keeps QUIC options while disabling their fields in TCP mode', () => {
    renderWithProviders(<Form />);
    fireEvent.click(screen.getByRole('button', { name: 'TCP / HTTP/2 compatibility' }));
    fireEvent.click(screen.getByText('Advanced tunnel settings'));
    const packetSize = screen.getByRole('spinbutton', { name: 'initial_packet_size' });
    expect((packetSize as HTMLInputElement).disabled).toBe(true);
    const settings = JSON.parse(screen.getByTestId('settings').textContent || '{}');
    expect(settings.advanced.initial_packet_size).toBe(1200);
    fireEvent.click(screen.getByRole('button', { name: 'QUIC / HTTP/3 only' }));
    expect((packetSize as HTMLInputElement).disabled).toBe(false);
    expect(
      JSON.parse(screen.getByTestId('settings').textContent || '{}').advanced.initial_packet_size,
    ).toBe(1200);
  });
});
