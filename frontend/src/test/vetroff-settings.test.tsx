import { fireEvent, screen } from '@testing-library/react';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { describe, expect, it } from 'vitest';
import { VetroffFields } from '@/pages/inbounds/form/protocols/vetroff';
import { VetroffOptionsSchema } from '@/schemas/protocols/inbound/vetroff';
import { renderWithProviders } from './test-utils';

function VetroffTestForm() {
  const form = useForm({
    defaultValues: {
      settings: {
        hostname: 'vpn.example.com',
        certificate: '/cert.pem',
        privateKey: '/key.pem',
        vetroff: VetroffOptionsSchema.parse({
          autoTLS: false,
          transport: 'wss',
          tlsBackend: 'nss',
          http3: false,
        }),
        clients: [{ email: 'alice', password: 'retained-client-password' }],
      },
    },
  });
  const settings = useWatch({ control: form.control, name: 'settings' });
  return (
    <FormProvider {...form}>
      <VetroffFields />
      <output data-testid="vetroff-settings">{JSON.stringify(settings)}</output>
    </FormProvider>
  );
}
function values() {
  return JSON.parse(screen.getByTestId('vetroff-settings').textContent || '{}');
}
describe('Vetroff automatic settings', () => {
  it('builds HTTP/3 and XHTTP presets without changing users, credentials or hostname', () => {
    renderWithProviders(<VetroffTestForm />);
    fireEvent.click(screen.getByRole('button', { name: 'HTTP/3' }));
    expect(values()).toMatchObject({
      hostname: 'vpn.example.com',
      certificate: '/cert.pem',
      clients: [{ password: 'retained-client-password' }],
      vetroff: {
        transport: 'h3-connect',
        http3: true,
        tlsBackend: 'auto',
        autoTLS: true,
        maxConnections: 512,
      },
    });
    fireEvent.click(screen.getByRole('button', { name: 'XHTTP' }));
    expect(values().vetroff).toMatchObject({
      transport: 'xhttp-stream-up',
      tlsBackend: 'auto',
      autoPin: true,
    });
  });
  it('shows custom certificate paths only in manual TLS mode', () => {
    renderWithProviders(<VetroffTestForm />);
    expect(screen.getByPlaceholderText('/etc/ssl/vetroff/fullchain.pem')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'HTTP/3' }));
    expect(screen.queryByPlaceholderText('/etc/ssl/vetroff/fullchain.pem')).toBeNull();
  });
});
