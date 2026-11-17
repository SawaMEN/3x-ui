import { act, fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, expect, it, vi } from 'vitest';
import { useState } from 'react';
import AmneziaVpnExport from '@/pages/clients/AmneziaVpnExport';
import { buildAmneziaVpnProfile } from '@/pages/clients/amneziaVpnProfile';
import { renderWithProviders } from './test-utils';

vi.mock('@/pages/clients/amneziaVpnProfile', () => ({ buildAmneziaVpnProfile: vi.fn() }));
vi.mock('@/pages/inbounds/qr', () => ({
  QrPanel: ({ value }: { value: string }) => <output>{value}</output>,
}));

const inbound = { id: 1, tag: 'awg', protocol: 'amneziawg', awgServer: { s3: 12, s4: 12 } };
const subject = (config: string) => (
  <AmneziaVpnExport config={config} remark="Test" downloadName="test.conf" inbound={inbound} />
);

beforeEach(() => vi.mocked(buildAmneziaVpnProfile).mockReset());

it('shows the application link and scans every QR part in order', async () => {
  vi.mocked(buildAmneziaVpnProfile).mockResolvedValue({
    link: 'vpn://profile',
    qr: ['part-one', 'part-two'],
  });
  renderWithProviders(subject('native config'));
  expect(screen.getByText('native config')).toBeTruthy();
  fireEvent.click(screen.getByText('Amnezia VPN'));
  await waitFor(() =>
    expect(screen.getByRole('link', { name: 'Open in Amnezia VPN' }).getAttribute('href')).toBe(
      'vpn://profile',
    ),
  );
  expect(screen.getByText('part-one')).toBeTruthy();
  fireEvent.click(screen.getByRole('button', { name: 'Next QR' }));
  expect(screen.getByText('part-two')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Next QR' }).hasAttribute('disabled')).toBe(true);
});

it('ignores an older profile request after changing the client configuration', async () => {
  let resolveOld!: (value: { link: string; qr: string[] }) => void;
  vi.mocked(buildAmneziaVpnProfile).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        resolveOld = resolve;
      }),
  );
  vi.mocked(buildAmneziaVpnProfile).mockResolvedValueOnce({ link: 'vpn://new', qr: ['new-part'] });
  function ChangingSubject() {
    const [config, setConfig] = useState('old config');
    return (
      <>
        <button onClick={() => setConfig('new config')}>Change config</button>
        {subject(config)}
      </>
    );
  }
  renderWithProviders(<ChangingSubject />);
  fireEvent.click(screen.getByText('Amnezia VPN'));
  fireEvent.click(screen.getByRole('button', { name: 'Change config' }));
  await waitFor(() => expect(screen.getByText('vpn://new')).toBeTruthy());
  await act(async () => {
    resolveOld({ link: 'vpn://old', qr: ['old-part'] });
  });
  expect(screen.queryByText('vpn://old')).toBeNull();
  expect(screen.getByText('new-part')).toBeTruthy();
});
