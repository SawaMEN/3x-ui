import { describe, expect, it } from 'vitest';
import { OpenFluxInboundSettingsSchema } from '@/schemas/protocols/inbound/additional-vpn';
import {
  openFluxMAXClientFiles,
  openFluxMAXChannels,
  suggestOpenFluxTransport,
  validOpenFluxChannelURL,
} from '@/lib/openflux';

describe('OpenFlux settings', () => {
  it('creates a direct channel with automatic credentials', () => {
    expect(OpenFluxInboundSettingsSchema.parse({})).toMatchObject({
      context: '',
      clients: [],
      transports: [{ type: 'direct', priority: 100 }],
    });
  });
  it('rejects duplicate transports and unfinished document channels', () => {
    expect(
      OpenFluxInboundSettingsSchema.safeParse({
        transports: [{ type: 'direct' }, { type: 'direct' }],
      }).success,
    ).toBe(false);
    expect(
      OpenFluxInboundSettingsSchema.safeParse({ transports: [{ type: 'vyandex', url: '' }] })
        .success,
    ).toBe(false);
  });
  it('rejects URL and INI mismatches before saving', () => {
    for (const url of [
      'http://docs.yandex.ru/a',
      'https://user:secret@example.org/a',
      'https://example.org/a#b',
      'https://example.org/a;b',
      ' https://example.org/a',
    ]) {
      expect(validOpenFluxChannelURL(url)).toBe(false);
    }
    expect(OpenFluxInboundSettingsSchema.safeParse({ context: ' padded ' }).success).toBe(false);
    expect(OpenFluxInboundSettingsSchema.safeParse({ context: 'http://#' }).success).toBe(true);
  });
  it('accepts every shareable carrier from the latest stable release', () => {
    const transports = [
      { type: 'direct' },
      ...['yandex', 'vyandex', 'boards', 'mailru', 'cupsonline', 'avito', 'telemost'].map(
        (type) => ({ type, url: 'https://example.org/room' }),
      ),
    ];
    expect(OpenFluxInboundSettingsSchema.safeParse({ transports }).success).toBe(true);
    expect(suggestOpenFluxTransport('https://cups.online/room/a')).toBe('cupsonline');
    expect(suggestOpenFluxTransport('https://code.avito.ru/a')).toBe('avito');
    expect(suggestOpenFluxTransport('https://telemost.yandex.ru/j/a')).toBe('telemost');
  });
  it('validates negotiated packet limits and safe logging levels', () => {
    for (const maxPacketSize of [0, 1280, 65000])
      expect(OpenFluxInboundSettingsSchema.safeParse({ maxPacketSize }).success).toBe(true);
    for (const maxPacketSize of [1, 1279, 65001])
      expect(OpenFluxInboundSettingsSchema.safeParse({ maxPacketSize }).success).toBe(false);
    expect(OpenFluxInboundSettingsSchema.safeParse({ debug: 3 }).success).toBe(false);
  });
  it('uses official UTF-16 secret lengths', () => {
    expect(
      OpenFluxInboundSettingsSchema.safeParse({
        clients: [{ email: 'a', password: 'я'.repeat(16) }],
      }).success,
    ).toBe(true);
    expect(
      OpenFluxInboundSettingsSchema.safeParse({
        clients: [{ email: 'a', password: 'я'.repeat(10) }],
      }).success,
    ).toBe(false);
  });
  it('suggests a channel without accepting lookalike provider domains', () => {
    expect(suggestOpenFluxTransport('https://docs.yandex.ru/edit/d/a')).toBe('vyandex');
    expect(suggestOpenFluxTransport('https://disk.yandex.ru/i/AbCd')).toBe('vyandex');
    expect(suggestOpenFluxTransport('https://cloud.mail.ru/public/a/b')).toBe('mailru');
    expect(suggestOpenFluxTransport('https://boards.yandex.ru/a')).toBe('boards');
    expect(suggestOpenFluxTransport('https://docs.yandex.ru.attacker.org/a')).toBeUndefined();
  });
});

describe('Web MAX configuration', () => {
  it('accepts server tokens and validates IDs without rounding 64-bit values', () => {
    expect(
      OpenFluxInboundSettingsSchema.safeParse({
        transports: [{ type: 'oneme', token: 'account-token', uid: '9223372036854775807' }],
      }).success,
    ).toBe(true);
    for (const uid of ['0', '-1', '9223372036854775808', '1.5', '123;UID'])
      expect(
        OpenFluxInboundSettingsSchema.safeParse({
          transports: [{ type: 'oneme', token: 'account-token', uid }],
        }).success,
      ).toBe(false);
    for (const token of ['', 'token\nRole=exit', 'token;comment', 'token#comment'])
      expect(
        OpenFluxInboundSettingsSchema.safeParse({ transports: [{ type: 'oneme', token }] }).success,
      ).toBe(false);
  });
  it('prepares a client config with the client account and exact legacy context', () => {
    const files = openFluxMAXClientFiles(
      'a'.repeat(32),
      'http://#',
      '123456',
      'client-token',
      'Instructions',
    );
    expect(files?.['openflux-client.conf']).toContain('Token = client-token');
    expect(files?.['openflux-client.conf']).toContain('UID = 123456');
    expect(files?.['README.txt']).toContain("--negotiate --session-context 'http://#'");
    expect(files?.['secret.txt']).toBe('a'.repeat(32));
    expect(openFluxMAXClientFiles('short', 'context', '123', 'token', '')).toBeUndefined();
    expect(
      openFluxMAXClientFiles('a'.repeat(32), 'context', '123', 'token\nUID=1', ''),
    ).toBeUndefined();
  });
});

it('distinguishes MAX-only setups from mixed QR-capable channels', () => {
  expect(openFluxMAXChannels(JSON.stringify({ transports: [{ type: 'oneme' }] }))).toEqual({
    present: true,
    only: true,
  });
  expect(openFluxMAXChannels({ transports: [{ type: 'oneme' }, { type: 'direct' }] })).toEqual({
    present: true,
    only: false,
  });
  expect(openFluxMAXChannels('invalid')).toEqual({ present: false, only: false });
});
