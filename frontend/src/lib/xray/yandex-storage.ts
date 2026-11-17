// Fixed provider presets shared by the XDRIVE editor. The active VPN core
// receives only its documented "template" or "s3" transport settings:
// no new sing-box transport kinds or proxy protocol are invented.
export type YandexStoragePreset = 'webdav' | 'disk-api' | 'object-storage' | 'custom';

export const YANDEX_WEBDAV = 'https://webdav.yandex.ru';
export const YANDEX_DISK_API = 'https://cloud-api.yandex.net/v1/disk';
export const YANDEX_OBJECT_STORAGE = 'https://storage.yandexcloud.net';

export const DEFAULT_YANDEX_FOLDER = 'xdrive';

export function validYandexFolder(value: string): boolean {
  return /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(value);
}

export function validYandexBucket(value: string): boolean {
  return (
    value.length >= 3 &&
    value.length <= 63 &&
    /^[a-z0-9][a-z0-9.-]*[a-z0-9]$/.test(value) &&
    !value.includes('..') &&
    !/^\d+\.\d+\.\d+\.\d+$/.test(value)
  );
}

export function yandexWebdavTemplate(): Record<string, unknown> {
  return {
    flatten: true,
    auth: { type: 'basic', username: '{secret0}', password: '{secret1}' },
    put: { method: 'PUT', url: `${YANDEX_WEBDAV}/{folder}/{name}` },
    get: { method: 'GET', url: `${YANDEX_WEBDAV}/{folder}/{name}` },
    delete: { method: 'DELETE', url: `${YANDEX_WEBDAV}/{folder}/{name}` },
    list: {
      method: 'PROPFIND',
      url: `${YANDEX_WEBDAV}/{folder}/`,
      headers: { Depth: '1' },
      namesRegex: '<d:href>[^<]*/([^/<]+)</d:href>',
    },
    retry: { status: [429, 500, 502, 503] },
  };
}

// The XDRIVE template backend follows the "href" returned by Yandex Disk.
// These presigned URLs can be on other hosts than cloud-api.yandex.net.
export function yandexDiskApiTemplate(): Record<string, unknown> {
  const base = `${YANDEX_DISK_API}/resources`;
  const root = 'disk%3A%2F{folder}';
  const path = `${root}%2F{name}`;
  return {
    flatten: true,
    auth: { type: 'static', header: { Authorization: 'OAuth {secret0}' } },
    put: {
      method: 'GET',
      url: `${base}/upload?path=${path}&overwrite=true`,
      hrefPath: 'href',
      hrefMethod: 'PUT',
    },
    get: { method: 'GET', url: `${base}/download?path=${path}`, hrefPath: 'href' },
    delete: { method: 'DELETE', url: `${base}?path=${path}&permanently=true` },
    list: {
      method: 'GET',
      url: `${base}?path=${root}&limit=1000&fields=_embedded.items.name`,
      namesRegex: '"name"\\s*:\\s*"([^"]+)"',
    },
    retry: { status: [429, 500, 502, 503] },
  };
}

export function identifyYandexStoragePreset(settings: {
  service?: string;
  template?: unknown;
  s3?: { endpoint?: string } | undefined;
}): YandexStoragePreset {
  if (settings.service === 's3' && settings.s3?.endpoint === YANDEX_OBJECT_STORAGE)
    return 'object-storage';
  if (
    settings.service !== 'template' ||
    !settings.template ||
    typeof settings.template !== 'object'
  )
    return 'custom';
  const template = settings.template as { get?: { url?: unknown }; auth?: { type?: unknown } };
  const url = String(template.get?.url ?? '');
  if (url.startsWith(YANDEX_WEBDAV + '/') && template.auth?.type === 'basic') return 'webdav';
  if (url.startsWith(YANDEX_DISK_API + '/') && template.auth?.type === 'static') return 'disk-api';
  return 'custom';
}

export function yandexStoragePresetValues(preset: Exclude<YandexStoragePreset, 'custom'>) {
  switch (preset) {
    case 'webdav':
      return {
        service: 'template',
        remoteFolder: DEFAULT_YANDEX_FOLDER,
        secrets: ['', ''],
        template: yandexWebdavTemplate(),
        segmentBytes: 262144,
        pollIntervalMs: 300,
        maxPollIntervalMs: 1500,
        flushIntervalMs: 100,
        concurrency: 8,
      };
    case 'disk-api':
      return {
        service: 'template',
        remoteFolder: DEFAULT_YANDEX_FOLDER,
        secrets: [''],
        template: yandexDiskApiTemplate(),
        segmentBytes: 262144,
        pollIntervalMs: 500,
        maxPollIntervalMs: 2000,
        flushIntervalMs: 150,
        concurrency: 4,
      };
    case 'object-storage':
      return {
        service: 's3',
        remoteFolder: DEFAULT_YANDEX_FOLDER,
        secrets: ['', ''],
        s3: {
          endpoint: YANDEX_OBJECT_STORAGE,
          region: 'ru-central1',
          bucket: '',
          pathStyle: true,
        },
        segmentBytes: 524288,
        pollIntervalMs: 100,
        maxPollIntervalMs: 500,
        flushIntervalMs: 20,
        concurrency: 8,
      };
  }
}
