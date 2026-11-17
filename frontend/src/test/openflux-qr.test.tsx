import { act, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import QrCodeModal from '@/pages/inbounds/qr/QrCodeModal';
import { DBInbound } from '@/models/dbinbound';
import { HttpUtil, Msg } from '@/utils';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

vi.mock('@/pages/inbounds/qr/QrPanel', () => ({
  default: ({ value }: { value: string }) => <output>{value}</output>,
}));

function row(mixed = false) {
  return new DBInbound({
    id: 42,
    protocol: 'openflux',
    port: 443,
    settings: {
      context: 'context',
      clients: [{ email: 'alice', enable: true, password: 'a'.repeat(32) }],
      transports: [
        { type: 'oneme', token: 'server-token', uid: '123' },
        ...(mixed ? [{ type: 'direct', priority: 100 }] : []),
      ],
    },
    streamSettings: {},
    sniffing: {},
  });
}

describe('MAX client QR guidance', () => {
  it('does not request an unsupported MAX-only share and never shows a cached direct QR', async () => {
    const queryClient = makeTestQueryClient();
    queryClient.setQueryData(
      ['inbound-server-links', 42, 'openflux', 'alice'],
      [{ link: 'openflux://stale-direct-profile', remark: 'old' }],
    );
    vi.mocked(HttpUtil.get).mockClear();
    renderWithProviders(
      <QrCodeModal open onClose={() => {}} dbInbound={row()} client={{ email: 'alice' }} />,
      { queryClient },
    );
    await act(async () => {});
    expect(screen.getByText(/For MAX, open the inbound settings/)).toBeTruthy();
    expect(screen.queryByText('openflux://stale-direct-profile')).toBeNull();
    expect(
      vi.mocked(HttpUtil.get).mock.calls.some(([path]) => String(path).includes('/links?')),
    ).toBe(false);
  });
  it('keeps the shareable channels available for a mixed MAX and direct setup', async () => {
    vi.mocked(HttpUtil.get).mockResolvedValue(new Msg(true, '', ['openflux://direct-profile']));
    renderWithProviders(
      <QrCodeModal open onClose={() => {}} dbInbound={row(true)} client={{ email: 'alice' }} />,
    );
    expect(await screen.findByText('openflux://direct-profile')).toBeTruthy();
    expect(screen.getByText(/For MAX, open the inbound settings/)).toBeTruthy();
  });
});
