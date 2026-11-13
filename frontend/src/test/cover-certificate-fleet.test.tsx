import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import CoverCertificateFleet from '@/pages/manager/CoverCertificateFleet';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

beforeEach(() => {
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url) =>
      new Msg(
        true,
        '',
        url.endsWith('/cover-sites')
          ? [{ nodeId: 2, domain: 'web.site.ru' }]
          : [
              {
                sourceNodeId: 2,
                domain: 'web.site.ru',
                targets: [{ nodeId: 3, state: 'ready', expiresAt: 2000000000 }],
              },
            ],
      ),
  );
  vi.mocked(HttpUtil.post).mockResolvedValue(new Msg(true, '', []));
  vi.mocked(HttpUtil.post).mockClear();
});

it('saves the source and recipients without sending keys or issuing another certificate', async () => {
  renderWithProviders(
    <CoverCertificateFleet
      nodes={[
        {
          id: 2,
          name: 'USA',
          enable: true,
          scheme: 'https',
          tlsVerifyMode: 'verify',
          status: 'online',
        },
        {
          id: 3,
          name: 'Finland',
          enable: true,
          scheme: 'https',
          tlsVerifyMode: 'verify',
          status: 'online',
        },
      ]}
    />,
  );
  await screen.findByText('Ready');
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Certificate source site' }));
  fireEvent.click(await screen.findByText('web.site.ru — USA'));
  const save = screen.getByRole('button', { name: 'Connect and update automatically' });
  await waitFor(() => expect(save).toHaveProperty('disabled', false));
  expect(HttpUtil.post).not.toHaveBeenCalled();
  fireEvent.click(save);
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/domains/certificates/fleet',
      { sourceNodeId: 2, domain: 'web.site.ru', nodeIds: [3] },
      { silent: true },
    ),
  );
});

it('keeps an unavailable saved source selectable so renewal delivery can be stopped', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url) =>
      new Msg(
        true,
        '',
        url.endsWith('/cover-sites')
          ? []
          : [
              {
                sourceNodeId: 77,
                domain: 'web.site.ru',
                targets: [{ nodeId: 3, state: 'retrying', expiresAt: 0 }],
              },
            ],
      ),
  );
  renderWithProviders(<CoverCertificateFleet nodes={[]} />);
  await screen.findByText('Retrying automatically');
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Certificate source site' }));
  expect(await screen.findByText('web.site.ru — #77')).toBeTruthy();
});

it('keeps saved recipients instead of replacing them with the current server default', async () => {
  renderWithProviders(<CoverCertificateFleet nodes={[]} defaultRecipientId={0} />);
  await screen.findByText('Ready');
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Certificate source site' }));
  fireEvent.click(await screen.findByText('web.site.ru — #2'));
  fireEvent.click(screen.getByRole('button', { name: 'Connect and update automatically' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/domains/certificates/fleet',
      { sourceNodeId: 2, domain: 'web.site.ru', nodeIds: [3] },
      { silent: true },
    ),
  );
});
