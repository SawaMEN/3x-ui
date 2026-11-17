import { describe, expect, it } from 'vitest';
import {
  YANDEX_WEBDAV,
  YANDEX_DISK_API,
  YANDEX_OBJECT_STORAGE,
  identifyYandexStoragePreset,
  validYandexFolder,
  validYandexBucket,
  yandexStoragePresetValues,
  yandexDiskApiTemplate,
  yandexWebdavTemplate,
} from '@/lib/xray/yandex-storage';
import { XDriveStreamSettingsSchema } from '@/schemas/protocols/stream/xdrive';
import { rawInboundToFormValues, formValuesToWirePayload } from '@/lib/xray/inbound-form-adapter';

describe('native sing-box Yandex XDRIVE presets', () => {
  it('builds a compatible Yandex Disk WebDAV template with no embedded secrets', () => {
    const values = yandexStoragePresetValues('webdav');
    expect(values.service).toBe('template');
    expect(values.secrets).toEqual(['', '']);
    expect(values.remoteFolder).toBe('xdrive');
    expect(JSON.stringify(values.template)).not.toContain('password123');
    expect(JSON.stringify(values.template)).toContain(YANDEX_WEBDAV);
    expect(yandexWebdavTemplate()).toMatchObject({
      flatten: true,
      auth: { type: 'basic', username: '{secret0}', password: '{secret1}' },
      list: { method: 'PROPFIND' },
    });
    expect(XDriveStreamSettingsSchema.parse(values)).toMatchObject(values);
    expect(identifyYandexStoragePreset(values)).toBe('webdav');
  });

  it('builds the Yandex Disk REST API two-stage upload and download', () => {
    const values = yandexStoragePresetValues('disk-api');
    const template = yandexDiskApiTemplate();
    expect(values.service).toBe('template');
    expect(template).toMatchObject({
      auth: { type: 'static', header: { Authorization: 'OAuth {secret0}' } },
      put: { method: 'GET', hrefPath: 'href', hrefMethod: 'PUT' },
      get: { hrefPath: 'href' },
    });
    expect(JSON.stringify(template)).toContain(YANDEX_DISK_API);
    expect(XDriveStreamSettingsSchema.parse(values)).toMatchObject(values);
    expect(identifyYandexStoragePreset(values)).toBe('disk-api');
  });

  it('configures Yandex Cloud S3 without mistaking it for personal Disk', () => {
    const values = yandexStoragePresetValues('object-storage');
    expect(values.service).toBe('s3');
    expect(values).toMatchObject({
      s3: { endpoint: YANDEX_OBJECT_STORAGE, region: 'ru-central1', pathStyle: true },
    });
    expect(identifyYandexStoragePreset(values)).toBe('object-storage');
    expect(XDriveStreamSettingsSchema.parse(values)).toMatchObject(values);
  });

  it('never treats a user-supplied template or arbitrary S3 endpoint as Yandex', () => {
    expect(
      identifyYandexStoragePreset({
        service: 'template',
        template: { get: { url: 'https://example.org/files' } },
      }),
    ).toBe('custom');
    expect(
      identifyYandexStoragePreset({
        service: 's3',
        s3: { endpoint: 'https://example.org' },
      }),
    ).toBe('custom');
  });

  it('rejects unsafe folder and bucket names', () => {
    expect(validYandexFolder('xdrive_test-9')).toBe(true);
    for (const value of ['', '../private', 'path/sub', '%2F', ' spaced ', 'a'.repeat(65)]) {
      expect(validYandexFolder(value)).toBe(false);
    }
    expect(validYandexBucket('xdrive-storage-1')).toBe(true);
    for (const value of ['', 'Abc', 'bad..bucket', '1.2.3.4', '-bad', 'bad/next']) {
      expect(validYandexBucket(value)).toBe(false);
    }
  });
  it('keeps the native WebDAV XDRIVE preset across inbound edit/save', () => {
    const xdriveSettings = {
      ...yandexStoragePresetValues('webdav'),
      secrets: ['dedicated-user', 'application-password'],
    };
    const stream = {
      network: 'xdrive',
      security: 'none',
      xdriveSettings,
    };
    const inbound = rawInboundToFormValues({
      protocol: 'vless',
      port: 443,
      settings: JSON.stringify({ decryption: 'private', encryption: 'public', clients: [] }),
      streamSettings: JSON.stringify(stream),
    });
    const wire = formValuesToWirePayload(inbound);
    expect(JSON.parse(wire.streamSettings)).toMatchObject(stream);
    expect(identifyYandexStoragePreset(JSON.parse(wire.streamSettings).xdriveSettings)).toBe(
      'webdav',
    );
  });
});
