import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { expect, it, vi } from 'vitest';

import SniSearchInput from '@/pages/inbounds/form/security/SniSearchInput';

const { sites } = vi.hoisted(() => ({
  sites: [
    { name: 'vpn.example.org', source: 'gateway', certFile: '/local.crt', keyFile: '/local.key' },
    {
      name: 'vpn.example.org',
      source: 'remote',
      nodeId: 7,
      nodeName: 'First node',
      certFile: '',
      keyFile: '',
    },
    {
      name: 'vpn.example.org',
      source: 'remote',
      nodeId: 9,
      nodeName: 'Second node',
      certFile: '',
      keyFile: '',
    },
  ],
}));

vi.mock('@/api/queries/useManagedSniDomains', () => ({
  useManagedSniDomains: () => ({ data: sites }),
}));
vi.mock('@/pages/inbounds/form/security/RealityTargetScannerModal', () => ({
  default: () => null,
  scannedServerName: () => '',
}));

it('selects the chosen node when several certificates share one SNI', async () => {
  const onChange = vi.fn();
  const onPick = vi.fn();
  function Harness() {
    const form = useForm({ defaultValues: { nodeId: 9 } });
    return (
      <FormProvider {...form}>
        <SniSearchInput onChange={onChange} onPickManagedDomain={onPick} />
      </FormProvider>
    );
  }
  render(<Harness />);
  fireEvent.mouseDown(screen.getByRole('combobox'));
  fireEvent.click(await screen.findByText('vpn.example.org · Second node'));
  expect(onChange).toHaveBeenCalledWith('vpn.example.org');
  expect(onPick).toHaveBeenCalledWith(sites[2]);
});
