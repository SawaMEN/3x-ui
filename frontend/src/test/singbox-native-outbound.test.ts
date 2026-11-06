import { describe, expect, it } from 'vitest';
import {
  buildNativeSingBoxOutbound,
  normalizeOutboundJsonForPanel,
  normalizeOutboundJsonImport,
} from '@/pages/xray/outbounds/singbox-native-outbound';
import { SINGBOX_NATIVE_OUTBOUND_PROTOCOLS } from '@/pages/xray/outbounds/outbound-form-constants';
import { isUdpOutbound } from '@/hooks/useXraySetting';
import { isUntestable } from '@/pages/xray/outbounds/outbounds-tab-helpers';

describe('native sing-box outbound templates', () => {
  it.each(SINGBOX_NATIVE_OUTBOUND_PROTOCOLS)('builds a native %s wrapper', (protocol) => {
    const outbound = buildNativeSingBoxOutbound(protocol, 'chosen-tag');
    expect(outbound.protocol).toBe(protocol);
    expect(outbound.tag).toBe('chosen-tag');
    expect(outbound.settings).toBeTypeOf('object');
    if (['singbox:vmess', 'singbox:vless', 'singbox:tuic'].includes(protocol))
      expect(outbound.settings).toHaveProperty('uuid');
  });
  it('preserves all options from official JSON in the native wrapper', () => {
    const raw = {
      type: 'VLESS',
      tag: 'proxy',
      server: '::1',
      server_port: 443,
      uuid: 'id',
      packet_encoding: 'xudp',
      multiplex: { enabled: true },
      tls: { enabled: true },
    };
    const { type: _, tag, ...settings } = raw;
    expect(normalizeOutboundJsonForPanel(raw)).toEqual({
      protocol: 'singbox:vless',
      tag,
      settings,
    });
  });
  it('accepts future native sing-box types without a panel allow-list', () => {
    expect(
      normalizeOutboundJsonForPanel({
        type: 'Future-Transport',
        tag: 'future',
        server: 'example.com',
        server_port: 443,
        experimental_option: { enabled: true },
      }),
    ).toEqual({
      protocol: 'singbox:future-transport',
      tag: 'future',
      settings: {
        server: 'example.com',
        server_port: 443,
        experimental_option: { enabled: true },
      },
    });
  });
  it('treats a typed native object as sing-box even if that type has a protocol option', () => {
    expect(
      normalizeOutboundJsonForPanel({
        type: 'future-transport',
        tag: 'future',
        protocol: 'udp',
        server: 'example.com',
      }),
    ).toEqual({
      protocol: 'singbox:future-transport',
      tag: 'future',
      settings: { protocol: 'udp', server: 'example.com' },
    });
  });
  it.each([
    'singbox:hysteria',
    'singbox:hysteria2',
    'singbox:tuic',
    'selector',
    'singbox:urltest',
    'mieru',
    'singbox:mieru',
  ])('uses a handshake probe for %s', (protocol) => {
    expect(isUdpOutbound({ protocol })).toBe(true);
  });
  it.each(['singbox:direct', 'singbox:block', 'singbox:dns'])(
    'disables testing for %s',
    (protocol) => {
      expect(isUntestable({ key: 0, tag: 'native', protocol })).toBe(true);
    },
  );
});

describe('outbound chain probe mode', () => {
  it('uses a handshake for a native detour', () => {
    expect(
      isUdpOutbound({ protocol: 'singbox:shadowsocks', settings: { detour: 'shadowtls' } }),
    ).toBe(true);
  });
  it('uses a handshake for a socket detour', () => {
    expect(
      isUdpOutbound({ protocol: 'vless', streamSettings: { sockopt: { dialerProxy: 'hop' } } }),
    ).toBe(true);
  });
  it('uses a handshake for native QUIC transport', () => {
    expect(
      isUdpOutbound({ protocol: 'singbox:vmess', settings: { transport: { type: 'quic' } } }),
    ).toBe(true);
  });
  it('keeps unchained TCP proxies in TCP mode', () => {
    expect(isUdpOutbound({ protocol: 'singbox:ssh', settings: { detour: '' } })).toBe(false);
  });
});

describe('atomic outbound JSON import', () => {
  it('normalizes native single, array and full config imports', () => {
    const native = { type: 'VLESS', server: '127.0.0.1', server_port: 443, uuid: 'id' };
    for (const value of [native, [native], { outbounds: [native] }]) {
      expect(normalizeOutboundJsonImport(value, ['vless-out'])).toEqual([
        {
          protocol: 'singbox:vless',
          tag: 'vless-out-2',
          settings: { server: '127.0.0.1', server_port: 443, uuid: 'id' },
        },
      ]);
    }
  });
  it.each([
    null,
    7,
    true,
    'text',
    [],
    {},
    [null],
    [7],
    [{ type: '' }],
    [{ type: 7 }],
    [{ protocol: 'http', tag: 7 }],
    [{ protocol: 'http', tag: ' x ' }],
    [{ protocol: 'http', tag: 'x', settings: [] }],
    [
      { protocol: 'http', tag: 'x' },
      { protocol: 'socks', tag: 'x' },
    ],
  ])('rejects malformed imports: %j', (value) => {
    expect(() => normalizeOutboundJsonImport(value)).toThrow();
  });
  it('rejects a collision with an existing tag', () => {
    expect(() => normalizeOutboundJsonImport({ type: 'http', tag: 'x' }, ['x'])).toThrow(
      'Duplicate',
    );
  });
});
