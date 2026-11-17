import { describe, expect, it } from 'vitest';
import { outboundBasicConversionLoss } from '@/lib/xray/outbound-loss-check';

describe('lossless outbound editing gate', () => {
  it('permits a simple VLESS outbound with nonstandard core-supported encryption', () => {
    expect(
      outboundBasicConversionLoss({
        protocol: 'vless',
        tag: 'pq-node',
        settings: {
          address: 'edge.example',
          port: 443,
          id: '11111111-2222-4333-8444-555555555555',
          encryption: 'mlkem768x25519plus.native.0rtt',
        },
        streamSettings: { network: 'tcp', security: 'none' },
      }),
    ).toBeNull();
  });

  it('allows one legacy VLESS vnext server/user without losing credentials', () => {
    expect(
      outboundBasicConversionLoss({
        protocol: 'vless',
        tag: 'legacy',
        settings: {
          vnext: [
            {
              address: 'edge.example',
              port: 443,
              users: [{ id: '11111111-2222-4333-8444-555555555555', encryption: 'none' }],
            },
          ],
        },
        streamSettings: { network: 'tcp', security: 'none' },
      }),
    ).toBeNull();
  });

  it('rejects hidden protocol settings before the basic editor can drop them', () => {
    expect(
      outboundBasicConversionLoss({
        protocol: 'vless',
        tag: 'advanced',
        settings: {
          address: 'edge.example',
          port: 443,
          id: '11111111-2222-4333-8444-555555555555',
          packetEncoding: 'xudp',
        },
      }),
    ).toBe('settings.packetEncoding');
  });

  it('keeps multiple legacy VLESS destinations in JSON', () => {
    const node = {
      address: 'edge.example',
      port: 443,
      users: [{ id: '11111111-2222-4333-8444-555555555555' }],
    };
    expect(
      outboundBasicConversionLoss({
        protocol: 'vless',
        tag: 'multi',
        settings: { vnext: [node, { ...node, address: 'other.example' }] },
      }),
    ).toBe('settings.vnext');
  });

  it('permits advanced TLS extensions when the adapter retains them', () => {
    expect(
      outboundBasicConversionLoss({
        protocol: 'vless',
        tag: 'tls',
        settings: {
          address: 'edge.example',
          port: 443,
          id: '11111111-2222-4333-8444-555555555555',
        },
        streamSettings: {
          network: 'tcp',
          security: 'tls',
          tlsSettings: { serverName: 'example.com', experimentalCoreOption: 'retain' },
        },
      }),
    ).toBeNull();
  });
});
