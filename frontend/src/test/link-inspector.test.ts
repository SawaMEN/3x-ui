import { describe, expect, it } from 'vitest';

import { inspectProxyLink } from '@/lib/xray/link-inspector';

describe('inspectProxyLink', () => {
  it('parses VLESS Reality links', () => {
    const result = inspectProxyLink(
      'vless://11111111-1111-4111-8111-111111111111@vpn.example.com:443?type=tcp&security=reality&sni=www.example.com&fp=chrome&pbk=public-key&sid=abcd#Reality',
    );

    expect(result.protocol).toBe('vless');
    expect(result.host).toBe('vpn.example.com');
    expect(result.port).toBe(443);
    expect(result.network).toBe('tcp');
    expect(result.security).toBe('reality');
    expect(result.sni).toBe('www.example.com');
    expect(result.fingerprint).toBe('chrome');
    expect(result.publicKey).toBe('public-key');
    expect(result.shortId).toBe('abcd');
    expect(result.name).toBe('Reality');
    expect(result.issues).toEqual([]);
  });

  it('parses VMess JSON links', () => {
    const result = inspectProxyLink(
      'vmess://eyJ2IjoiMiIsInBzIjoiVk1lc3MgVGVzdCIsImFkZCI6InZwbi5leGFtcGxlLmNvbSIsInBvcnQiOiI0NDMiLCJpZCI6IjExMTExMTExLTExMTEtNDExMS04MTExLTExMTExMTExMTExMSIsImFpZCI6IjAiLCJzY3kiOiJhdXRvIiwibmV0Ijoid3MiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJjZG4uZXhhbXBsZS5jb20iLCJwYXRoIjoiL3dzIiwidGxzIjoidGxzIiwic25pIjoidnBuLmV4YW1wbGUuY29tIiwiZnAiOiJjaHJvbWUifQ==',
    );

    expect(result.protocol).toBe('vmess');
    expect(result.name).toBe('VMess Test');
    expect(result.host).toBe('vpn.example.com');
    expect(result.port).toBe(443);
    expect(result.network).toBe('ws');
    expect(result.hostHeader).toBe('cdn.example.com');
    expect(result.path).toBe('/ws');
    expect(result.security).toBe('tls');
    expect(result.issues).toEqual([]);
  });

  it('parses SIP002 Shadowsocks links', () => {
    const result = inspectProxyLink(
      'ss://MjAyMi1ibGFrZTMtYWVzLTEyOC1nY206c2VjcmV0@ss.example.com:8443/?plugin=v2ray-plugin%3Btls#SS',
    );

    expect(result.protocol).toBe('ss');
    expect(result.host).toBe('ss.example.com');
    expect(result.port).toBe(8443);
    expect(result.username).toBe('2022-blake3-aes-128-gcm');
    expect(result.password).toBe('secret');
    expect(result.params.plugin).toBe('v2ray-plugin;tls');
    expect(result.name).toBe('SS');
    expect(result.issues).toEqual([]);
  });

  it('reports semantic VLESS validation warnings without rejecting the URI', () => {
    const result = inspectProxyLink('vless://not-a-uuid@example.com:443?security=tls');

    expect(result.protocol).toBe('vless');
    expect(result.issues).toContain('VLESS credential is not a valid UUID');
  });

  it('rejects unsupported URI schemes', () => {
    expect(() => inspectProxyLink('hysteria2://secret@example.com:443')).toThrow(
      'Unsupported proxy URI scheme: hysteria2',
    );
  });
});
