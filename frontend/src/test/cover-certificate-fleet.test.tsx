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
        url.split('?')[0].endsWith('/cover-sites')
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
        url.split('?')[0].endsWith('/cover-sites')
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

it('offers a remote Telemt website to the main server using certificate-source discovery', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url) =>
      new Msg(
        true,
        '',
        url.includes('/cover-sites')
          ? {
              sites: [{ nodeId: 8, domain: 'telemt.example.org', siteCatalog: 'telemt' }],
              nodes: [{ nodeId: 8, nodeName: 'Website node', state: 'ready' }],
            }
          : [],
      ),
  );
  renderWithProviders(
    <CoverCertificateFleet nodes={[{ id: 8, name: 'Website node' }]} defaultRecipientId={0} />,
  );
  const source = await screen.findByRole('combobox', { name: 'Certificate source site' });
  await waitFor(() => expect(source).toHaveProperty('disabled', false));
  fireEvent.mouseDown(source);
  fireEvent.click(await screen.findByText('telemt.example.org — Website node'));
  fireEvent.click(screen.getByRole('button', { name: 'Connect and update automatically' }));
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/domains/certificates/fleet',
      { sourceNodeId: 8, domain: 'telemt.example.org', nodeIds: [0] },
      { silent: true },
    ),
  );
  expect(HttpUtil.get).toHaveBeenCalledWith(
    '/panel/api/domains/cover-sites?certificates=1',
    undefined,
    { silent: true },
  );
});

it('explains why only local sites are visible and can rediscover a recovered remote node', async () => {
  let refreshed = false;
  vi.mocked(HttpUtil.get).mockImplementation(
    async (url) =>
      new Msg(
        true,
        '',
        url.includes('/cover-sites')
          ? {
              sites: [
                { nodeId: 0, domain: 'local.example.org' },
                ...(refreshed ? [{ nodeId: 8, domain: 'remote.example.org' }] : []),
              ],
              nodes: [
                {
                  nodeId: 8,
                  nodeName: 'Remote node',
                  state: refreshed ? 'ready' : 'update_required',
                },
                { nodeId: 9, nodeName: 'Locked node', state: 'access_denied' },
                { nodeId: 10, nodeName: 'HTTP node', state: 'https_required' },
              ],
            }
          : [],
      ),
  );
  renderWithProviders(
    <CoverCertificateFleet nodes={[{ id: 8, name: 'Remote node' }]} defaultRecipientId={0} />,
  );
  expect(await screen.findByText(/Update this node panel/)).toBeTruthy();
  expect(screen.getByText(/Check the node API token/)).toBeTruthy();
  expect(screen.getByText(/Configure verified HTTPS/)).toBeTruthy();
  refreshed = true;
  fireEvent.click(screen.getByRole('button', { name: 'Refresh certificate sources' }));
  expect(await screen.findByText('Sites found')).toBeTruthy();
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Certificate source site' }));
  expect(await screen.findByText('remote.example.org — Remote node')).toBeTruthy();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});

it('explains a legacy main-panel response instead of hiding missing node discovery', async () => {
  renderWithProviders(<CoverCertificateFleet nodes={[]} />);
  expect(
    await screen.findByText(/Update the main panel: it returned a legacy site list/),
  ).toBeTruthy();
});
