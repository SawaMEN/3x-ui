import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { Form } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';

import { HttpUtil, Msg } from '@/utils';
import { useSecurityActions } from '@/pages/inbounds/form/useSecurityActions';
import type { InboundFormValues } from '@/schemas/forms/inbound-form';
import type { RealityScanResult, SNIJobSnapshot } from '@/generated/types';
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
function doneScan(rows: RealityScanResult[]): SNIJobSnapshot {
  return {
    id: 'test-scan',
    state: 'done',
    phase: 'done',
    source: 'local',
    createdAt: '',
    total: rows.length,
    completed: rows.length,
    verified: 0,
    verifyTotal: 0,
    results: rows,
  };
}
beforeEach(() => {
  vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', []));
});
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

function Harness({ children, nodeId }: { children: ReactNode; nodeId?: number }) {
  const methods = useForm({
    defaultValues: {
      nodeId,
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
      const post = vi
        .spyOn(HttpUtil, 'post')
        .mockResolvedValue(new Msg(true, '', doneScan([candidate])));
      // Mtproto's outbound selector is unrelated to the scanner.
      vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', []));
      renderWithProviders(<Harness>{fields}</Harness>);
      expect(post.mock.calls.some(([url]) => url === '/panel/api/server/sni/jobs')).toBe(false);
      await pick();
      expect(post).toHaveBeenCalledWith(
        '/panel/api/server/sni/jobs',
        {
          mode: 'candidates',
          targets: '',
          purpose: path.includes('handshake')
            ? 'shadowtls'
            : path.includes('fakeTls')
              ? 'fakeTls'
              : 'tls',
          nodeId: 0,
        },
        {
          silent: true,
          timeout: 15_000,
          signal: expect.any(AbortSignal),
          headers: { 'Content-Type': 'application/json' },
        },
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
      new Msg(true, '', doneScan([{ ...candidate, serverNames: [] }])),
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
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockResolvedValue(new Msg(true, '', doneScan([candidate])));
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
        '/panel/api/server/sni/jobs',
        { mode: 'candidates', targets: 'example.org, 203.0.113.0/30', purpose: 'tls', nodeId: 0 },
        {
          silent: true,
          timeout: 15_000,
          signal: expect.any(AbortSignal),
          headers: { 'Content-Type': 'application/json' },
        },
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
      .mockResolvedValueOnce(new Msg(true, '', doneScan([candidate])));
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

it('switches to nearby mode, passes a server reference and ignores stale results', async () => {
  let finishInitial!: (rows: RealityScanResult[]) => void;
  const scan = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<RealityScanResult[]>((resolve) => {
          finishInitial = resolve;
        }),
    )
    .mockResolvedValue([]);
  renderWithProviders(
    <RealityTargetScannerModal open onClose={noop} onPick={noop} scanRealityCandidates={scan} />,
  );
  await waitFor(() => expect(scan).toHaveBeenCalledTimes(1));
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Search mode' }));
  fireEvent.click(await screen.findByText('Find SNI near the server'));
  expect(scan.mock.calls[0][2].aborted).toBe(true);
  const input = screen.getByPlaceholderText('Server IP/domain (optional)');
  fireEvent.change(input, { target: { value: '8.8.8.8' } });
  fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', charCode: 13 });
  await waitFor(() =>
    expect(scan).toHaveBeenLastCalledWith(
      '8.8.8.8',
      'nearby',
      expect.any(AbortSignal),
      expect.any(Function),
      undefined,
    ),
  );
  await act(async () => finishInitial([candidate]));
  expect(screen.queryByText('edge.example:8443')).toBeNull();
});

it('sends nearby mode through the shared SNI API with automatic server discovery', async () => {
  const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', doneScan([])));
  renderWithProviders(<Harness>{tls}</Harness>);
  fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.mouseDown(within(dialog).getByRole('combobox', { name: 'Search mode' }));
  fireEvent.click(await screen.findByText('Find SNI near the server'));
  const input = within(dialog).getByPlaceholderText('Server IP/domain (optional)');
  fireEvent.keyDown(input, { key: 'Enter', code: 'Enter', charCode: 13 });
  await waitFor(() =>
    expect(post).toHaveBeenLastCalledWith(
      '/panel/api/server/sni/jobs',
      { targets: '', mode: 'nearby', purpose: 'tls', nodeId: 0 },
      {
        silent: true,
        timeout: 15_000,
        signal: expect.any(AbortSignal),
        headers: { 'Content-Type': 'application/json' },
      },
    ),
  );
});

it('aborts an active scan when the scanner closes', async () => {
  const scan = vi.fn().mockResolvedValue([]);
  const modal = (open: boolean) => (
    <RealityTargetScannerModal
      open={open}
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={scan}
    />
  );
  const { rerender } = renderWithProviders(modal(true));
  await waitFor(() => expect(scan).toHaveBeenCalledTimes(1));
  const signal = scan.mock.calls[0][2] as AbortSignal;
  expect(signal.aborted).toBe(false);
  rerender(modal(false));
  expect(signal.aborted).toBe(true);
});

it.each([
  ['incompatible REALITY endpoint', { ...candidate, feasible: false }, false],
  ['insufficient ML-DSA-65 chain', { ...candidate, feasible: true, certChainBytes: 3000 }, true],
])('prevents selecting an %s', async (_, row, mldsa65Enabled) => {
  const pick = vi.fn();
  renderWithProviders(
    <RealityTargetScannerModal
      open
      onClose={noop}
      onPick={pick}
      scanRealityCandidates={async () => [row]}
      mldsa65Enabled={mldsa65Enabled}
    />,
  );
  const button = await screen.findByRole('button', { name: 'Use' });
  expect((button as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(button);
  expect(pick).not.toHaveBeenCalled();
});

function RealitySelectionHarness() {
  const methods = useForm<InboundFormValues>({
    defaultValues: {
      streamSettings: {
        security: 'reality',
        realitySettings: { target: 'old.example:443', serverNames: ['old.example'], xver: 2 },
      },
    } as never,
  });
  const actions = useSecurityActions({
    methods,
    setSaving: noop,
    setScanResult: noop,
    setScanning: noop,
    messageApi: {} as never,
    modal: {} as never,
    nodeId: null,
  });
  const current = useWatch({ control: methods.control });
  return (
    <>
      <button
        onClick={() =>
          actions.applyRealityScanResult(
            { ...candidate, target: '8.8.8.7:8443', serverNames: ['edge.example'] },
            true,
          )
        }
      >
        Select nearby target
      </button>
      <output data-testid="reality-values">{JSON.stringify(current)}</output>
    </>
  );
}

it('applies the scanned REALITY IP and verified SNI without an old PROXY protocol setting', () => {
  renderWithProviders(<RealitySelectionHarness />);
  fireEvent.click(screen.getByRole('button', { name: 'Select nearby target' }));
  const current = JSON.parse(screen.getByTestId('reality-values').textContent || '{}');
  expect(current.streamSettings.realitySettings).toMatchObject({
    target: '8.8.8.7:8443',
    serverNames: ['edge.example'],
    xver: 0,
  });
});

it('shows an API scan error instead of treating it as an empty successful scan', async () => {
  vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(false, 'Scanner unavailable'));
  renderWithProviders(<Harness>{tls}</Harness>);
  fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
  const dialog = await screen.findByRole('dialog');
  await waitFor(() => expect(dialog.querySelector('.ant-alert-error')).toBeTruthy());
});

it('displays a successful sub-millisecond TLS connection as 0 ms', async () => {
  renderWithProviders(
    <RealityTargetScannerModal
      open
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={async () => [{ ...candidate, latencyMs: 0 }]}
    />,
  );
  expect(await screen.findByText('0 ms')).toBeTruthy();
});

it('shows intermediate scan results and lets the user stop active work', async () => {
  const scan = vi.fn((_targets, _mode, _signal, progress) => {
    progress({
      ...doneScan([candidate]),
      state: 'running',
      phase: 'discovering',
      total: 256,
      completed: 1,
    });
    return new Promise<RealityScanResult[]>(() => {});
  });
  renderWithProviders(
    <RealityTargetScannerModal
      open
      purpose="tls"
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={scan}
    />,
  );
  expect(await screen.findByText('edge.example:8443')).toBeTruthy();
  expect(screen.getByText(/1\/256/)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Stop scan' }));
  expect(scan.mock.calls[0][2].aborted).toBe(true);
  expect(screen.getByText(/Stopped/)).toBeTruthy();
  expect(screen.getByText('edge.example:8443')).toBeTruthy();
});

it('recommends a stable result ahead of a faster unreliable endpoint', async () => {
  const unstable = {
    ...candidate,
    target: 'unstable.example:443',
    host: 'unstable.example',
    feasible: true,
    latencyMs: 1,
    attempts: 5,
    successes: 3,
    medianMs: 1,
  };
  const stable = {
    ...candidate,
    feasible: true,
    latencyMs: 10,
    attempts: 5,
    successes: 5,
    medianMs: 10,
    jitterMs: 1,
    http2OK: true,
    dnsMatchesIP: false,
  };
  renderWithProviders(
    <RealityTargetScannerModal
      open
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={async () => [unstable, stable]}
    />,
  );
  expect(await screen.findByText('Recommended')).toBeTruthy();
  const buttons = screen.getAllByRole('button', { name: 'Use' }) as HTMLButtonElement[];
  expect(buttons.map((button) => button.disabled)).toEqual([false, true]);
  expect(screen.getByText('HTTP/2 OK')).toBeTruthy();
});

it('loads saved measurements and rechecks the saved IP/SNI pair', async () => {
  const stored = {
    ...doneScan([candidate]),
    createdAt: '2026-11-01T10:00:00Z',
    finishedAt: '2026-11-01T10:01:00Z',
  };
  vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', [stored]));
  const scan = vi.fn().mockResolvedValue([]);
  renderWithProviders(
    <RealityTargetScannerModal
      open
      enableHistory
      nodeId={7}
      purpose="tls"
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={scan}
    />,
  );
  fireEvent.click(screen.getByText('Saved results'));
  await waitFor(() =>
    expect(
      (screen.getByRole('combobox', { name: 'Saved results' }) as HTMLInputElement).disabled,
    ).toBe(false),
  );
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Saved results' }));
  fireEvent.click(await screen.findByText(/1 SNI/));
  expect(screen.getByText('edge.example:8443')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Recheck saved targets' }));
  await waitFor(() =>
    expect(scan).toHaveBeenLastCalledWith(
      undefined,
      'recheck',
      expect.any(AbortSignal),
      expect.any(Function),
      [candidate],
    ),
  );
  expect(HttpUtil.get).toHaveBeenCalledWith(
    '/panel/api/server/sni/history?nodeId=7',
    undefined,
    expect.anything(),
  );
});

it('cancels the old scan when the execution node changes', async () => {
  const scan = vi.fn().mockResolvedValue([]);
  const modal = (nodeId: number) => (
    <RealityTargetScannerModal
      open
      nodeId={nodeId}
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={scan}
    />
  );
  const { rerender } = renderWithProviders(modal(7));
  await waitFor(() => expect(scan).toHaveBeenCalledTimes(1));
  const signal = scan.mock.calls[0][2] as AbortSignal;
  rerender(modal(8));
  await waitFor(() => expect(scan).toHaveBeenCalledTimes(2));
  expect(signal.aborted).toBe(true);
  expect(screen.getByText('Scanning from selected node #8')).toBeTruthy();
});

it('uses the inbound’s selected node for protocol-form SNI searches', async () => {
  const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', doneScan([])));
  renderWithProviders(
    <Harness nodeId={7}>
      <NaiveFields />
    </Harness>,
  );
  fireEvent.click(screen.getByRole('button', { name: 'Find SNI' }));
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith(
      '/panel/api/server/sni/jobs',
      expect.objectContaining({ nodeId: 7, purpose: 'tls' }),
      expect.anything(),
    ),
  );
  expect(screen.getByText('Scanning from selected node #7')).toBeTruthy();
});

it('ranks an ML-DSA-65-compatible target ahead of a faster undersized certificate', async () => {
  const small = {
    ...candidate,
    target: 'small.example:443',
    host: 'small.example',
    feasible: true,
    certChainBytes: 3000,
    attempts: 5,
    successes: 5,
    medianMs: 1,
  };
  const compatible = {
    ...candidate,
    feasible: true,
    certChainBytes: 4000,
    attempts: 5,
    successes: 5,
    medianMs: 10,
  };
  renderWithProviders(
    <RealityTargetScannerModal
      open
      mldsa65Enabled
      onClose={noop}
      onPick={noop}
      scanRealityCandidates={async () => [small, compatible]}
    />,
  );
  expect(await screen.findByText('Recommended')).toBeTruthy();
  expect(
    (screen.getAllByRole('button', { name: 'Use' }) as HTMLButtonElement[]).map(
      (button) => button.disabled,
    ),
  ).toEqual([false, true]);
});
