import { describe, expect, it } from 'vitest';
import { TelemtWebProxyRequestSchema, TelemtWebSiteSchema } from '@/schemas/telemt-web';

describe('Telemt WEB request validation', () => {
  const valid = {
    domain: 'web.example.com',
    frontend: 'haproxy',
    carrier: 'https',
    carriers: ['websocket', 'https'],
  };
  it('preserves carrier order and trims the domain', () => {
    expect(TelemtWebProxyRequestSchema.parse({ ...valid, domain: ' web.example.com ' })).toEqual(
      valid,
    );
  });
  it.each([
    { frontend: 'shell-command' },
    { carrier: 'unsupported' },
    { carriers: ['websocket', 'websocket'] },
    { domain: 'https://web.example.com/path' },
  ])('rejects an invalid request %o', (fields) => {
    expect(TelemtWebProxyRequestSchema.safeParse({ ...valid, ...fields }).success).toBe(false);
  });
});

describe('Telemt optional cover website', () => {
  it('retains generated theme settings and accepts complete HTML', () => {
    expect(
      TelemtWebSiteSchema.parse({ mode: 'generate', theme: 'travel', title: 'My trips' }),
    ).toEqual({ mode: 'generate', theme: 'travel', title: 'My trips' });
    expect(
      TelemtWebSiteSchema.safeParse({
        mode: 'html',
        html: '<html lang="en"><body>Ready</body></html>',
      }).success,
    ).toBe(true);
  });
  it('rejects unsupported themes, incomplete pages and oversized UTF-8 uploads', () => {
    for (const site of [
      { mode: 'generate', theme: '../../etc' },
      { mode: 'html', html: 'not a document' },
      { mode: 'html', html: '<html>' + 'я'.repeat(132000) },
    ])
      expect(TelemtWebSiteSchema.safeParse(site).success).toBe(false);
  });
});
