import { describe, expect, it } from 'vitest';
import {
  isNativeEndpoint,
  nativeConnectionLink,
  nativeEndpointTypes,
  nativeOutboundTypes,
  renameImportedConnections,
  rewriteNativeReferences,
} from '@/pages/singbox/native-import';
import { outboundErrors } from '@/pages/singbox/simple-settings';
import { parseOutboundJson, strings } from '@/pages/singbox/outbound-editor-model';
describe('complete native import model', () => {
  it.each([...nativeOutboundTypes, ...nativeEndpointTypes])(
    'supports lossless JSON and UTF-8 share links for %s',
    (type) => {
      const native = {
        type,
        tag: 'Тест',
        future_option: { password: 'a:b@c', enabled: true },
        final_mask: { tcp: [{ type: 'fragment', settings: { packets: 'tlshello' } }] },
      };
      const link = nativeConnectionLink(native);
      const decoded = JSON.parse(
        new TextDecoder().decode(
          Uint8Array.from(atob(link.slice(10).replace(/-/g, '+').replace(/_/g, '/')), (c) =>
            c.charCodeAt(0),
          ),
        ),
      );
      expect(decoded).toEqual(native);
      expect(parseOutboundJson(JSON.stringify(decoded))).toEqual(native);
      expect(isNativeEndpoint(native)).toBe(nativeEndpointTypes.includes(type));
    },
  );
  it('renames batch nodes together with detours and group references', () => {
    const input = [
      { type: 'direct', tag: 'exit' },
      { type: 'vless', tag: 'proxy', detour: 'exit' },
      { type: 'selector', tag: 'group', outbounds: ['proxy'], default: 'proxy' },
    ];
    expect(renameImportedConnections(input, ['exit', 'proxy'])).toEqual([
      { type: 'direct', tag: 'exit-2' },
      { type: 'vless', tag: 'proxy-2', detour: 'exit-2' },
      { type: 'selector', tag: 'group', outbounds: ['proxy-2'], default: 'proxy-2' },
    ]);
    expect(input[1].detour).toBe('exit');
  });
  it('retags nested download dependencies while leaving HTTP headers unchanged', () => {
    expect(
      rewriteNativeReferences(
        {
          transport: { download: { detour: 'exit', headers: { detour: 'exit' } } },
          route: { final: 'exit' },
        },
        new Map([['exit', 'exit-2']]),
      ),
    ).toEqual({
      transport: { download: { detour: 'exit-2', headers: { detour: 'exit' } } },
      route: { final: 'exit-2' },
    });
  });
  it('accepts kernel Listable strings and protobuf enum names', () => {
    expect(strings('h2')).toEqual(['h2']);
    expect(
      outboundErrors(
        {
          type: 'hysteria2',
          tag: 'proxy',
          server: 'host',
          server_ports: '10000:10010',
          password: 'pass',
          tls: { enabled: true },
        },
        [],
      ),
    ).toEqual([]);
    expect(
      outboundErrors(
        {
          type: 'mieru',
          tag: 'proxy',
          server: 'host',
          username: 'user',
          password: 'pass',
          multiplexing: 'MULTIPLEXING_HIGH',
          handshake_mode: 'HANDSHAKE_NO_WAIT',
          portBindings: [{ protocol: 'TCP', port: 443 }],
        },
        [],
      ),
    ).toEqual([]);
  });
  it('allows Vision backed by core-supported VLESS encryption', () => {
    expect(
      outboundErrors(
        {
          type: 'vless',
          tag: 'proxy',
          server: 'host',
          server_port: 443,
          uuid: '12345678-1234-1234-1234-123456789abc',
          flow: 'xtls-rprx-vision',
          encryption: 'mlkem768x25519plus.native.0rtt.KEY',
        },
        [],
      ),
    ).toEqual([]);
  });
});
