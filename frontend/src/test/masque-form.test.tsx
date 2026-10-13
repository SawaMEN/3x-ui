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
});
