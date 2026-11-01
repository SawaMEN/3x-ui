import { describe, expect, it } from 'vitest';
import registry from '@/pages/singbox/native-outbound-fields.json';
import { dedicatedFields } from '@/pages/singbox/ProtocolOptionFields';
import { autoConfigureOutbound } from '@/pages/singbox/outbound-automation';
import {
  optionVisible,
  patchOption,
  type OptionSchema,
} from '@/pages/singbox/protocol-option-model';
import { nativeEndpointTypes, nativeOutboundTypes } from '@/pages/singbox/native-import';

describe('automatic native protocol configuration', () => {
  it.each([...nativeOutboundTypes, ...nativeEndpointTypes])(
    'keeps %s configuration idempotent and preserves imported secrets',
    (type) => {
      const original = {
        type,
        tag: 'proxy',
        server: 'example.com',
        private_key: 'server-owned',
        custom: { preserve: true },
        tls: { enabled: true, server_name: 'explicit.test' },
      };
      const configured = autoConfigureOutbound(original);
      expect(autoConfigureOutbound(configured)).toEqual(configured);
      expect(original).not.toHaveProperty('server_port');
      expect(configured).toMatchObject({
        type,
        tag: 'proxy',
        private_key: 'server-owned',
        custom: { preserve: true },
        tls: { server_name: 'explicit.test' },
      });
    },
  );
  it('preserves Hysteria2 port hopping without adding a conflicting server_port', () => {
    const configured = autoConfigureOutbound({
      type: 'hysteria2',
      tag: 'hy2',
      server_ports: ['10000:10100'],
    });
    expect(configured).not.toHaveProperty('server_port');
    expect(configured.server_ports).toEqual(['10000:10100']);
  });
  it('derives Mieru bindings from a custom imported server port', () => {
    expect(autoConfigureOutbound({ type: 'mieru', server_port: 8443 })).toHaveProperty(
      'portBindings',
      [{ protocol: 'TCP', port: 8443 }],
    );
  });
  it('renders each Snell field only once; network and reuse stay advanced', () => {
    const dedicated = dedicatedFields('snell');
    for (const field of ['psk', 'userkey', 'version', 'mode', 'obfs_mode', 'obfs_host']) {
      expect(dedicated.has(field)).toBe(true);
    }
    expect(dedicated.has('network')).toBe(false);
    expect(dedicated.has('reuse')).toBe(false);
    expect(registry.protocols.snell.network.kind).toBe('network');
  });

  it('cleans only incompatible version-dependent fields', () => {
    const schema = registry.protocols.snell as OptionSchema;
    const configured = patchOption(
      { version: 4, obfs_host: 'host', obfs_mode: 'http', custom: 'keep' },
      'version',
      6,
      schema,
    );
    expect(configured).toEqual({ version: 6, custom: 'keep' });
    expect(optionVisible(schema.mode, configured)).toBe(true);
    expect(optionVisible(schema.obfs_mode, configured)).toBe(false);
    for (const version of [4, 5]) {
      expect(
        patchOption({ ...configured, mode: 'unshaped' }, 'version', version, schema),
      ).not.toHaveProperty('mode');
    }
    expect(registry.protocols.snell.version.enum).toEqual([4, 5, 6]);
    expect(
      autoConfigureOutbound({ type: 'snell', version: 5, psk: 'existing-server-key' }),
    ).toMatchObject({ version: 5, psk: 'existing-server-key' });
  });
  it('contains structured controls for protocol-specific lists and nested options', () => {
    expect(registry.protocols.wireguard.peers).toMatchObject({
      kind: 'array',
      item: { kind: 'object', ref: 'WireGuardPeer' },
    });
    expect(registry.definitions.AwgOptions).toHaveProperty('jc');
    expect(registry.definitions.MieruPortBinding.protocol.enum).toEqual(['TCP', 'UDP']);
    expect(registry.definitions.OpenFluxTransportOptions.type.enum).toContain('direct');
    expect(registry.protocols.mieru.multiplexing.enum).not.toContain('MULTIPLEXING_OFF');
  });
});
