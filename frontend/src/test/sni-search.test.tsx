import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { Form } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { HttpUtil, Msg } from '@/utils';
import type { RealityScanResult } from '@/generated/types';
import { TlsForm } from '@/pages/inbounds/form/security';
import AnyTlsFields from '@/pages/inbounds/form/protocols/anytls';
import NaiveFields from '@/pages/inbounds/form/protocols/naive';
import MasqueFields from '@/pages/inbounds/form/protocols/masque';
import MtprotoFields from '@/pages/inbounds/form/protocols/mtproto';
import TuicFields from '@/pages/inbounds/form/protocols/tuic';
import ShadowTlsFields from '@/pages/inbounds/form/protocols/shadowtls';
import RealityTargetScannerModal from '@/pages/inbounds/form/security/RealityTargetScannerModal';
import { renderWithProviders } from './test-utils';

const candidate: RealityScanResult = {
  target: 'edge.example:8443',
  host: 'edge.example',
  port: 8443,
  ip: '203.0.113.1',
  serverNames: ['other.example', 'edge.example'],
  certValid: true,
  certChainValid: true,
  certSubject: 'edge.example',
  certIssuer: 'Test CA',
  certChainBytes: 3000,
  notAfter: '',
  tls13: true,
  tlsVersion: '1.3',
  h2: false,
  alpn: 'http/1.1',
  x25519: false,
  curveID: 'P256',
  latencyMs: 25,
  feasible: false,
  privateTarget: false,
  reason: 'server does not negotiate HTTP/2 (h2)',
};
const noop = () => {};
const tls = (
  <TlsForm
    saving={false}
    setCertFromPanel={noop}
    clearCertFiles={noop}
    pinFromCert={noop}
    pinFromRemote={noop}
    getNewEchCert={noop}
    clearEchCert={noop}
  />
);

function Harness({ children }: { children: ReactNode }) {
  const methods = useForm({
    defaultValues: {
      settings: {
        tls: { serverName: 'old.example', certificatePath: '/custom/cert', keyPath: '/custom/key' },
        server: { sni: 'old.example', certificate: '/custom/cert', private_key: '/custom/key' },
        handshake: { server: 'old.example', serverPort: 443 },
        shadowtls: { handshake: { server: 'old.example', serverPort: 443 } },
        fakeTlsDomain: 'old.example',
        version: [2],
        webProxy: { enabled: false },
      },
      streamSettings: { tlsSettings: { serverName: 'old.example' } },
    },
  });
  const values = useWatch({ control: methods.control });
  return (
    <FormProvider {...methods}>
      <Form>
        {children}
        <output data-testid="values">{JSON.stringify(values)}</output>
      </Form>
    </FormProvider>
  );
}
function values() {
  return JSON.parse(screen.getByTestId('values').textContent || '{}');
}
async function pick() {
  fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(await within(dialog).findByRole('button', { name: 'Use' }));
}

afterEach(() => vi.restoreAllMocks());

describe('SNI search in protocol forms', () => {
  it.each([
    [
      'shared TLS (VMess/VLESS/Trojan/HTTP/Mixed/Hysteria)',
      tls,
      'streamSettings.tlsSettings.serverName',
    ],
    ['AnyTLS', <AnyTlsFields key="AnyTlsFields" />, 'settings.tls.serverName'],
    ['NaiveProxy', <NaiveFields key="NaiveFields" />, 'settings.tls.serverName'],
    ['MASQUE', <MasqueFields key="MasqueFields" />, 'settings.tls.serverName'],
    ['MTProto FakeTLS', <MtprotoFields key="MtprotoFields" />, 'settings.fakeTlsDomain'],
    ['TUIC', <TuicFields key="TuicFields" />, 'settings.server.sni'],
    ['ShadowTLS standalone', <ShadowTlsFields key="standalone" />, 'settings.handshake.server'],
    [
      'ShadowTLS wrapper',
      <ShadowTlsFields key="wrapper" prefix="shadowtls" />,
      'settings.shadowtls.handshake.server',
    ],
  ])(
    'applies a picked domain to %s without changing custom certificates',
    async (_, fields, path) => {
      const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', [candidate]));
      // Mtproto's outbound selector is unrelated to the scanner.
      vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', []));
      renderWithProviders(<Harness>{fields}</Harness>);
      expect(post.mock.calls.some(([url]) => url === '/panel/api/server/scanRealityTargets')).toBe(
        false,
      );
      await pick();
      expect(post).toHaveBeenCalledWith(
        '/panel/api/server/scanRealityTargets',
        {},
        { silent: true },
      );
      const saved = values();
      expect(path.split('.').reduce((value, key) => value[key], saved)).toBe('edge.example');
      expect(saved.settings.tls.certificatePath).toBe('/custom/cert');
      expect(saved.settings.tls.keyPath).toBe('/custom/key');
      expect(saved.settings.server.certificate).toBe('/custom/cert');
      expect(saved.settings.server.private_key).toBe('/custom/key');
      if (path.includes('handshake')) {
        expect(
          path
            .replace(/server$/, 'serverPort')
            .split('.')
            .reduce((value, key) => value[key], saved),
        ).toBe(8443);
      }
    },
  );

  it('uses the verified hostname when its certificate contains only wildcard SANs', async () => {
    vi.spyOn(HttpUtil, 'post').mockResolvedValue(
      new Msg(true, '', [{ ...candidate, serverNames: [] }]),
    );
    renderWithProviders(
      <Harness>
        <NaiveFields />
      </Harness>,
    );
    await pick();
    expect(values().settings.tls.serverName).toBe('edge.example');
  });

  it('keeps manual typing and forwards custom domain/IP/CIDR searches', async () => {
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', [candidate]));
    renderWithProviders(
      <Harness>
        <NaiveFields key="NaiveFields" />
      </Harness>,
    );
    fireEvent.change(screen.getByLabelText('SNI'), { target: { value: 'typed.example' } });
    expect(values().settings.tls.serverName).toBe('typed.example');
    fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('button', { name: 'Use' });
    await waitFor(() => expect(dialog.querySelector('.ant-btn-loading')).toBeNull());
    const search = within(dialog).getByRole('searchbox');
    fireEvent.change(search, { target: { value: '  example.org, 203.0.113.0/30  ' } });
    fireEvent.click(within(dialog).getByRole('button', { name: /\bScan$/ }));
    await waitFor(() =>
      expect(post).toHaveBeenLastCalledWith(
        '/panel/api/server/scanRealityTargets',
        { targets: 'example.org, 203.0.113.0/30' },
        { silent: true },
      ),
    );
  });

  it.each(['tls', 'shadowtls', 'fakeTls'] as const)(
    'prevents selecting untrusted names in %s mode',
    async (purpose) => {
      renderWithProviders(
        <RealityTargetScannerModal
          open
          purpose={purpose}
          onClose={noop}
          onPick={noop}
          scanRealityCandidates={async () => [{ ...candidate, certValid: false }]}
        />,
      );
      const button = await screen.findByRole('button', { name: 'Use' });
      expect(button.hasAttribute('disabled')).toBe(true);
    },
  );

  it.each([
    ['tls', false],
    ['shadowtls', true],
    ['fakeTls', true],
  ] as const)('TLS 1.2 selection in %s mode is disabled=%s', async (purpose, disabled) => {
    renderWithProviders(
      <RealityTargetScannerModal
        open
        purpose={purpose}
        onClose={noop}
        onPick={noop}
        scanRealityCandidates={async () => [{ ...candidate, tls13: false, tlsVersion: '1.2' }]}
      />,
    );
    expect((await screen.findByRole('button', { name: 'Use' })).hasAttribute('disabled')).toBe(
      disabled,
    );
  });

  it('shows a failed request and allows a retry', async () => {
    vi.spyOn(HttpUtil, 'post')
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(new Msg(true, '', [candidate]));
    renderWithProviders(
      <Harness>
        <NaiveFields key="NaiveFields" />
      </Harness>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
    const dialog = await screen.findByRole('dialog');
    await within(dialog).findByRole('alert');
    await waitFor(() => expect(dialog.querySelector('.ant-btn-loading')).toBeNull());
    fireEvent.click(within(dialog).getByRole('button', { name: /Rescan$/ }));
    expect(
      (await within(dialog).findByRole('button', { name: 'Use' })).hasAttribute('disabled'),
    ).toBe(false);
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('ignores a scan that finishes after the scanner is closed and reopened', async () => {
    let complete!: (rows: RealityScanResult[]) => void;
    const scan = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            complete = resolve;
          }),
      )
      .mockResolvedValueOnce([candidate]);
    const modal = (open: boolean) => (
      <RealityTargetScannerModal
        open={open}
        purpose="tls"
        onClose={noop}
        onPick={noop}
        scanRealityCandidates={scan}
      />
    );
    const { rerender } = render(modal(true));
    await waitFor(() => expect(scan).toHaveBeenCalledTimes(1));
    rerender(modal(false));
    rerender(modal(true));
    await screen.findByText('edge.example:8443');
    await act(async () => complete([{ ...candidate, target: 'stale.example:443' }]));
    expect(screen.queryByText('stale.example:443')).toBeNull();
    expect(screen.getByText('edge.example:8443')).toBeTruthy();
  });
});
