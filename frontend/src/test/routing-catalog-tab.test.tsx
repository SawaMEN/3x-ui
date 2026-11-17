import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { expect, it, vi } from 'vitest';
import RoutingCatalogTab from '@/pages/xray/routing/RoutingCatalogTab';
import { HttpUtil } from '@/utils';
import type { XraySettingsValue, SetTemplate } from '@/hooks/useXraySetting';

vi.mock('@/components/geodata', () => ({ GeoBrowserModal: () => null }));

function mount(setTemplateSettings: SetTemplate) {
  const template = {
    outbounds: [],
    routing: { rules: [{ type: 'field', outboundTag: 'direct' }] },
  } as XraySettingsValue;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <RoutingCatalogTab
        templateSettings={template}
        setTemplateSettings={setTemplateSettings}
        outboundTags={['vpn', 'direct']}
      />
    </QueryClientProvider>,
  );
  return { template, ...view };
}

it('stages a catalog route without publishing or overwriting existing rules', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
    success: true,
    msg: '',
    obj: url.endsWith('/categories')
      ? [
          { id: 'youtube', token: 'geosite:youtube', available: true },
          { id: 'discord', token: 'geosite:discord', available: false },
        ]
      : [],
  }));
  vi.mocked(HttpUtil.post).mockClear();
  const setTemplate = vi.fn<SetTemplate>();
  const { template } = mount(setTemplate);
  const youtubeRow = (await screen.findByText('geosite:youtube')).closest('tr')!;
  fireEvent.click(within(youtubeRow).getByRole('button', { name: 'Assign route' }));
  await screen.findByRole('dialog');
  fireEvent.click(screen.getByRole('button', { name: 'Add routing rule' }));
  expect(setTemplate).toHaveBeenCalledOnce();
  const updater = setTemplate.mock.calls[0][0];
  if (typeof updater !== 'function') throw new Error('Expected functional template update');
  expect(updater(template)?.routing?.rules).toEqual([
    expect.objectContaining({ domain: ['geosite:youtube'], outboundTag: 'vpn' }),
    template.routing!.rules![0],
  ]);
  expect(HttpUtil.post).not.toHaveBeenCalled();
  const discordRow = screen.getByText('geosite:discord').closest('tr')!;
  expect(
    within(discordRow).getByRole('button', { name: 'Assign route' }).hasAttribute('disabled'),
  ).toBe(true);
}, 15000);

it('shows cached source failure and adds its stable list reference', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
    success: true,
    msg: '',
    obj: url.endsWith('/categories')
      ? []
      : [
          {
            id: 7,
            name: 'My source',
            url: 'https://example.com/list',
            updateInterval: 86400,
            domainCount: 10,
            ipCount: 2,
            lastUpdated: 123,
            lastError: 'HTTP 503',
          },
        ],
  }));
  const setTemplate = vi.fn<SetTemplate>();
  const { template } = mount(setTemplate);
  await screen.findByText('Last working list retained: HTTP 503');
  const row = screen.getByText('My source').closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Assign route' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Add routing rule' }));
  await waitFor(() => expect(setTemplate).toHaveBeenCalledOnce());
  const updater = setTemplate.mock.calls[0][0];
  if (typeof updater !== 'function') throw new Error('Expected functional template update');
  expect(updater(template)?.routing?.rules?.[0]).toMatchObject({
    routingListId: 7,
    outboundTag: 'vpn',
  });
  expect(screen.getByText('Every 24 hours')).toBeTruthy();
}, 15000);

it('removes catalog notifications when the tab unmounts', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
    success: true,
    msg: '',
    obj: url.endsWith('/categories')
      ? [{ id: 'youtube', token: 'geosite:youtube', available: true }]
      : [],
  }));
  const { unmount } = mount(vi.fn<SetTemplate>());
  const row = (await screen.findByText('geosite:youtube')).closest('tr')!;
  fireEvent.click(within(row).getByRole('button', { name: 'Assign route' }));
  const apply = await screen.findByRole('button', { name: 'Add routing rule' });
  await act(async () => fireEvent.click(apply));
  const notice = 'Rule added. Save the configuration to apply it.';
  await screen.findByText(notice);
  unmount();
  expect(screen.queryByText(notice)).toBeNull();
}, 15000);

it('cancels catalog reads when leaving the tab', async () => {
  const signals: AbortSignal[] = [];
  vi.mocked(HttpUtil.get).mockImplementation((_url, _params, options) => {
    if (options?.signal) signals.push(options.signal);
    return new Promise<never>(() => {});
  });
  const { unmount } = mount(vi.fn<SetTemplate>());
  await waitFor(() => expect(signals).toHaveLength(2));
  expect(signals.every((signal) => !signal.aborted)).toBe(true);
  unmount();
  expect(signals.every((signal) => signal.aborted)).toBe(true);
});
