import { describe, expect, it } from 'vitest';
import { selectableRoutingInboundTags } from '@/pages/xray/routing/inbound-options';

const options = [
  { tag: 'local-vless', protocol: 'vless', enable: true, nodeId: null },
  { tag: 'wg-local', protocol: 'wireguard', enable: true },
  { tag: 'vetroff-sidecar', protocol: 'vetroff', enable: true },
  { tag: 'disabled', protocol: 'vless', enable: false },
  { tag: 'remote', protocol: 'vless', enable: true, nodeId: 9 },
  { tag: ' ', protocol: 'vless', enable: true },
];

describe('routing inbound selector', () => {
  it('recovers active local native inbounds when the sing-box snapshot is empty', () => {
    expect(selectableRoutingInboundTags('sing-box', [], options)).toEqual([
      'local-vless',
      'wg-local',
    ]);
  });

  it('preserves generated relays but does not invent core listeners for sidecars', () => {
    expect(selectableRoutingInboundTags('sing-box', ['relay', 'local-vless'], options)).toEqual([
      'relay',
      'local-vless',
      'wg-local',
    ]);
  });

  it('uses the authenticated local list in Xray mode without duplicates', () => {
    expect(selectableRoutingInboundTags('xray', ['local-vless'], options)).toEqual([
      'local-vless',
      'wg-local',
      'vetroff-sidecar',
    ]);
  });

  it('rejects disabled, remote and blank database tags', () => {
    expect(selectableRoutingInboundTags('sing-box', [], options.slice(3))).toEqual([]);
  });

  it.each(['xray', 'sing-box'] as const)('filters stale snapshot tags on %s', (engine) => {
    expect(
      selectableRoutingInboundTags(engine, ['remote', 'disabled', 'relay', 'local-vless'], options),
    ).toEqual(expect.not.arrayContaining(['remote', 'disabled']));
    expect(selectableRoutingInboundTags(engine, ['relay'], options)).toContain('relay');
  });
});
