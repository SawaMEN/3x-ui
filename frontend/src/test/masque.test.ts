import { describe, expect, it } from 'vitest';
import { MasqueInboundSettingsSchema } from '@/schemas/protocols/inbound/masque';
import { createDefaultInboundSettings } from '@/lib/xray/inbound-defaults';
import { formValuesToWirePayload, rawInboundToFormValues } from '@/lib/xray/inbound-form-adapter';
import { isClientAttachableProtocol } from '@/lib/inbounds/client-attachable';
import { canEnableSniffing, canEnableStream } from '@/lib/xray/protocol-capabilities';

describe('MASQUE inbound', () => {
  it('seeds defaults and retains endpoint fields through the form adapter', () => {
    const settings = createDefaultInboundSettings('masque');
    expect(settings).toEqual(MasqueInboundSettingsSchema.parse({}));
    const values = rawInboundToFormValues({
      protocol: 'masque',
      port: 443,
      settings: JSON.stringify(settings),
    });
    const wire = formValuesToWirePayload(values);
    expect(JSON.parse(wire.settings).address).toEqual(['172.31.255.1/24', 'fd7a:115c:a1e0::1/64']);
    expect(JSON.parse(wire.settings).version).toEqual([3, 2, 1]);
    expect(isClientAttachableProtocol('masque')).toBe(true);
    expect(canEnableStream({ protocol: 'masque' })).toBe(false);
    expect(canEnableSniffing({ protocol: 'masque' })).toBe(false);
  });
});

describe('MASQUE validation', () => {
  it.each(['/tunnel{?target,ipproto}', '/tunnel?address={target}&protocol={ipproto}'])(
    'accepts query template %s',
    (path) => {
      expect(MasqueInboundSettingsSchema.parse({ path }).path).toBe(path);
    },
  );
  it.each(['/bad path', '/{target', '/{}', '/{unknown}', '/{+target}', '/%zz', '/tunnel#fragment'])(
    'rejects invalid template %s',
    (path) => {
      expect(MasqueInboundSettingsSchema.safeParse({ path }).success).toBe(false);
    },
  );
  it('rejects duplicate versions and invalid or duplicate usernames', () => {
    expect(MasqueInboundSettingsSchema.safeParse({ version: [3, 3] }).success).toBe(false);
    expect(
      MasqueInboundSettingsSchema.safeParse({ clients: [{ email: 'alice:bob' }] }).success,
    ).toBe(false);
    expect(
      MasqueInboundSettingsSchema.safeParse({ clients: [{ email: 'alice' }, { email: 'ALICE' }] })
        .success,
    ).toBe(false);
  });
});
