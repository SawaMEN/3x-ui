import { describe, expect, it } from 'vitest';
import { masqueClientDefaults, warpMasqueDefaults } from '@/pages/singbox/masque-settings';
import { outboundProtocolLabel } from '@/pages/singbox/outbound-editor-model';
import { outboundDefaults, outboundErrors } from '@/pages/singbox/simple-settings';

describe('MASQUE native configuration', () => {
  it('creates Cloudflare WARP with native MASQUE defaults in the protocol editor', () => {
    const warp = warpMasqueDefaults('warp-masque');
    expect(warp).toEqual({
      type: 'masque',
      tag: 'warp-masque',
      use_http2: false,
      use_ipv6: false,
      profile: { recreate: false },
    });
    expect(outboundDefaults('masque', 'warp-masque')).toEqual(warp);
    expect(outboundErrors(warp, [])).toEqual([]);
    expect(outboundErrors(warp, ['warp-masque'])).toContain(
      'Это название уже используется другим подключением или туннелем.',
    );
    expect(outboundProtocolLabel('masque')).toBe('Cloudflare WARP (MASQUE)');
    expect(outboundProtocolLabel('masque-client')).toBe('MASQUE Client (CONNECT-IP)');
  });
  it('prevents saving an incomplete CONNECT-IP client', () => {
    const value = masqueClientDefaults('proxy');
    expect(outboundDefaults('masque-client', 'proxy')).toEqual(value);
    expect(outboundErrors(value, [])).toContain('Укажите MASQUE-сервер.');
    expect(outboundErrors({ ...value, server: 'vpn.example.com' }, [])).toEqual([]);
    expect(
      outboundErrors({ ...value, server: 'vpn.example.com', username: 'alice:bob' }, []),
    ).toContain('Имя пользователя MASQUE не может содержать двоеточие.');
    expect(
      outboundErrors({ ...value, server: 'vpn.example.com', tls: { enabled: false } }, []),
    ).toContain('MASQUE требует TLS.');
  });
});

it.each([
  { username: 'alice\nproxy' },
  { username: 123 },
  { version: '3' },
  { version: true },
  { path: '/{unknown}' },
  { path: '/bad path' },
])('rejects a MASQUE client the core cannot use: %j', (patch) => {
  expect(
    outboundErrors({ ...masqueClientDefaults('proxy'), server: 'vpn.example.com', ...patch }, []),
  ).not.toEqual([]);
});
