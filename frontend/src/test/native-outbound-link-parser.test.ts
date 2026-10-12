import { describe, expect, it } from 'vitest';
import { parseNativeOutboundLink } from '@/lib/xray/native-outbound-link-parser';

describe('native outbound link import', () => {
  it('imports AnyTLS credentials, uTLS, SNI and TLS options without Xray conversion', () => {
    expect(
      parseNativeOutboundLink(
        'anytls://secret@node.example:443?security=tls&sni=cover.example&fp=chrome&alpn=h2%2Chttp%2F1.1&insecure=1#AnyTLS',
      ),
    ).toEqual({
      protocol: 'singbox:anytls',
      tag: 'AnyTLS',
      settings: {
        server: 'node.example',
        server_port: 443,
        password: 'secret',
        tls: {
          enabled: true,
          server_name: 'cover.example',
          insecure: true,
          alpn: ['h2', 'http/1.1'],
          utls: { enabled: true, fingerprint: 'chrome' },
        },
      },
    });
  });

  it('imports TUIC auth, congestion, heartbeat and TLS', () => {
    const out = parseNativeOutboundLink(
      'tuic://uuid:password@tuic.example:8443?congestion_control=cubic&udp_relay_mode=quic&heartbeat=15s&sni=tuic.example#TUIC',
    );
    expect(out?.protocol).toBe('singbox:tuic');
    expect(out?.settings).toMatchObject({
      server: 'tuic.example',
      server_port: 8443,
      uuid: 'uuid',
      password: 'password',
      congestion_control: 'cubic',
      udp_relay_mode: 'quic',
      heartbeat: '15s',
      tls: { enabled: true, server_name: 'tuic.example' },
    });
  });

  it('imports Hysteria v1 with bandwidth and obfuscation', () => {
    const out = parseNativeOutboundLink(
      'hysteria://auth@node.example:443?up_mbps=80&down_mbps=180&obfs=obfuscated',
    );
    expect(out?.protocol).toBe('singbox:hysteria');
    expect(out?.settings).toMatchObject({
      auth_str: 'auth',
      up_mbps: 80,
      down_mbps: 180,
      obfs: 'obfuscated',
      tls: { enabled: true },
    });
  });

  it('imports ShadowTLS, Mieru and SSH without losing authentication', () => {
    expect(
      parseNativeOutboundLink('shadowtls://secret@server.example:443?version=3&sni=cover.example')
        ?.settings,
    ).toMatchObject({
      password: 'secret',
      version: 3,
      tls: { server_name: 'cover.example' },
    });
    expect(
      parseNativeOutboundLink('mieru://alice:password@server.example:8080?transport=tcp')?.settings,
    ).toMatchObject({
      username: 'alice',
      password: 'password',
      portBindings: [{ protocol: 'TCP', port: 8080 }],
    });
    expect(
      parseNativeOutboundLink('ssh://alice:password@server.example:22')?.settings,
    ).toMatchObject({
      user: 'alice',
      password: 'password',
      server_port: 22,
    });
  });

  it('preserves native extra JSON settings for future core features', () => {
    const extra = encodeURIComponent(
      JSON.stringify({
        tls: { enabled: true, reality: { enabled: true, public_key: 'public-key' } },
        multiplex: { enabled: true, protocol: 'smux' },
        detour: 'shadowtls-out',
      }),
    );
    const out = parseNativeOutboundLink('anytls://secret@server.example:443?extra=' + extra);
    expect(out?.settings).toMatchObject({
      password: 'secret',
      multiplex: { enabled: true, protocol: 'smux' },
      detour: 'shadowtls-out',
      tls: { reality: { public_key: 'public-key' } },
    });
  });

  it.each([
    'unsupported://user@example.com:443',
    'tuic://user@',
    'tuic://user@node.example:65536',
    'anytls://secret@node.example:443?extra=%5B%5D',
    'anytls://secret@node.example:443?extra=broken',
  ])('rejects malformed or unrecognized URIs: %s', (input) => {
    expect(parseNativeOutboundLink(input)).toBeNull();
  });
});
