import { describe, expect, it } from 'vitest';
import {
  changeProtocol,
  changeSecurity,
  changeServer,
  changeTransport,
  parseOutboundJson,
} from '@/pages/singbox/outbound-editor-model';
import { outboundDefaults, outboundErrors } from '@/pages/singbox/simple-settings';
describe('native outbound editor synchronization', () => {
  it('tracks automatic SNI but preserves an explicit SNI', () => {
    const value = {
      type: 'vless',
      server: 'old.test',
      tls: { enabled: true, server_name: 'old.test' },
    };
    expect(changeServer(value, 'new.test')).toHaveProperty('tls.server_name', 'new.test');
    expect(
      changeServer({ ...value, tls: { enabled: true, server_name: 'cdn.test' } }, 'new.test'),
    ).toHaveProperty('tls.server_name', 'cdn.test');
  });
  it('enables Reality with uTLS and removes conflicting certificate bypass', () => {
    expect(changeSecurity({ server: 'host', tls: { insecure: true } }, 'reality')).toEqual({
      server: 'host',
      tls: {
        enabled: true,
        server_name: 'host',
        reality: { enabled: true },
        utls: { enabled: true, fingerprint: 'chrome' },
      },
    });
    expect(changeSecurity({ flow: 'xtls-rprx-vision', tls: { enabled: true } }, 'none')).toEqual(
      {},
    );
  });
  it('removes TCP-only options on transport change and maps HTTP hosts correctly', () => {
    const value = {
      flow: 'xtls-rprx-vision',
      transport: { type: 'ws', headers: { Host: 'cdn.test' }, path: '/ws', max_early_data: 2048 },
    };
    expect(changeTransport(value, 'http')).toEqual({
      transport: { type: 'http', path: '/ws', host: ['cdn.test'] },
    });
    expect(changeTransport(value, 'tcp')).toEqual({});
  });
  it('keeps server and identity but drops protocol-incompatible fields', () => {
    expect(
      changeProtocol(
        {
          type: 'vless',
          tag: 'proxy',
          server: 'host',
          server_port: 8443,
          uuid: 'uuid',
          flow: 'vision',
          tls: { enabled: true, reality: { enabled: true }, utls: { enabled: true } },
        },
        'tuic',
      ),
    ).toMatchObject({
      type: 'tuic',
      tag: 'proxy',
      server: 'host',
      server_port: 8443,
      uuid: 'uuid',
      tls: { enabled: true },
    });
    const next = changeProtocol(
      {
        type: 'vless',
        tls: { enabled: true, utls: { enabled: true }, reality: { enabled: true } },
      },
      'tuic',
    );
    expect(next).not.toHaveProperty('tls.utls');
    expect(next).not.toHaveProperty('tls.reality');
    expect(next).not.toHaveProperty('flow');
  });
  it('preserves arbitrary advanced JSON fields and rejects non-object input', () => {
    expect(parseOutboundJson('{"type":"anytls","idle_session_timeout":"30s"}')).toHaveProperty(
      'idle_session_timeout',
      '30s',
    );
    expect(() => parseOutboundJson('[]')).toThrow();
    expect(() => parseOutboundJson('{}')).toThrow();
  });
  it.each(['anytls', 'hysteria', 'hysteria2', 'tuic', 'shadowtls', 'naive'])(
    'enables required TLS for %s',
    (type) => {
      expect(outboundDefaults(type, 'proxy')).toHaveProperty('tls.enabled', true);
    },
  );
  it('validates new protocol requirements and invalid JSON combinations', () => {
    expect(
      outboundErrors(
        { type: 'anytls', tag: 'proxy', server: 'host', server_port: 443, tls: { enabled: true } },
        [],
      ),
    ).toContain('Укажите пароль.');
    expect(
      outboundErrors(
        {
          type: 'vless',
          tag: 'proxy',
          uuid: '12345678-1234-1234-1234-123456789abc',
          server: 'host',
          server_port: 443,
          flow: 'xtls-rprx-vision',
          transport: { type: 'ws' },
        },
        [],
      ),
    ).toContain('Vision требует TLS, Reality или шифрование VLESS.');
  });
});
