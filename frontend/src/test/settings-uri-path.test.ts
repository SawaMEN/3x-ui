import { describe, expect, it } from 'vitest';
import { sanitizePath } from '@/pages/settings/uriPath';

describe('panel and subscription paths', () => {
  it('removes router parameters, wildcards and URL delimiters', () => {
    expect(sanitizePath('/:panel*?#%\\/')).toBe('/panel/');
    expect(sanitizePath('/my-secret_123/')).toBe('/my-secret_123/');
  });
});
