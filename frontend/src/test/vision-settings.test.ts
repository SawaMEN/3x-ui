import { describe, expect, it } from 'vitest';
import { VisionTestseedSchema } from '@/schemas/protocols/vision';
import { formValuesToWirePayload, rawOutboundToFormValues } from '@/lib/xray/outbound-form-adapter';

describe('Vision padding configuration', () => {
  it('accepts defaults and the largest valid wire padding', () => {
    for (const seed of [[], [900, 500, 900, 256], [1, 1, 0, 65536]]) {
      expect(VisionTestseedSchema.safeParse(seed).success).toBe(true);
    }
  });
  it('rejects incomplete, negative, empty random ranges and wire overflows', () => {
    for (const seed of [
      [900],
      [900, 0, 900, 256],
      [900, 1, 1, 256],
      [900, 65535, 900, 256],
      [900, 500, 900, 65537],
      [900, 500, -1, 256],
    ]) {
      expect(VisionTestseedSchema.safeParse(seed).success).toBe(false);
    }
  });
  it('preserves invalid imported values until explicit validation', () => {
    const wire = {
      protocol: 'vless',
      tag: 'vision-invalid',
      settings: {
        address: 'example.com',
        port: 443,
        id: '11111111-1111-4111-8111-111111111111',
        flow: 'xtls-rprx-vision',
        testseed: [900, 0, 900],
      },
    };
    const form = rawOutboundToFormValues(wire);
    expect(formValuesToWirePayload(form).settings).toHaveProperty('testseed', [900, 0, 900]);
  });
});
