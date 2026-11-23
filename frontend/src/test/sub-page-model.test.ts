import { describe, expect, it } from 'vitest';

import {
  buildSubApps,
  buildTelemtEndpoint,
  daysUntil,
  detectPlatform,
  resolveSubStatus,
  usagePercent,
} from '@/pages/sub/subPageModel';

const DAY = 86_400_000;
const NOW = Date.UTC(2026, 8, 15, 12, 0, 0);

describe('buildTelemtEndpoint', () => {
  it('uses the page origin after the subscription listener port changes', () => {
    expect(buildTelemtEndpoint('https://sub.example:8443/custom/id?html=1#links', 'id')).toBe(
      'https://sub.example:8443/custom/id?format=telemt',
    );
  });

  it('keeps a migrated Hiddify alias on its public port', () => {
    expect(buildTelemtEndpoint('https://cdn.example/LegacyPath/id/', 'id')).toBe(
      'https://cdn.example/LegacyPath/id/?format=telemt',
    );
  });

  it('does not build a Telemt endpoint without a subscription id', () => {
    expect(buildTelemtEndpoint('https://sub.example/custom/id/', '')).toBe('');
  });

  it('rejects malformed page URLs', () => {
    expect(buildTelemtEndpoint('not a url', 'id')).toBe('');
  });

  it('rejects non-http page URLs', () => {
    expect(buildTelemtEndpoint('file:///tmp/sub/id/index.html', 'id')).toBe('');
  });
});

describe('resolveSubStatus', () => {
  const base = { enabled: true, usedByte: 10, totalByte: 100, expireMs: NOW + DAY };

  it.each([
    [
      'disabled wins over expiry and quota',
      { ...base, enabled: false, expireMs: NOW - DAY },
      'disabled',
    ],
    ['expired at the expiry instant', { ...base, expireMs: NOW }, 'expired'],
    ['expired beats depleted', { ...base, usedByte: 100, expireMs: NOW - DAY }, 'expired'],
    ['depleted once usage reaches the quota', { ...base, usedByte: 100 }, 'depleted'],
    [
      'unlimited with neither quota nor expiry',
      { ...base, totalByte: 0, expireMs: 0 },
      'unlimited',
    ],
    ['active without quota but with a future expiry', { ...base, totalByte: 0 }, 'active'],
    ['active inside quota and before expiry', base, 'active'],
  ] as const)('%s', (_name, input, want) => {
    expect(resolveSubStatus(input, NOW)).toBe(want);
  });
});

describe('daysUntil', () => {
  it('is null for a subscription that never expires', () => {
    expect(daysUntil(0, NOW)).toBeNull();
  });

  it('rounds the last partial day up to 1', () => {
    expect(daysUntil(NOW + 3 * 3_600_000, NOW)).toBe(1);
  });

  it('counts whole days', () => {
    expect(daysUntil(NOW + 23 * DAY, NOW)).toBe(23);
  });

  it('stays at 0 after expiry', () => {
    expect(daysUntil(NOW - DAY, NOW)).toBe(0);
  });
});

describe('usagePercent', () => {
  it('is 0 without a quota instead of NaN or Infinity', () => {
    expect(usagePercent(5_000, 0)).toBe(0);
  });

  it('clamps an over-quota client to 100', () => {
    expect(usagePercent(150, 100)).toBe(100);
  });
});

describe('detectPlatform', () => {
  it.each([
    ['Mozilla/5.0 (Linux; Android 14; Pixel 8)', 5, 'android'],
    ['Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X)', 5, 'ios'],
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)', 5, 'ios'],
    ['Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)', 0, 'macos'],
    ['Mozilla/5.0 (Windows NT 10.0; Win64; x64)', 10, 'windows'],
    ['Mozilla/5.0 (X11; Linux x86_64)', 0, 'linux'],
    ['Unknown browser', 0, 'android'],
  ])('detects %s with %i touch points as %s', (ua, touchPoints, expected) => {
    expect(detectPlatform(ua, touchPoints)).toBe(expected);
  });
});

describe('buildSubApps', () => {
  const subUrl = 'https://sub.example.com/sub/abc?token=1';

  it('uses the default subscription and preserves other query parameters', () => {
    const original = `${subUrl}&key=a%2Bb&format=all#old`;
    const [hiddify] = buildSubApps('android', original, 'New');
    const deepLink = new URL(hiddify.url!);
    expect(deepLink.searchParams.get('url')).toBe(`${subUrl}&key=a%2Bb`);
    expect(deepLink.hash).toBe('#New');

    const [xray] = buildSubApps('android', subUrl, 'Xray');
    expect(new URL(xray.url!).searchParams.get('url')).toBe(subUrl);
  });

  it.each([
    ['android', 'app.hiddify.com', 'com.happproxy', 'org.amnezia.awg'],
    ['ios', 'id6596777532', 'id6504287215', 'id6478942365?platform=iphone'],
    [
      'windows',
      'apps.microsoft.com/detail/9ndgwm32bnxk',
      'happ-desktop/releases/latest',
      'amneziawg-windows-client/releases/latest',
    ],
    [
      'macos',
      'id6596777532?platform=mac',
      'id6504287215?platform=mac',
      'id6478942365?platform=mac',
    ],
    ['linux', 'hiddify-app/releases/latest', 'happ-desktop/releases/latest', 'amneziawg-tools'],
  ] as const)(
    'offers the ordered clients and download links for %s',
    (platform, hiddify, happ, awg) => {
      const apps = buildSubApps(platform, subUrl, 'Nova Net');
      expect(apps.map((app) => app.name)).toEqual(['Hiddify', 'Hiddify Beta', 'Happ', 'AmneziaWG']);
      expect(apps.map((app) => app.url)).toEqual([
        `hiddify://import/?url=${encodeURIComponent(subUrl)}#Nova%20Net`,
        `hiddify://import/?url=${encodeURIComponent(subUrl)}#Nova%20Net`,
        `happ://add/${subUrl}`,
        undefined,
      ]);
      expect(apps[0].downloads[0].href).toContain(hiddify);
      expect(apps[1].downloads).toEqual([
        { label: 'GitHub', href: 'https://github.com/hiddify/hiddify-app/releases' },
      ]);
      expect(apps[2].downloads[0].href).toContain(happ);
      expect(apps[3].downloads[0].href).toContain(awg);
    },
  );
});
