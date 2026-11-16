import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { useState } from 'react';
import { OpenFluxFields } from '@/pages/inbounds/form/protocols/additional-vpn';
import OpenFluxOutboundEditor from '@/pages/xray/outbounds/OpenFluxOutboundEditor';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

function Inbound() {
  const form = useForm({
    defaultValues: {
      nodeId: 7,
      settings: {
        context: 'keep-this-context',
        transports: [{ type: 'direct', url: '', priority: 100 }],
      },
    },
  });
  const values = useWatch({ control: form.control });
  return (
    <FormProvider {...form}>
      <OpenFluxFields />
      <output data-testid="values">{JSON.stringify(values)}</output>
    </FormProvider>
  );
}

describe('OpenFlux channel setup', () => {
  it('preserves encryption context when selecting a preset', () => {
    renderWithProviders(<Inbound />);
    fireEvent.click(screen.getByRole('button', { name: 'Direct channel' }));
    expect(screen.getByTestId('values').textContent).toContain('keep-this-context');
  });
  it('collects a valid backup URL before adding a channel', async () => {
    renderWithProviders(<Inbound />);
    fireEvent.click(screen.getByRole('button', { name: 'Direct + backup channel' }));
    const modal = await screen.findByRole('dialog');
    const ok = within(modal).getByRole('button', { name: /^OK$/ });
    expect(ok).toHaveProperty('disabled', true);
    fireEvent.change(within(modal).getByRole('textbox', { name: 'Channel URL' }), {
      target: { value: 'https://docs.yandex.ru/edit/d/channel' },
    });
    await waitFor(() => expect(ok).toHaveProperty('disabled', false));
    fireEvent.click(ok);
    await waitFor(() =>
      expect(screen.getByTestId('values').textContent).toContain('"type":"vyandex"'),
    );
    expect(screen.getByTestId('values').textContent).toContain('keep-this-context');
  });
  it('shows question-mark help for channel selection and advanced settings', async () => {
    renderWithProviders(<Inbound />);
    expect(document.querySelectorAll('.ant-form-item-tooltip').length).toBeGreaterThanOrEqual(2);
    fireEvent.click(screen.getByText('Advanced Settings'));
    expect(await screen.findByText('Maximum session packet (bytes)')).toBeTruthy();
  });
  it('offers component guidance without installation controls in the inbound form', () => {
    vi.mocked(HttpUtil.post).mockClear();
    renderWithProviders(<Inbound />);
    expect(screen.queryByRole('button', { name: 'Install OpenFlux' })).toBeNull();
    expect(screen.getByText(/Installation and component updates/)).toBeTruthy();
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });
  it('imports all client connection settings without editing JSON', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValueOnce(
      new Msg(true, '', {
        protocol: 'openflux',
        settings: {
          secret: 'a'.repeat(32),
          context: 'imported-context',
          transports: [{ type: 'direct', dial: 'vpn.example.org:443', priority: 100 }],
        },
      }),
    );
    function Client() {
      const [settings, setSettings] = useState<Record<string, unknown>>({});
      return (
        <>
          <OpenFluxOutboundEditor
            settings={settings}
            update={(patch) => setSettings((current) => ({ ...current, ...patch }))}
          />
          <output data-testid="client">{JSON.stringify(settings)}</output>
        </>
      );
    }
    renderWithProviders(<Client />);
    fireEvent.change(screen.getByRole('searchbox', { name: 'Import OpenFlux connection' }), {
      target: { value: 'openflux://v1/example' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Import connection' }));
    await waitFor(() =>
      expect(screen.getByTestId('client').textContent).toContain('vpn.example.org:443'),
    );
    expect(screen.getByTestId('client').textContent).toContain('imported-context');
  });
});
