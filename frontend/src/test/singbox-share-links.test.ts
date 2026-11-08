import { execFileSync } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { parseShareLink, uniqueTag } from '@/pages/singbox/share-links';
import { outboundErrors } from '@/pages/singbox/simple-settings';

const uuid = '12345678-1234-1234-1234-123456789abc';
describe('sing-box share link import', () => {
  it('enables TLS for a standard Trojan link and decodes the complete password', () => {
    expect(parseShareLink('trojan://hello%3Aworld@proxy.test#Test')).toMatchObject({
      password: 'hello:world',
      tls: { enabled: true, server_name: 'proxy.test' },
    });
    expect(parseShareLink('trojan://hello:world@proxy.test')).toHaveProperty(
      'password',
      'hello:world',
    );
  });
  it.each([
    ['http', 80],
    ['https', 443],
    ['socks', 1080],
    ['socks5', 1080],
  ])('uses the default %s port', (scheme, port) => {
    expect(parseShareLink(`${scheme}://user:pass@proxy.test`)).toHaveProperty('server_port', port);
  });
  it('preserves explicit ports and IPv6 addresses', () => {
    expect(parseShareLink('socks5://[2001:db8::1]:9000')).toMatchObject({
      server: '2001:db8::1',
      server_port: 9000,
    });
  });
  it('imports Trojan WebSocket and TLS options', () => {
    expect(
      parseShareLink(
        'trojan://pass@proxy.test?type=ws&path=%2Fsocket&host=cdn.test&sni=tls.test&alpn=h2&fp=chrome',
      ),
    ).toMatchObject({
      transport: { type: 'ws', path: '/socket', headers: { Host: 'cdn.test' } },
      tls: {
        enabled: true,
        server_name: 'tls.test',
        alpn: ['h2'],
        utls: { enabled: true, fingerprint: 'chrome' },
      },
    });
  });
  it('uses the native HTTP host list and gRPC service name without leaking HTTP fields', () => {
    expect(
      parseShareLink(`vless://${uuid}@proxy.test?type=http&host=a.test,b.test`),
    ).toHaveProperty('transport.host', ['a.test', 'b.test']);
    expect(
      parseShareLink(`vless://${uuid}@proxy.test?type=grpc&serviceName=service&path=/bad&host=bad`),
    ).toHaveProperty('transport', { type: 'grpc', service_name: 'service' });
  });
  it('imports Reality even when the optional short ID is empty', () => {
    expect(
      parseShareLink(`vless://${uuid}@proxy.test?security=reality&pbk=key&sid=`),
    ).toHaveProperty('tls.reality', { enabled: true, public_key: 'key', short_id: '' });
  });
  it.each(['type=xhttp', 'type=kcp', 'security=reality', 'type=tcp&headerType=http'])(
    'rejects unsupported or incomplete settings: %s',
    (query) => {
      expect(() => parseShareLink(`vless://${uuid}@proxy.test?${query}`)).toThrow();
    },
  );
  it('avoids tag collisions with imported names and endpoints', () => {
    expect(uniqueTag('proxy', ['proxy', 'proxy-2', 'proxy-3'])).toBe('proxy-4');
  });
});

describe('outbound validation', () => {
  it('checks TUIC UUID, port and TLS while allowing an empty password', () => {
    expect(
      outboundErrors(
        {
          type: 'tuic',
          tag: 'test',
          server: 'proxy.test',
          server_port: 0,
          tls: { enabled: false },
        },
        [],
      ),
    ).toHaveLength(3);
    expect(
      outboundErrors(
        {
          type: 'tuic',
          tag: 'test',
          server: 'proxy.test',
          server_port: 443,
          uuid,
          password: '',
          tls: { enabled: true },
        },
        [],
      ),
    ).toEqual([]);
  });
  it('rejects duplicate tags and empty or self-referencing groups', () => {
    expect(outboundErrors({ type: 'direct', tag: 'proxy' }, ['proxy'])).toHaveLength(1);
    expect(
      outboundErrors({ type: 'selector', tag: 'group', outbounds: [] }, ['proxy']),
    ).toHaveLength(1);
    expect(
      outboundErrors({ type: 'selector', tag: 'group', outbounds: ['group'] }, ['proxy']),
    ).toHaveLength(1);
  });
});

it('imports HY2 auth, IPv6, hopping and Gecko without a conflicting server_port', () => {
  const outbound = parseShareLink(
    'hy2://user:p%40ss@[2001:db8::1]:443,10000-10010?obfs=gecko&obfs-password=secret&obfs-min-packet-size=20&obfs-max-packet-size=100&sni=tls.test#HY2',
  );
  expect(outbound).toMatchObject({
    type: 'hysteria2',
    tag: 'HY2',
    server: '2001:db8::1',
    password: 'user:p@ss',
    server_ports: ['443:443', '10000:10010'],
    obfs: { type: 'gecko', password: 'secret', min_packet_size: 20, max_packet_size: 100 },
    tls: { enabled: true, server_name: 'tls.test' },
  });
  expect(outbound.server_port).toBeUndefined();
  expect(outboundErrors(outbound, [])).toEqual([]);
});
it('imports default HY2 port and certificate pin', () => {
  const outbound = parseShareLink(`hysteria2://pass@proxy.test?pinSHA256=${'00'.repeat(32)}`);
  expect(outbound.server_port).toBe(443);
  expect(outbound.tls).toMatchObject({ certificate_sha256: [btoa('\0'.repeat(32))] });
});
it.each([
  'hy2://@host',
  'hy2://p@host:90000',
  'hy2://p@host:9000-8000',
  'hy2://p@host?obfs=unknown',
  'hy2://p@host?obfs=gecko',
  'hy2://p@host?pinSHA256=bad',
  'hy2://p@host?obfs=gecko&obfs-password=x&obfs-min-packet-size=3000',
])('rejects invalid HY2 options: %s', (link) => {
  expect(() => parseShareLink(link)).toThrow();
});
it('validates Mieru bindings without requiring server_port', () => {
  expect(
    outboundErrors(
      {
        type: 'mieru',
        tag: 'm',
        server: 'host',
        username: 'u',
        password: 'p',
        portBindings: [{ protocol: 'TCP', portRange: '2012-2022' }],
      },
      [],
    ),
  ).toEqual([]);
  expect(
    outboundErrors(
      {
        type: 'mieru',
        tag: 'm',
        server: 'host',
        username: 'u',
        password: 'p',
        server_port: 443,
        portBindings: [{ protocol: 'TCP', portRange: '2012-2022' }],
      },
      [],
    ).length,
  ).toBeGreaterThan(0);
});

it.skipIf(!process.env.HIDDIFY_CONFIG_CHECK_BINARY)(
  'checks actual imported HY2 JSON with the pinned owner core',
  () => {
    const dir = mkdtempSync(join(tmpdir(), 'hy2-import-'));
    try {
      for (const link of [
        'hy2://pass@proxy.test',
        'hy2://pass@proxy.test:443,10000-10010?obfs=gecko&obfs-password=x&obfs-min-packet-size=20&obfs-max-packet-size=100',
        `hy2://pass@proxy.test?pinSHA256=${'00'.repeat(32)}`,
      ]) {
        const file = join(dir, 'config.json');
        writeFileSync(file, JSON.stringify({ outbounds: [parseShareLink(link)] }));
        expect(() =>
          execFileSync(process.env.HIDDIFY_CONFIG_CHECK_BINARY!, ['check', '-c', file], {
            timeout: 15000,
          }),
        ).not.toThrow();
      }
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  },
);

it('accepts panel-exported HY2 security/fingerprint hints and Gecko aliases', () => {
  expect(
    parseShareLink(
      'hysteria2://p@host:443?security=tls&fp=chrome&obfs=gecko&obfs-password=x&minPacketSize=100&maxPacketSize=200',
    ),
  ).toMatchObject({ obfs: { type: 'gecko', min_packet_size: 100, max_packet_size: 200 } });
  expect(() => parseShareLink('hy2://p@host?security=none')).toThrow();
  expect(() => parseShareLink('hy2://p@host?vcn=different')).toThrow();
  expect(() =>
    parseShareLink(
      'hy2://p@host?obfs=gecko&obfs-password=x&minPacketSize=100&obfs-min-packet-size=101',
    ),
  ).toThrow();
});
