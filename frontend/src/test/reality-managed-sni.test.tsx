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

function Harness({ target = '' }: { target?: string }) {
  const methods = useForm({
    defaultValues: { streamSettings: { realitySettings: { target, serverNames: [] } } },
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
  ['saved.example.org', '', ''],
  ['legacy.example.org', '', ''],
  ['site.example.org', '', 'site.example.org:8443'],
  ['site.example.org', 'custom.example.org:9443', 'custom.example.org:9443'],
])('offers %s and preserves or fills its target correctly', async (name, target, expected) => {
  renderWithProviders(<Harness target={target} />);
  fireEvent.mouseDown(screen.getByLabelText('SNI'));
  fireEvent.click(
    await screen.findByText(`${name} · ${name.startsWith('saved') ? 'Foreign node' : 'TLS'}`),
  );
  await waitFor(() => {
    const values = JSON.parse(screen.getByTestId('values').textContent || '{}');
    expect(values.streamSettings.realitySettings.serverNames).toEqual([name]);
    expect(values.streamSettings.realitySettings.target).toBe(expected);
  });
});
