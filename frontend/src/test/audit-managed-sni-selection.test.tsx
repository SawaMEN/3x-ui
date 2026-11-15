import { fireEvent, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import { expect, it, vi } from 'vitest';

import SniSearchInput from '@/pages/inbounds/form/security/SniSearchInput';
import { renderWithProviders } from './test-utils';

const { sites } = vi.hoisted(() => ({
  sites: [
    { name: 'vpn.example.org', source: 'gateway', certFile: '/local.crt', keyFile: '/local.key' },
    {
      name: 'vpn.example.org',
      source: 'remote',
      nodeId: 7,
      nodeName: 'First node',
      expiresAt: 4102444800,
      certFile: '',
      keyFile: '',
    },
    {
      name: 'vpn.example.org',
      source: 'remote',
      nodeId: 9,
      nodeName: 'Second node',
      expiresAt: 4102444800,
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

it('offers only the selected node and refreshes suggestions when that node changes', async () => {
  const onChange = vi.fn();
  const onPick = vi.fn();
  function Harness() {
    const form = useForm({ defaultValues: { nodeId: 9 } });
    return (
      <FormProvider {...form}>
        <SniSearchInput onChange={onChange} onPickManagedDomain={onPick} />
        <button type="button" onClick={() => form.setValue('nodeId', 7)}>
          Select first node
        </button>
      </FormProvider>
    );
  }
  renderWithProviders(<Harness />);
  fireEvent.mouseDown(screen.getByRole('combobox'));
  expect(screen.queryByText('vpn.example.org · First node')).toBeNull();
  expect(screen.queryByText('vpn.example.org · TLS')).toBeNull();
  fireEvent.click(await screen.findByText('vpn.example.org · Second node'));
  expect(onChange).toHaveBeenCalledWith('vpn.example.org');
  expect(onPick).toHaveBeenCalledWith(sites[2]);
  fireEvent.click(screen.getByRole('button', { name: 'Select first node' }));
  fireEvent.mouseDown(screen.getByRole('combobox'));
  fireEvent.click(await screen.findByText('vpn.example.org · First node'));
  expect(screen.queryByText('vpn.example.org · Second node')).toBeNull();
  expect(onPick).toHaveBeenLastCalledWith(sites[1]);
});
