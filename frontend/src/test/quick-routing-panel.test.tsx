import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import QuickRoutingPanel from '@/pages/xray/routing/QuickRoutingPanel';
import { HttpUtil } from '@/utils';

vi.mock('@/api/queries/useInboundOptions', () => ({
  useInboundOptions: () => ({
    data: [{ tag: 'local', protocol: 'vless', enable: true, nodeId: null }],
    isPending: false,
    isError: false,
  }),
}));

const target = {
  tag: 'node-route-42',
  inboundId: 42,
  nodeId: 2,
  nodeName: 'Foreign',
  remark: 'Exit',
  protocol: 'vless',
  port: 443,
  available: true,
};

describe('quick route activation', () => {
  beforeEach(() => {
    vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, obj: [target] } as never);
  });

  it('prepares destination credentials before enabling a saved route', async () => {
    let finish: (value: never) => void = () => {};
    vi.mocked(HttpUtil.post).mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const onChange = vi.fn();
    const rule = {
      comment: 'panel-quick:inbound:test',
      enabled: false,
      action: 'route',
      inbound: ['local'],
      outbound: target.tag,
    };
    render(
      <QuickRoutingPanel
        engine="sing-box"
        rules={[rule]}
        inboundTags={['local']}
        outboundTags={['direct']}
        onChange={onChange}
      />,
    );
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('switch', { name: 'Включить маршрут #1' }));
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/xray/node-route/ensure', {
        inboundId: 42,
        core: 'sing-box',
      }),
    );
    expect(onChange).not.toHaveBeenCalled();
    finish({ success: true, obj: { tag: target.tag } } as never);
    await waitFor(() => expect(onChange).toHaveBeenCalledWith([{ ...rule, enabled: true }]));
  });

  it('keeps the route disabled when destination preparation fails', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: false,
      msg: 'Destination offline',
    } as never);
    const onChange = vi.fn();
    render(
      <QuickRoutingPanel
        engine="xray"
        rules={[
          {
            ruleTag: 'panel-quick:inbound:test',
            enabled: false,
            inboundTag: ['local'],
            outboundTag: target.tag,
          },
        ]}
        inboundTags={['local']}
        outboundTags={['direct']}
        onChange={onChange}
      />,
    );
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('switch', { name: 'Включить маршрут #1' }));
    await screen.findByText('Destination offline');
    expect(onChange).not.toHaveBeenCalled();
  });
});
