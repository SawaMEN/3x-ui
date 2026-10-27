import { useForm, useWatch, FormProvider } from 'react-hook-form';
import { Form } from 'antd';
import { fireEvent, screen, within, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, describe, expect, it, vi } from 'vitest';
import MtprotoFields from '@/pages/inbounds/form/protocols/mtproto';
import SubscriptionGeneralTab from '@/pages/settings/SubscriptionGeneralTab';
import TelemtPage from '@/pages/telemt/TelemtPage';
import RemoteTelegramProxies from '@/pages/telemt/RemoteTelegramProxies';
import { AllSetting } from '@/models/setting';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

afterEach(() => vi.restoreAllMocks());

function PublicationForm({
  settings = {},
}: {
  settings?: { showOnParent?: boolean; showInSubscription?: boolean };
}) {
  const form = useForm({ defaultValues: { settings } });
  const current = useWatch({ control: form.control, name: 'settings' });
  return (
    <FormProvider {...form}>
      <Form>
        <MtprotoFields />
      </Form>
      <output data-testid="publication">{JSON.stringify(current)}</output>
    </FormProvider>
  );
}

function publicationSwitch(label: string) {
  return within(screen.getByText(label).closest('.ant-form-item') as HTMLElement).getByRole(
    'switch',
  );
}

describe('MTProto publication controls', () => {
  it('preserves legacy visibility and saves the parent and subscription switches independently', () => {
    vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', '{}'));
    const page = renderWithProviders(<PublicationForm />);
    const parent = publicationSwitch('Show on parent node');
    const subscription = publicationSwitch('Show in subscriptions');
    expect(parent.getAttribute('aria-checked')).toBe('true');
    expect(subscription.getAttribute('aria-checked')).toBe('true');
    fireEvent.click(parent);
    expect(screen.getByTestId('publication').textContent).toContain('"showOnParent":false');
    expect(subscription.getAttribute('aria-checked')).toBe('true');
    page.unmount();
    renderWithProviders(
      <PublicationForm settings={{ showOnParent: false, showInSubscription: false }} />,
    );
    expect(publicationSwitch('Show on parent node').getAttribute('aria-checked')).toBe('false');
    fireEvent.click(publicationSwitch('Show in subscriptions'));
    expect(screen.getByTestId('publication').textContent).toContain('"showInSubscription":true');
    expect(screen.getByTestId('publication').textContent).toContain('"showOnParent":false');
  });

  it('saves local, remote and WEB visibility independently in subscription settings', () => {
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <SubscriptionGeneralTab allSetting={new AllSetting()} updateSetting={update} />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole('tab', { name: /Profile/ }));
    for (const [label, key] of [
      ['Local Telegram proxies on subscription page', 'subTelegramLocalEnable'],
      ['Remote-node Telegram proxies on subscription page', 'subTelegramRemoteEnable'],
      ['WEB proxies on subscription page', 'subTelegramWebEnable'],
    ]) {
      fireEvent.click(screen.getByRole('switch', { name: label }));
      expect(update).toHaveBeenCalledWith({ [key]: false });
    }
  });

  it('shows published remote profiles when the parent has no local Telemt installation', async () => {
    const get = vi.spyOn(HttpUtil, 'get').mockImplementation(
      async (url) =>
        new Msg(
          true,
          '',
          url.endsWith('/remote-proxies')
            ? [
                {
                  nodeId: 7,
                  nodeName: 'Remote node',
                  profile: {
                    name: 'Published MTProto',
                    host: 'proxy.example',
                    port: 443,
                    tls: true,
                    link: 'tg://proxy?server=proxy.example&port=443&secret=ee1234',
                  },
                },
              ]
            : url.endsWith('/subscription-proxy')
              ? { enabled: false }
              : {
                  installed: false,
                  active: false,
                  webProxy: {
                    enabled: false,
                    supported: false,
                    carriers: [],
                    domain: '',
                    frontend: 'nginx',
                  },
                },
        ),
    );
    renderWithProviders(<TelemtPage />);
    expect(await screen.findByText('Published MTProto')).toBeTruthy();
    expect(screen.getByText('Remote node')).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Open in Telegram' }).getAttribute('href')).toContain(
      'tg://proxy?',
    );
    await waitFor(() => expect(get).toHaveBeenCalledWith('/panel/api/telemt/status'));
    expect(get.mock.calls.some(([url]) => url.endsWith('/config') || url.endsWith('/proxy'))).toBe(
      false,
    );
  });
  it('paginates a large remote inventory without querying nodes again', async () => {
    const profiles = Array.from({ length: 13 }, (_, index) => ({
      nodeId: 1,
      nodeName: 'Remote',
      profile: {
        name: `Proxy ${index}`,
        host: 'proxy.example',
        port: 443,
        tls: true,
        link: `tg://proxy?server=proxy.example&port=443&secret=dd${index}`,
      },
    }));
    const get = vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', profiles));
    const page = renderWithProviders(<RemoteTelegramProxies />);
    expect(await screen.findByText('Proxy 0')).toBeTruthy();
    expect(screen.queryByText('Proxy 12')).toBeNull();
    fireEvent.click(page.container.querySelector('.ant-pagination-next') as HTMLElement);
    expect(await screen.findByText('Proxy 12')).toBeTruthy();
    expect(screen.queryByText('Proxy 0')).toBeNull();
    expect(get).toHaveBeenCalledTimes(1);
  });
});
