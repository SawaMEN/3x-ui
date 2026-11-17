import { fireEvent, screen, waitFor } from '@testing-library/react';
import { Form } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { expect, it, vi } from 'vitest';

import RealityForm from '@/pages/inbounds/form/security/reality';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useManagedSniDomains', () => ({
  useManagedSniDomains: () => ({
    data: [
      { name: 'saved.example.org', source: 'remote', certFile: '', keyFile: '' },
      { name: 'legacy.example.org', source: 'gateway', certFile: '/cert', keyFile: '/key' },
      {
        name: 'node.example.org',
        source: 'remote',
        nodeId: 7,
        certFile: '',
        keyFile: '',
        realityTarget: 'node.example.org:9443',
        expiresAt: 4102444800,
      },
      {
        name: 'site.example.org',
        source: 'gateway',
        certFile: '/cert',
        keyFile: '/key',
        realityTarget: 'site.example.org:8443',
      },
    ],
  }),
}));

const noop = () => {};

function Harness({ target = '', nodeId = 0 }: { target?: string; nodeId?: number }) {
  const methods = useForm({
    defaultValues: { nodeId, streamSettings: { realitySettings: { target, serverNames: [] } } },
  });
  const values = useWatch({ control: methods.control });
  return (
    <FormProvider {...methods}>
      <Form>
        <RealityForm
          saving={false}
          scanning={false}
          scanResult={null}
          scanRealityTarget={noop}
          scanRealityCandidates={async () => []}
          applyRealityScanResult={noop}
          randomizeShortIds={noop}
          randomizeSpiderX={noop}
          genRealityKeypair={noop}
          clearRealityKeypair={noop}
          genMldsa65={noop}
          clearMldsa65={noop}
        />
        <output data-testid="values">{JSON.stringify(values)}</output>
      </Form>
    </FormProvider>
  );
}

it.each([
  ['site.example.org', 0, '', 'site.example.org:8443'],
  ['site.example.org', 0, 'custom.example.org:9443', 'custom.example.org:9443'],
  ['node.example.org', 7, '', 'node.example.org:9443'],
])(
  'offers %s on node %s and preserves or fills its target',
  async (name, nodeId, target, expected) => {
    renderWithProviders(<Harness target={target} nodeId={nodeId} />);
    fireEvent.mouseDown(screen.getByLabelText('SNI'));
    fireEvent.click(await screen.findByText(`${name} · ${nodeId ? 'Foreign node' : 'TLS'}`));
    await waitFor(() => {
      const values = JSON.parse(screen.getByTestId('values').textContent || '{}');
      expect(values.streamSettings.realitySettings.serverNames).toEqual([name]);
      expect(values.streamSettings.realitySettings.target).toBe(expected);
    });
  },
);

it.each([0, 7])(
  'hides sites on other nodes and entries without an HTTPS target on node %s',
  async (nodeId) => {
    renderWithProviders(<Harness nodeId={nodeId} />);
    fireEvent.mouseDown(screen.getByLabelText('SNI'));
    await screen.findByText(
      `${nodeId ? 'node' : 'site'}.example.org · ${nodeId ? 'Foreign node' : 'TLS'}`,
    );
    expect(screen.queryByText('saved.example.org · Foreign node')).toBeNull();
    expect(screen.queryByText('legacy.example.org · TLS')).toBeNull();
    expect(
      screen.queryByText(
        `${nodeId ? 'site' : 'node'}.example.org · ${nodeId ? 'TLS' : 'Foreign node'}`,
      ),
    ).toBeNull();
  },
);
