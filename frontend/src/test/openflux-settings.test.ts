import { describe, expect, it } from 'vitest';
import { OpenFluxInboundSettingsSchema } from '@/schemas/protocols/inbound/additional-vpn';
import { suggestOpenFluxTransport, validOpenFluxChannelURL } from '@/lib/openflux';

describe('OpenFlux settings', () => {
  it('creates a direct channel with automatic credentials', () => {
    expect(OpenFluxInboundSettingsSchema.parse({})).toMatchObject({
      context: '', clients: [], transports: [{ type: 'direct', priority: 100 }],
    });
  });
  it('rejects duplicate transports and unfinished document channels', () => {
    expect(OpenFluxInboundSettingsSchema.safeParse({ transports: [{ type: 'direct' }, { type: 'direct' }] }).success).toBe(false);
    expect(OpenFluxInboundSettingsSchema.safeParse({ transports: [{ type: 'vyandex', url: '' }] }).success).toBe(false);
  });
  it('rejects URL and INI mismatches before saving', () => {
    for (const url of ['http://docs.yandex.ru/a', 'https://user:secret@example.org/a', 'https://example.org/a#b', 'https://example.org/a;b', ' https://example.org/a']) {
      expect(validOpenFluxChannelURL(url)).toBe(false);
    }
    expect(OpenFluxInboundSettingsSchema.safeParse({ context: ' padded ' }).success).toBe(false);
    expect(OpenFluxInboundSettingsSchema.safeParse({ context: 'http://#' }).success).toBe(true);
  });
  it('uses official UTF-16 secret lengths', () => {
    expect(OpenFluxInboundSettingsSchema.safeParse({ clients: [{ email: 'a', password: 'я'.repeat(16) }] }).success).toBe(true);
    expect(OpenFluxInboundSettingsSchema.safeParse({ clients: [{ email: 'a', password: 'я'.repeat(10) }] }).success).toBe(false);
  });
  it('suggests a channel without accepting lookalike provider domains', () => {
    expect(suggestOpenFluxTransport('https://docs.yandex.ru/edit/d/a')).toBe('vyandex');
    expect(suggestOpenFluxTransport('https://boards.yandex.ru/a')).toBe('boards');
    expect(suggestOpenFluxTransport('https://docs.yandex.ru.attacker.org/a')).toBeUndefined();
  });
});
