import { expect, it } from 'vitest';
import { CookieManager, LanguageManager } from '@/utils';

it('ignores a malformed language cookie instead of aborting application startup', () => {
  document.cookie = 'lang=%E0%A4%A; path=/';
  try {
    expect(CookieManager.getCookie('lang')).toBe('');
    expect(LanguageManager.getLanguage()).toMatch(/^(ru-RU|en-US)$/);
  } finally {
    document.cookie = 'lang=; max-age=0; path=/';
  }
});
