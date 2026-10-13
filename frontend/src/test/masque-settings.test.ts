import { describe, expect, it } from 'vitest';
import { addWarpMasque, masqueClientDefaults } from '@/pages/singbox/masque-settings';
import { outboundDefaults, outboundErrors } from '@/pages/singbox/simple-settings';

describe('MASQUE native configuration', () => {
  it('registers WARP separately and keeps existing cache and connections', () => {
    const config = {
      outbounds: [{ type: 'wireguard', tag: 'warp' }],
      endpoints: [{ type: 'masque-client', tag: 'warp-masque' }],
      experimental: { cache_file: { path: '/data/cache.db', store_fakeip: true } },
    };
    const next = addWarpMasque(config);
    expect(next.outbounds).toEqual([
      { type: 'wireguard', tag: 'warp' },
      {
        type: 'masque',
        tag: 'warp-masque-2',
        use_http2: false,
        use_ipv6: false,
        profile: { recreate: false },
      },
    ]);
    expect(next.experimental).toEqual({
      cache_file: {
        path: '/data/cache.db',
        store_fakeip: true,
        enabled: true,
        store_masque_config: true,
      },
    });
    expect(config.outbounds).toHaveLength(1);
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
