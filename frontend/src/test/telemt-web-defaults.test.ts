import { describe, expect, it } from 'vitest';

import {
  completeTelemtWebSettings,
  defaultTelemtWebSettings,
  TelemtWebProxyRequestSchema,
} from '@/schemas/telemt-web';
import { MtprotoInboundSettingsSchema } from '@/schemas/protocols/inbound/mtproto';

describe('Telegram WEB Proxy defaults', () => {
  it('creates complete independent defaults without guessing a domain', () => {
    const first = defaultTelemtWebSettings();
    expect(first).toEqual({
      enabled: false,
      site: { mode: 'auto' },
      domain: '',
      basePath: '',
      aliases: [],
      frontend: 'nginx',
      carrier: 'https',
      carriers: [],
      retryJitter: { enabled: false, minMs: 0, maxMs: 250 },
    });
    first.aliases.push('first.example.com');
    expect(defaultTelemtWebSettings().aliases).toEqual([]);
  });

  it('fills missing legacy fields and preserves explicit user choices', () => {
    const completed = completeTelemtWebSettings({
      enabled: true,
      domain: 'web.example.com',
      frontend: 'haproxy',
      carrier: 'websocket',
      carriers: ['websocket', 'https'],
      retryJitter: { enabled: true, minMs: 5, maxMs: 600 },
    });
    expect(completed).toMatchObject({
      enabled: true,
      site: { mode: 'auto' },
      domain: 'web.example.com',
      frontend: 'haproxy',
      carrier: 'websocket',
      carriers: ['websocket', 'https'],
      retryJitter: { enabled: true, minMs: 5, maxMs: 600 },
    });
    const parsed = TelemtWebProxyRequestSchema.safeParse(completed);
    expect(parsed.success).toBe(true);
  });

  it('applies schema defaults for partially saved WEB configurations', () => {
    const input = {
      clients: [],
      webProxy: {
        enabled: true,
        domain: 'web.example.com',
        retryJitter: { enabled: false },
      },
    };
    const parsed = MtprotoInboundSettingsSchema.parse(input);
    expect(parsed.webProxy).toMatchObject({
      frontend: 'nginx',
      carrier: 'https',
      carriers: [],
      site: { mode: 'auto' },
      retryJitter: { enabled: false, minMs: 0, maxMs: 250 },
    });
    const newRequest = TelemtWebProxyRequestSchema.parse({ domain: 'web.example.com' });
    expect(newRequest).toMatchObject({
      frontend: 'nginx',
      carrier: 'https',
      carriers: [],
      site: { mode: 'auto' },
      retryJitter: { enabled: false, minMs: 0, maxMs: 250 },
    });
  });

  it('does not invent a domain or repair invalid explicit settings', () => {
    expect(TelemtWebProxyRequestSchema.safeParse(completeTelemtWebSettings({})).success).toBe(
      false,
    );
    expect(
      TelemtWebProxyRequestSchema.safeParse({
        domain: 'web.example.com',
        retryJitter: { enabled: true, minMs: 200, maxMs: 100 },
      }).success,
    ).toBe(false);
    expect(
      TelemtWebProxyRequestSchema.safeParse({
        domain: 'web.example.com',
        carriers: ['https', 'https'],
      }).success,
    ).toBe(false);
  });
});

describe('Shared HTTPS ingress migration', () => {
  it('migrates the old single inbound and preserves explicit clearing', () => {
    expect(
      completeTelemtWebSettings({ sharedIngress: { enabled: true, naiveInboundId: 42 } })
        .sharedIngress,
    ).toEqual({ enabled: true, inboundIds: [42] });
    expect(
      completeTelemtWebSettings({
        sharedIngress: { enabled: true, naiveInboundId: 42, inboundIds: [] },
      }).sharedIngress,
    ).toEqual({ enabled: true, inboundIds: [] });
  });
});
