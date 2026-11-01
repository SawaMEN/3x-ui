import { describe, expect, it } from 'vitest';
import { inflateSync } from 'node:zlib';
import {
  amneziaProfile,
  amneziaQrChunks,
  buildAmneziaVpnProfile,
} from '@/pages/clients/amneziaVpnProfile';
import { awgVersion, generateAwgPreset } from '@/lib/xray/amneziawg-obfuscation';
import { buildAmneziaWGClientConfig } from '@/pages/clients/amneziawgConfig';
import { genAmneziaWGConfig } from '@/lib/xray/inbound-link';
import { AmneziawgServerSchema } from '@/schemas/protocols/inbound/amneziawg';

const conf = `[Interface]\nPrivateKey = client==\nAddress = 10.8.1.2/32\nDNS = 1.1.1.1, 8.8.8.8\nMTU = 1392\nJc = 4\nH1 = 123\nI1 = <r 64>\nHeaderProtectionKey = protection==\n[Peer]\nPublicKey = server==\nPresharedKey = psk==\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = [2001:db8::1]:51820\nPersistentKeepalive = 25`;

describe('AmneziaVPN export', () => {
  it('preserves endpoint, identity, MTU and AWG options without administrative credentials', () => {
    const profile = amneziaProfile(conf, 'Сервер');
    expect(profile.hostName).toBe('2001:db8::1');
    expect(profile.description).toBe('Сервер');
    const client = JSON.parse(profile.containers[0].awg.last_config);
    expect(client).toMatchObject({
      client_priv_key: 'client==',
      server_pub_key: 'server==',
      client_ip: '10.8.1.2/32',
      mtu: '1392',
      psk_key: 'psk==',
      I1: '<r 64>',
      HeaderProtectionKey: 'protection==',
      port: 51820,
    });
    expect(profile).not.toHaveProperty('userName');
    expect(profile).not.toHaveProperty('password');
    expect(profile).not.toHaveProperty('port');
    expect(client.config).toBe(conf);
  });

  it('produces Qt-compatible zlib framing and QR parts that reconstruct the identical profile', async () => {
    const result = await buildAmneziaVpnProfile(conf, 'Test');
    const wire = Buffer.from(result.link.slice(6), 'base64url');
    const json = wire[0] === 123 ? wire : inflateSync(wire.subarray(4));
    if (wire[0] !== 123) expect(wire.readUInt32BE(0)).toBe(json.length);
    expect(JSON.parse(json.toString())).toEqual(amneziaProfile(conf, 'Test'));
    const chunks = result.qr.map((qr, index) => {
      const part = Buffer.from(qr, 'base64url');
      expect(part.readInt16BE(0)).toBe(1984);
      expect(part[2]).toBe(result.qr.length);
      expect(part[3]).toBe(index);
      expect(part.readUInt32BE(4)).toBe(part.length - 8);
      return part.subarray(8);
    });
    expect(Buffer.concat(chunks)).toEqual(wire);
  });

  it('splits large profiles without losing bytes', () => {
    const bytes = Uint8Array.from({ length: 2800 }, (_, i) => i % 256);
    const chunks = amneziaQrChunks(bytes);
    expect(chunks).toHaveLength(4);
    expect(Buffer.concat(chunks.map((s) => Buffer.from(s, 'base64url').subarray(8)))).toEqual(
      Buffer.from(bytes),
    );
  });
});

describe('AWG compatibility and client export', () => {
  it('exports a bracketed IPv6 endpoint accepted by the Amnezia profile decoder', () => {
    const server = AmneziawgServerSchema.parse(generateAwgPreset('2.0'));
    server.publicKey = 'server';
    const config = buildAmneziaWGClientConfig(
      { email: 'peer', privateKey: 'client', allowedIPs: '10.8.1.2/32' },
      { id: 1, tag: 'awg', protocol: 'amneziawg', port: 51820, awgServer: server },
      '2001:db8::1',
    );
    expect(config).toContain('Endpoint = [2001:db8::1]:51820');
    expect(amneziaProfile(config, 'IPv6').hostName).toBe('2001:db8::1');
  });

  it.each(['1.0', '1.5', '2.0', '3.0', '3.1'] as const)(
    'generates a %s preset with the expected minimum generation',
    (version) => {
      const preset = generateAwgPreset(version);
      expect(awgVersion(preset)).toBe(version);
      expect(AmneziawgServerSchema.safeParse(preset).success).toBe(true);
    },
  );

  it('applies local overrides in both frontend emitters without mutating server settings', () => {
    const server = AmneziawgServerSchema.parse(generateAwgPreset('3.1'));
    const before = structuredClone(server);
    const options = { jc: 0, jmin: 80, jmax: 100, i1: '', mtu: 1380 };
    const client = {
      email: 'peer',
      privateKey: 'priv',
      allowedIPs: '10.8.1.2/32',
      awgClientOptions: options,
    };
    const text = buildAmneziaWGClientConfig(
      client,
      { id: 1, tag: 'awg', port: 51820, protocol: 'amneziawg', awgServer: server },
      'example.com',
    );
    const other = genAmneziaWGConfig({
      settings: {
        server,
        clients: [
          {
            ...client,
            allowedIPs: ['10.8.1.2/32'],
            enable: true,
            subId: '',
            tgId: 0,
            limitIp: 0,
            totalGB: 0,
            expiryTime: 0,
            comment: '',
            reset: 0,
            forwardedPorts: '',
          },
        ],
      },
      address: 'example.com',
      port: 51820,
      peerIndex: 0,
    });
    for (const config of [text, other]) {
      expect(config).toContain('Jc = 0');
      expect(config).toContain('Jmin = 80');
      expect(config).toContain('MTU = 1380');
      expect(config).not.toContain('I1 =');
      expect(config).toContain(`HeaderProtectionKey = ${server.headerProtectionKey}`);
    }
    expect(server).toEqual(before);
  });
});
