import { describe, expect, it } from 'vitest';

import {
  COVER_SITE_HTML_MAX_BYTES,
  coverSiteHTMLValidation,
  starterCoverSiteHTML,
} from '@/components/CoverSiteHTMLEditor';

describe('cover-site custom HTML', () => {
  it('provides a complete, valid starting page', () => {
    expect(coverSiteHTMLValidation(starterCoverSiteHTML)).toBeNull();
  });

  it('rejects empty content, fragments and NUL bytes', () => {
    expect(coverSiteHTMLValidation('')).toBe('document');
    expect(coverSiteHTMLValidation('<main>Not a document</main>')).toBe('document');
    expect(coverSiteHTMLValidation('<html>invalid\0content</html>')).toBe('document');
  });

  it('enforces the byte limit, not the character count', () => {
    const opening = '<html>';
    const closing = '</html>';
    const allowance = COVER_SITE_HTML_MAX_BYTES - opening.length - closing.length;
    expect(coverSiteHTMLValidation(opening + 'a'.repeat(allowance) + closing)).toBeNull();
    expect(coverSiteHTMLValidation(opening + 'a'.repeat(allowance + 1) + closing)).toBe('size');
    expect(coverSiteHTMLValidation(opening + 'я'.repeat(allowance / 2 + 1) + closing)).toBe('size');
  });
});
