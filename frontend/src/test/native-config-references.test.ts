import { describe, expect, it } from 'vitest';
import {
  putConnection,
  removeConnection,
  replaceReferences,
  referencePaths,
} from '@/pages/singbox/config-references';
import {
  buildDnsServer,
  dnsServerForm,
  putDnsServer,
  removeDnsServer,
} from '@/pages/singbox/dns-editor';

const config = {
  outbounds: [
    { type: 'direct', tag: 'shared' },
    { type: 'selector', tag: 'select', default: 'shared', outbounds: ['shared', 'tunnel'] },
  ],
  endpoints: [{ type: 'wireguard', tag: 'tunnel', detour: 'shared', private_key: 'keep' }],
  route: {
    final: 'shared',
    rules: [
      { type: 'logical', rules: [{ domain: ['shared'] }], action: 'route', outbound: 'shared' },
    ],
    default_domain_resolver: { server: 'shared', strategy: 'ipv4_only' },
  },
  dns: {
    final: 'shared',
    servers: [
      {
        type: 'https',
        tag: 'shared',
        server: 'shared',
        detour: 'shared',
        tls: { enabled: true, server_name: 'shared' },
      },
    ],
    rules: [{ action: 'route', server: 'shared', domain: ['shared'] }],
  },
};
describe('native configuration references', () => {
  it('updates route, DNS detours and group members on an outbound rename without changing DNS server tags or credentials', () => {
    const next = putConnection(config, 0, { type: 'direct', tag: 'renamed' });
    expect(referencePaths(next, 'connection', 'shared')).toEqual([]);
    expect(next.route).toMatchObject({
      final: 'renamed',
      rules: [{ outbound: 'renamed' }],
      default_domain_resolver: { server: 'shared' },
    });
    expect(next.dns).toMatchObject({
      final: 'shared',
      servers: [
        { tag: 'shared', server: 'shared', detour: 'renamed', tls: { server_name: 'shared' } },
      ],
    });
    expect(config.route.final).toBe('shared');
  });
  it('edits an endpoint at its combined list position and moves it between native collections', () => {
    const edited = putConnection(config, 2, { ...config.endpoints[0], tag: 'new-tunnel' });
    expect(edited.endpoints).toMatchObject([{ tag: 'new-tunnel', private_key: 'keep' }]);
    expect(edited.outbounds).toMatchObject([
      { tag: 'shared' },
      { outbounds: ['shared', 'new-tunnel'] },
    ]);
    const moved = putConnection(edited, 2, {
      type: 'socks',
      tag: 'new-tunnel',
      server: 'proxy.test',
      server_port: 1080,
    });
    expect(moved.endpoints).toEqual([]);
    expect(moved.outbounds).toHaveLength(3);
  });
  it('blocks deletion of referenced connections and duplicate names', () => {
    expect(() => removeConnection(config, 0)).toThrow(/route.final/);
    expect(() => putConnection(config, null, { type: 'direct', tag: 'shared' })).toThrow();
    expect(
      removeConnection({ outbounds: [{ type: 'direct', tag: 'unused' }] }, 0).outbounds,
    ).toEqual([]);
  });
  it('renames DNS references without touching hostnames or outbound targets with the same name', () => {
    const next = putDnsServer(config, 0, { ...config.dns.servers[0], tag: 'new-dns' });
    expect(referencePaths(next, 'dns', 'shared')).toEqual([]);
    expect(next.route).toMatchObject({
      final: 'shared',
      default_domain_resolver: { server: 'new-dns', strategy: 'ipv4_only' },
    });
    expect(next.dns).toMatchObject({
      final: 'new-dns',
      servers: [{ tag: 'new-dns', server: 'shared', detour: 'shared' }],
      rules: [{ server: 'new-dns', domain: ['shared'] }],
    });
    expect(() => removeDnsServer(config, 0)).toThrow(/dns.final/);
  });
  it('does not rewrite literal values in HTTP headers', () => {
    expect(
      replaceReferences(
        { headers: { outbound: 'shared', detour: 'shared' } },
        'connection',
        'shared',
        'new',
      ),
    ).toEqual({ headers: { outbound: 'shared', detour: 'shared' } });
  });
});

describe('DNS server form', () => {
  it('parses a pasted DoH URL and enables verified TLS', () => {
    const value = { type: 'udp', tag: 'dns', server: '1.1.1.1', server_port: 53 };
    expect(
      buildDnsServer(value, {
        ...dnsServerForm(value),
        server: 'https://dns.test:8443/custom-query',
      }),
    ).toMatchObject({
      type: 'https',
      tag: 'dns',
      server: 'dns.test',
      server_port: 8443,
      path: '/custom-query',
      tls: { enabled: true },
    });
  });
  it('preserves custom TLS, resolver and transport options while changing the tag', () => {
    const value = {
      type: 'https',
      tag: 'dns',
      server: 'dns.test',
      path: '/dns-query',
      tls: { enabled: true, server_name: 'dns.test', certificate_path: ['keep.pem'] },
      domain_resolver: { server: 'bootstrap', strategy: 'ipv4_only' },
    };
    expect(buildDnsServer(value, { ...dnsServerForm(value), tag: 'new' })).toMatchObject({
      ...value,
      tag: 'new',
    });
  });
  it('rejects incomplete remote servers and clears incompatible fields when changing type', () => {
    const value = { type: 'https', tag: 'dns', server: 'dns.test', tls: { enabled: true } };
    expect(() => buildDnsServer({}, { ...dnsServerForm(value), server: '' })).toThrow();
    expect(buildDnsServer(value, { ...dnsServerForm(value), type: 'local' })).toEqual({
      type: 'local',
      tag: 'dns',
    });
  });
});

it('replaces all connection dependencies before removing a used outbound', () => {
  const next = removeConnection(config, 0, 'tunnel');
  expect(referencePaths(next, 'connection', 'shared')).toEqual([]);
  expect(next.route).toMatchObject({ final: 'tunnel' });
  expect(next.dns).toMatchObject({ final: 'shared' });
  expect(config.outbounds).toHaveLength(2);
});

it('rejects duplicate imported connection tags before changing the draft', () => {
  const cfg = { outbounds: [{ type: 'direct', tag: 'direct' }] };
  expect(() =>
    putConnection(cfg, null, { type: 'selector', tag: 'proxy' }, [
      { type: 'socks', tag: 'direct' },
    ]),
  ).toThrow();
  expect(cfg.outbounds).toHaveLength(1);
});
