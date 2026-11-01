import { expect, it } from 'vitest';
import { readableMessage } from '@/lib/readable-message';
it('decodes core configuration indexes without parsing HTML', () => {
  expect(readableMessage('route.rules&#91;3&#93;.outbound')).toBe('route.rules[3].outbound');
  expect(readableMessage('&#x5b;tag&#x5d;')).toBe('[tag]');
  expect(readableMessage('<script>text</script>')).toBe('<script>text</script>');
  expect(readableMessage('bad: &#999999999999; &#0;')).toBe('bad: &#999999999999; &#0;');
});
