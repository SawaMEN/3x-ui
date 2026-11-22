import { describe, expect, it } from 'vitest';
import { rawOutboundToFormValues, formValuesToWirePayload } from '@/lib/xray/outbound-form-adapter';
import { OutboundFormSchema } from '@/schemas/forms/outbound-form';
import { outboundBasicConversionLoss } from '@/lib/xray/outbound-loss-check';

describe('zapret2 virtual outbound', () => {
  it('roundtrips a dedicated local port and selected strategy', () => {
    const raw = {
      protocol: 'zapret2',
      tag: 'zapret2-rf',
      settings: {
        port: 15900,
        profile: 'balanced',
        tcp: true,
        udp: false,
        testDomains: ['rutracker.org'],
      },
    };
    const form = rawOutboundToFormValues(raw);
    expect(form.protocol).toBe('zapret2');
    expect(OutboundFormSchema.safeParse(form).success).toBe(true);
    expect(formValuesToWirePayload(form)).toMatchObject(raw);
    expect(outboundBasicConversionLoss(raw)).toBeNull();
  });

  it('refuses privileged SOCKS ports and unsupported modes', () => {
    const form = rawOutboundToFormValues({
      protocol: 'zapret2',
      settings: { port: 80, profile: 'safe' },
    });
    expect(OutboundFormSchema.safeParse(form).success).toBe(false);
  });
});
