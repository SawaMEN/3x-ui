import { describe, expect, it } from 'vitest';
import { SnellInboundSettingsSchema } from '@/schemas/protocols/inbound/snell';
import { parseOutboundLink } from '@/lib/xray/outbound-link-parser';

describe('Snell settings and subscription links', () => {
  it('keeps an empty PSK for secure generation on save', () => {
    const parsed = SnellInboundSettingsSchema.parse({});
    expect(parsed.version).toBe(6);
    expect(parsed.mode).toBe('default');
    expect(parsed.psk).toBe('');
  });

  it('checks v6 PSK byte limits but accepts the v5-compatible PSK', () => {
    expect(SnellInboundSettingsSchema.safeParse({ version: 6, psk: 'short' }).success).toBe(false);
    expect(SnellInboundSettingsSchema.safeParse({ version: 5, psk: 'short' }).success).toBe(true);
    expect(
      SnellInboundSettingsSchema.safeParse({ version: 6, psk: '😀'.repeat(3) }).success,
    ).toBe(true);
    expect(
      SnellInboundSettingsSchema.safeParse({ version: 6, psk: 'a'.repeat(256) }).success,
    ).toBe(false);
    expect(SnellInboundSettingsSchema.safeParse({ version: 6, psk: 'bad\nkey' }).success).toBe(
      false,
    );
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
