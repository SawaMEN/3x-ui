import { describe, expect, it } from 'vitest';
import {
  generateOutboundCredentials,
  outboundCredentialSummary,
} from '@/pages/singbox/outbound-credentials';
import { autoConfigureOutbound } from '@/pages/singbox/outbound-automation';
const decode = (value: unknown) =>
  Uint8Array.from(atob(String(value)), (char) => char.charCodeAt(0));
const hex = (value: Uint8Array) =>
  Array.from(value, (byte) => byte.toString(16).padStart(2, '0')).join('');
describe('protocol credential generation', () => {
  it.each([
    ['2022-blake3-aes-128-gcm', 16],
    ['2022-blake3-aes-256-gcm', 32],
    ['2022-blake3-chacha20-poly1305', 32],
  ] as const)('generates a %s key of the required size', (method, size) => {
    expect(
      decode(generateOutboundCredentials({ type: 'shadowsocks', method }).password),
    ).toHaveLength(size);
  });
  it('uses valid UUIDs and does not overwrite existing authentication', () => {
    const result = generateOutboundCredentials({ type: 'tuic', password: 'existing' });
    expect(result.uuid).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
    );
    expect(result.password).toBe('existing');
    expect(generateOutboundCredentials(result)).toEqual(result);
  });
  it.each(['wireguard', 'wireguard_legacy', 'awg', 'tailcat'])(
    'generates a clamped %s private key and exports its public counterpart separately',
    (type) => {
      const generated = generateOutboundCredentials({ type });
      const key = decode(generated.private_key);
      expect(key).toHaveLength(32);
      expect(key[0] & 7).toBe(0);
      expect(key[31] & 128).toBe(0);
      expect(key[31] & 64).toBe(64);
      expect(decode(outboundCredentialSummary(generated).public_key)).toHaveLength(32);
      expect(generated).not.toHaveProperty('public_key');
    },
  );
  it('derives the public key matching the RFC 7748 Alice test vector', () => {
    const raw = '77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a';
    const private_key = btoa(
      String.fromCharCode(...raw.match(/../g)!.map((byte) => parseInt(byte, 16))),
    );
    expect(
      hex(decode(outboundCredentialSummary({ type: 'wireguard', private_key }).public_key)),
    ).toBe('8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a');
  });
  it('generates a canonical Sudoku private scalar accepted by the native parser', () => {
    const order = 2n ** 252n + 27742317777372353535851937790883648493n;
    for (let index = 0; index < 12; index++) {
      const key = String(generateOutboundCredentials({ type: 'sudoku' }).key);
      expect(key).toMatch(/^[0-9a-f]{64}$/);
      const scalar = BigInt(`0x${key.match(/../g)!.reverse().join('')}`);
      expect(scalar > 0n && scalar < order).toBe(true);
    }
  });
  it('uses a 256-bit GooseRelay tunnel key and valid PingTunnel integer', () => {
    expect(generateOutboundCredentials({ type: 'gooserelay' }).tunnel_key).toMatch(
      /^[0-9a-f]{64}$/,
    );
    const key = Number(generateOutboundCredentials({ type: 'pingtunnel' }).key);
    expect(Number.isInteger(key) && key >= 0 && key <= 2147483647).toBe(true);
  });
  it('keeps anonymous HTTP/SOCKS intact unless credential creation is explicitly selected', () => {
    expect(autoConfigureOutbound({ type: 'http', server: 'example.com' })).not.toHaveProperty(
      'password',
    );
    expect(generateOutboundCredentials({ type: 'http' }, true)).toMatchObject({
      username: expect.any(String),
      password: expect.any(String),
    });
  });
  it('creates different secrets for independent new connections and keeps SSH key authentication', () => {
    expect(generateOutboundCredentials({ type: 'naive' }).password).not.toBe(
      generateOutboundCredentials({ type: 'naive' }).password,
    );
    expect(
      generateOutboundCredentials({ type: 'ssh', private_key: ['existing-key'] }),
    ).not.toHaveProperty('password');
  });
});

describe('protocol-specific automatic requirements', () => {
  it('does not invent a Snell PSK for a server it does not control', () => {
    const result = autoConfigureOutbound({ type: 'snell', tag: 'snell', server: 'vpn.test' });
    expect(result).toMatchObject({ server_port: 443, version: 6 });
    expect(result).not.toHaveProperty('psk');
    expect(result).not.toHaveProperty('userkey');
    expect(generateOutboundCredentials({ type: 'snell' }, true)).not.toHaveProperty('psk');
    expect(
      autoConfigureOutbound({ ...result, psk: 'server-psk', userkey: 'server-user' }),
    ).toMatchObject({ psk: 'server-psk', userkey: 'server-user' });
  });
  it('enables the TLS required by TrustTunnel and generates authentication', () => {
    expect(autoConfigureOutbound({ type: 'trusttunnel', server: 'vpn.test' })).toMatchObject({
      tls: { enabled: true, server_name: 'vpn.test' },
      username: expect.any(String),
      password: expect.any(String),
    });
  });
  it('generates the selected Hysteria2 obfuscation password without rotating an existing one', () => {
    const generated = autoConfigureOutbound({ type: 'hysteria2', obfs: { type: 'salamander' } });
    expect(generated).toHaveProperty('obfs.password', expect.any(String));
    expect(autoConfigureOutbound(generated)).toEqual(generated);
  });
});
