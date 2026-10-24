import { describe, expect, it } from 'vitest';
import { SnellInboundSettingsSchema } from '@/schemas/protocols/inbound/snell';
import { parseOutboundLink } from '@/lib/xray/outbound-link-parser';

describe('Snell settings and subscription links', () => {
  const valid = (version: 5 | 6, psk: string) =>
    SnellInboundSettingsSchema.safeParse({ version, psk }).success;

  it('keeps an empty PSK for secure generation on save', () => {
    const parsed = SnellInboundSettingsSchema.parse({});
    expect(parsed.version).toBe(6);
    expect(parsed.mode).toBe('default');
    expect(parsed.psk).toBe('');
  });

  it('checks v6 PSK byte limits but accepts the v5-compatible PSK', () => {
    expect(valid(6, 'short')).toBe(false);
    expect(valid(5, 'short')).toBe(true);
    expect(valid(6, '😀'.repeat(3))).toBe(true);
    expect(valid(6, 'a'.repeat(256))).toBe(false);
    expect(valid(6, 'bad\nkey')).toBe(false);
  });

  it('imports a multi-user v5 link as client v4 with HTTP obfuscation', () => {
    const link =
      'snell://deploy-psk@example.org:443?version=4&userkey=user-key&obfs=http&obfs-host=www.example.org#Snell';
    const parsed = parseOutboundLink(link);
    expect(parsed?.protocol).toBe('snell');
    expect(parsed?.settings).toMatchObject({
      version: 4,
      psk: 'deploy-psk',
      userkey: 'user-key',
      obfs_mode: 'http',
      obfs_host: 'www.example.org',
    });
  });

  it('preserves Snell v5 in a QUIC proxy link', () => {
    const link = 'snell://deploy-psk@example.org:443?version=5&userkey=user-key#QUIC';
    expect(parseOutboundLink(link)?.settings).toMatchObject({
      version: 5,
      psk: 'deploy-psk',
      userkey: 'user-key',
    });
  });

  it('rejects QUIC Proxy on v6 and accepts v5', () => {
    expect(SnellInboundSettingsSchema.safeParse({ version: 5, quicProxy: true, psk: 'secret' }).success).toBe(true);
    expect(SnellInboundSettingsSchema.safeParse({ version: 6, quicProxy: true, psk: '123456789012' }).success).toBe(false);
  });

  it('imports a v6 link with mode and a distinct user key', () => {
    const link =
      'snell://deploy-psk-12345@example.org:443?version=6&userkey=another-key&mode=unshaped#Snell';
    expect(parseOutboundLink(link)?.settings).toMatchObject({
      version: 6,
      mode: 'unshaped',
      userkey: 'another-key',
    });
  });
});
