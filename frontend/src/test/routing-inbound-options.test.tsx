import { describe, expect, it } from 'vitest';

import { uniqueSelectableInboundTags } from '@/pages/xray/routing/helpers';

describe('Routing inbound options', () => {
  it('includes panel-managed tags even without Xray template or config tags', () => {
    expect(uniqueSelectableInboundTags(['in-live'])).toEqual(['in-live']);
  });

  it('deduplicates tags from API options, the template and the config response', () => {
    expect(uniqueSelectableInboundTags(['in-live', 'in-live', 'in-legacy', 'in-live'])).toEqual([
      'in-live',
      'in-legacy',
    ]);
  });

  it('retains fallback config tags and hides internal balancer loopbacks', () => {
    expect(uniqueSelectableInboundTags([undefined, 'in-from-config', '_bl_internal', ''])).toEqual([
      'in-from-config',
    ]);
  });
});
