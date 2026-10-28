import { describe, expect, it } from 'vitest';
import { DBInbound } from '@/models/dbinbound';
import { buildRowActionsMenu } from '@/pages/inbounds/list/RowActions';
import { isInboundMultiUser } from '@/pages/inbounds/list/helpers';

describe('inbound client actions', () => {
  it.each([
    'hysteria',
    'naive',
    'anytls',
    'shadowtls',
    'mieru',
    'sudoku',
    'vless',
    'wireguard',
    'tuic',
    'masque',
    'snell',
    'mtproto',
    'amneziawg',
    'fptn',
    'openflux',
    'trusttunnel',
    'vk-turn-proxy',
  ])('exposes bulk client actions for %s', (protocol) => {
    const record = new DBInbound({ id: 1, protocol, settings: '{}' });
    const items = buildRowActionsMenu({
      record,
      subEnable: true,
      hasClients: true,
      t: (key) => key,
    });
    const keys = items?.map((item) => (item && 'key' in item ? item.key : undefined));
    expect(keys).toEqual(
      expect.arrayContaining([
        'attachExisting',
        'attachClients',
        'detachClients',
        'addToGroup',
        'delAllClients',
        'export',
        'subs',
      ]),
    );
    const empty = buildRowActionsMenu({
      record,
      subEnable: true,
      hasClients: false,
      t: (key) => key,
    });
    expect(empty?.some((item) => item && 'key' in item && item.key === 'attachExisting')).toBe(
      true,
    );
  });
  it.each(['http', 'mixed', 'tun', 'tunnel', 'pingtunnel'])(
    'keeps listener/account protocols out of shared client actions: %s',
    (protocol) => {
      expect(isInboundMultiUser({ protocol, settings: {} })).toBe(false);
    },
  );
  it('preserves Shadowsocks single-user restrictions', () => {
    expect(
      isInboundMultiUser({
        protocol: 'shadowsocks',
        settings: { method: '2022-blake3-chacha20-poly1305' },
      }),
    ).toBe(false);
  });
});
