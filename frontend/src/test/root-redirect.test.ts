import { expect, it } from 'vitest';
import { rootRedirectError } from '@/pages/settings/root-redirect';
it('rejects invalid, oversized and self-root redirect targets', () => {
  for (const value of [
    '',
    'javascript:alert(1)',
    '//example.com',
    'https://user:pass@example.com',
    'https://example.com:99999',
    'https://example.com/a\nb',
    'https://example.com/' + 'x'.repeat(2048),
  ])
    expect(rootRedirectError(value)).not.toBe('');
  expect(rootRedirectError('https://PANEL.example./', ['panel.example'])).not.toBe('');
});
it('allows same-domain pages and the explicit panel bypass', () => {
  expect(rootRedirectError('https://other.example/page', ['panel.example'])).toBe('');
  expect(rootRedirectError('https://panel.example/login', ['panel.example'])).toBe('');
  expect(rootRedirectError('https://panel.example/?panel=1', ['panel.example'])).toBe('');
});
