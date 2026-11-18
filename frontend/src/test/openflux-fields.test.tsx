import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { useState } from 'react';
import { unzipSync, strFromU8 } from 'fflate';
import { OpenFluxFields } from '@/pages/inbounds/form/protocols/additional-vpn';
import OpenFluxOutboundEditor from '@/pages/xray/outbounds/OpenFluxOutboundEditor';
import { FileManager, HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

function Inbound({ max = false, inboundId = 0 }: { max?: boolean; inboundId?: number }) {
  const form = useForm({
    defaultValues: {
      settings: {
        context: 'keep-this-context',
        clients: [{ enable: true, password: 'a'.repeat(32) }],
        transports: max
          ? [
              {
                type: 'oneme',
                token: 'server-private-token',
                uid: '123456',
                url: '',
                priority: 100,
              },
            ]
          : [{ type: 'direct', url: '', priority: 100 }],
      },
    },
  });
  const values = useWatch({ control: form.control });
  return (
    <FormProvider {...form}>
      <OpenFluxFields inboundId={inboundId} />
      <output data-testid="values">{JSON.stringify(values)}</output>
    </FormProvider>
  );
}

describe('OpenFlux channel setup', () => {
  it('adds a valid channel without replacing the secret or context', async () => {
    renderWithProviders(<Inbound />);
    fireEvent.click(screen.getByRole('button', { name: 'Add transport' }));
    const modal = await screen.findByRole('dialog');
    const add = within(modal).getByRole('button', { name: 'Add' });
    expect(add).toHaveProperty('disabled', true);
    fireEvent.change(within(modal).getByRole('textbox', { name: 'Channel URL' }), {
      target: { value: 'https://docs.yandex.ru/edit/d/channel' },
    });
    await waitFor(() => expect(add).toHaveProperty('disabled', false));
    fireEvent.click(add);
    await waitFor(() =>
      expect(screen.getByTestId('values').textContent).toContain('"type":"vyandex"'),
    );
    expect(screen.getByTestId('values').textContent).toContain('keep-this-context');
    expect(screen.getByTestId('values').textContent).toContain('a'.repeat(32));
  });
  it('shows MAX as a usable transport with account fields, not a URL field', async () => {
    renderWithProviders(<Inbound />);
    fireEvent.click(screen.getByRole('button', { name: 'Add transport' }));
    const modal = await screen.findByRole('dialog');
    fireEvent.mouseDown(within(modal).getByRole('combobox', { name: 'Transport' }));
    fireEvent.click(await screen.findByText('Web MAX (WebRTC)'));
    expect(within(modal).queryByRole('textbox', { name: 'Channel URL' })).toBeNull();
    const add = within(modal).getByRole('button', { name: 'Add' });
    expect(add).toHaveProperty('disabled', true);
    fireEvent.change(within(modal).getByLabelText('Server Web MAX token'), {
      target: { value: 'server-token' },
    });
    fireEvent.change(within(modal).getByRole('textbox', { name: 'Server MAX account ID' }), {
      target: { value: '123456' },
    });
    fireEvent.click(add);
    await waitFor(() =>
      expect(screen.getByTestId('values').textContent).toContain('"type":"oneme"'),
    );
  });
  it('keeps installation notices out of the form and advanced settings collapsed', () => {
    vi.mocked(HttpUtil.post).mockClear();
    renderWithProviders(<Inbound />);
    expect(screen.queryByText(/Installation and component updates/)).toBeNull();
    expect(screen.queryByRole('button', { name: 'Install OpenFlux' })).toBeNull();
    expect(screen.queryByText('Maximum session packet (bytes)')).toBeNull();
    fireEvent.click(screen.getByText('Advanced Settings'));
    expect(screen.getByText('Maximum session packet (bytes)')).toBeTruthy();
    expect(document.querySelectorAll('.ant-form-item-tooltip').length).toBeGreaterThan(2);
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });
  it('downloads a MAX client archive without the server account token or a network request', async () => {
    const download = vi.spyOn(FileManager, 'downloadTextFile').mockImplementation(() => {});
    vi.mocked(HttpUtil.post).mockClear();
    renderWithProviders(<Inbound max />);
    fireEvent.click(screen.getByText('Connect a client through MAX'));
    const button = screen.getByRole('button', { name: 'Download MAX client settings' });
    expect(button).toHaveProperty('disabled', true);
    fireEvent.change(screen.getByLabelText('Client Web MAX token'), {
      target: { value: 'server-private-token' },
    });
    expect(button).toHaveProperty('disabled', true);
    fireEvent.change(screen.getByLabelText('Client Web MAX token'), {
      target: { value: 'client-private-token' },
    });
    fireEvent.click(button);
    expect(download).toHaveBeenCalledOnce();
    const files = unzipSync(download.mock.calls[0][0] as Uint8Array);
    const text = Object.values(files)
      .map((bytes) => strFromU8(bytes))
      .join('\n');
    expect(text).toContain('client-private-token');
    expect(text).not.toContain('server-private-token');
    expect(text).toContain('UID = 123456');
    expect(HttpUtil.post).not.toHaveBeenCalled();
    expect(screen.getByTestId('values').textContent).not.toContain('client-private-token');
    download.mockRestore();
  });
  it('checks provider TLS without sending a document link or account credentials', async () => {
    vi.mocked(HttpUtil.post).mockClear();
    vi.mocked(HttpUtil.post).mockResolvedValueOnce(
      new Msg(true, '', { host: 'docs.yandex.ru', reachable: true, scope: 'provider-tls' }),
    );
    renderWithProviders(<Inbound />);
    fireEvent.click(screen.getByRole('button', { name: 'Add transport' }));
    const modal = await screen.findByRole('dialog');
    fireEvent.change(within(modal).getByRole('textbox', { name: 'Channel URL' }), {
      target: { value: 'https://disk.yandex.ru/i/shared-document' },
    });
    fireEvent.click(within(modal).getByRole('button', { name: 'Add' }));
    await waitFor(() =>
      expect(screen.getByTestId('values').textContent).toContain('"type":"vyandex"'),
    );
    fireEvent.click(screen.getAllByRole('button', { name: 'Check service reachability' })[1]);
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
        '/panel/api/server/openflux/check-transport',
        { type: 'vyandex' },
        expect.any(Object),
      ),
    );
    await waitFor(() => {
      expect(document.querySelector('.ant-alert-description')?.textContent).toContain(
        'TLS connection',
      );
    });
    const request = vi.mocked(HttpUtil.post).mock.calls.at(-1);
    expect(request?.[0]).toBe('/panel/api/server/openflux/check-transport');
    expect(request?.[1]).toEqual({ type: 'vyandex' });
    expect(JSON.stringify(request)).not.toContain('shared-document');
    expect(screen.getByText(/actual OpenFlux tunnel are NOT yet verified/)).toBeTruthy();
  });
  it('never sends a MAX session token during the provider network check', async () => {
    vi.mocked(HttpUtil.post).mockClear();
    vi.mocked(HttpUtil.post).mockResolvedValueOnce(
      new Msg(true, '', { host: 'ws-api.oneme.ru', reachable: true, scope: 'provider-tls' }),
    );
    renderWithProviders(<Inbound max />);
    fireEvent.click(screen.getByRole('button', { name: 'Check service reachability' }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
        '/panel/api/server/openflux/check-transport',
        { type: 'oneme' },
        expect.any(Object),
      ),
    );
    await waitFor(() => {
      expect(document.querySelector('.ant-alert-description')?.textContent).toContain(
        'TLS connection',
      );
    });
    const request = vi.mocked(HttpUtil.post).mock.calls.at(-1);
    expect(request?.[1]).toEqual({ type: 'oneme' });
    expect(JSON.stringify(request)).not.toContain('server-private-token');
  });
  it('runs a real transport-specific diagnostic with saved inbound identity only', async () => {
    vi.mocked(HttpUtil.post).mockClear();
    vi.mocked(HttpUtil.post).mockResolvedValueOnce(
      new Msg(true, '', { runId: 'diagnostic-17', state: 'running' }),
    );
    vi.mocked(HttpUtil.get).mockResolvedValueOnce(
      new Msg(true, '', {
        runId: 'diagnostic-17',
        state: 'success',
        result: {
          type: 'direct',
          scope: 'local-openflux-e2e',
          passed: true,
          steps: [
            { stage: 'configuration', status: 'passed', detail: 'Saved inbound is valid' },
            { stage: 'provider', status: 'skipped', detail: 'Local direct test' },
            { stage: 'session', status: 'passed', detail: 'SOCKS CONNECT completed' },
            { stage: 'payload', status: 'passed', detail: '48 bytes round-trip', elapsedMs: 42 },
          ],
        },
      }),
    );
    renderWithProviders(<Inbound inboundId={17} />);
    fireEvent.click(screen.getByRole('button', { name: 'Full connection diagnostic' }));
    const confirm = await screen.findByRole('dialog');
    fireEvent.click(within(confirm).getByRole('button', { name: 'Run real test' }));
    await waitFor(() =>
      expect(vi.mocked(HttpUtil.post)).toHaveBeenCalledWith(
        '/panel/api/server/openflux/diagnose-transport',
        { inboundId: 17, type: 'direct', confirmInterruption: true },
        expect.any(Object),
      ),
    );
    expect(vi.mocked(HttpUtil.get)).toHaveBeenCalledWith(
      '/panel/api/server/openflux/diagnose-status',
      { runId: 'diagnostic-17' },
      expect.any(Object),
    );
    expect(await screen.findByText(/48 bytes round-trip/)).toBeTruthy();
    expect(screen.getByText(/Verified: encrypted OpenFlux payload received/)).toBeTruthy();
  });
  it('skips a live MAX test without a separate client account token', async () => {
    vi.mocked(HttpUtil.post).mockClear();
    renderWithProviders(<Inbound max inboundId={28} />);
    fireEvent.click(screen.getByRole('button', { name: 'Full connection diagnostic' }));
    const confirm = await screen.findByRole('dialog');
    fireEvent.click(within(confirm).getByRole('button', { name: 'Run real test' }));
    expect(await screen.findByText(/Skipped: enter a valid Web MAX token/)).toBeTruthy();
    expect(vi.mocked(HttpUtil.post)).not.toHaveBeenCalled();
  });
  it('imports client settings while preserving the generated channel values', async () => {
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
            update={(patch) => setSettings((s) => ({ ...s, ...patch }))}
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
