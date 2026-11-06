import { expect, it } from 'vitest';
import { rawInboundToFormValues, formValuesToWirePayload } from '@/lib/xray/inbound-form-adapter';
import { fillStreamDefaults } from '@/lib/xray/stream-defaults';

it('keeps XDrive credentials and tuning in an edit-save round trip', () => {
  const stream = {
    network: 'xdrive',
    security: 'none',
    xdriveSettings: {
      service: 'google-drive',
      remoteFolder: 'folder-id',
      secrets: ['client-id', 'client-secret', 'refresh-token'],
      concurrency: 8,
      pollIntervalMs: 50,
    },
  };
  expect(fillStreamDefaults(stream).xdriveSettings).toMatchObject(stream.xdriveSettings);
  const form = rawInboundToFormValues({
    protocol: 'vless',
    port: 443,
    settings: JSON.stringify({ clients: [], decryption: 'private-key', encryption: 'public-key' }),
    streamSettings: JSON.stringify(stream),
  });
  const wire = formValuesToWirePayload(form);
  expect(JSON.parse(wire.streamSettings)).toMatchObject(stream);
  expect(JSON.parse(wire.settings).decryption).toBe('private-key');
});
