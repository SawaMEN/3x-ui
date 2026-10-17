import { describe, expect, it } from 'vitest';
import { TelemtWebProxyRequestSchema } from '@/schemas/telemt-web';

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
