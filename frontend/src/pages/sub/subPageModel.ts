const DAY_MS = 86_400_000;

export function buildTelemtEndpoint(pageURL: string, subID: string): string {
  if (!subID) return '';
  try {
    const endpoint = new URL(pageURL);
    if (endpoint.protocol !== 'http:' && endpoint.protocol !== 'https:') return '';
    endpoint.search = '?format=telemt';
    endpoint.hash = '';
    return endpoint.toString();
  } catch {
    return '';
  }
}

export type SubStatus = 'active' | 'unlimited' | 'expired' | 'depleted' | 'disabled';

export interface SubUsage {
  enabled: boolean;
  usedByte: number;
  totalByte: number;
  expireMs: number;
}

export function resolveSubStatus(sub: SubUsage, now: number): SubStatus {
  if (!sub.enabled) return 'disabled';
  if (sub.expireMs > 0 && now >= sub.expireMs) return 'expired';
  if (sub.totalByte > 0 && sub.usedByte >= sub.totalByte) return 'depleted';
  if (sub.totalByte <= 0 && sub.expireMs === 0) return 'unlimited';
  return 'active';
}

export function daysUntil(expireMs: number, now: number): number | null {
  if (expireMs <= 0) return null;
  return Math.max(0, Math.ceil((expireMs - now) / DAY_MS));
}

export function usagePercent(usedByte: number, totalByte: number): number {
  if (totalByte <= 0) return 0;
  const pct = (usedByte / totalByte) * 100;
  return Number.isFinite(pct) ? Math.min(100, Math.max(0, pct)) : 0;
}

export const APP_PLATFORMS = {
  android: 'Android',
  ios: 'iOS',
  windows: 'Windows',
  macos: 'macOS',
  linux: 'Linux',
} as const;

export type AppPlatform = keyof typeof APP_PLATFORMS;

export function detectPlatform(userAgent: string, maxTouchPoints = 0): AppPlatform {
  if (/iphone|ipad|ipod/i.test(userAgent)) return 'ios';
  if (/android/i.test(userAgent)) return 'android';
  if (/windows/i.test(userAgent)) return 'windows';
  // iPadOS Safari uses a Macintosh user agent in desktop mode.
  if (/macintosh|mac os x/i.test(userAgent)) return maxTouchPoints > 1 ? 'ios' : 'macos';
  if (/linux/i.test(userAgent)) return 'linux';
  return 'android';
}

const HIDDIFY_RELEASES = 'https://github.com/hiddify/hiddify-app/releases';
const HAPP_DESKTOP = 'https://github.com/Happ-proxy/happ-desktop/releases/latest';

const HIDDIFY_DOWNLOADS: Record<AppPlatform, string> = {
  android: 'https://play.google.com/store/apps/details?id=app.hiddify.com',
  ios: 'https://apps.apple.com/app/hiddify-proxy-vpn/id6596777532',
  windows: `${HIDDIFY_RELEASES}/latest/download/Hiddify-Windows-Setup-x64.exe`,
  macos: `${HIDDIFY_RELEASES}/latest/download/Hiddify-MacOS.dmg`,
  linux: `${HIDDIFY_RELEASES}/latest`,
};

const HAPP_DOWNLOADS: Record<AppPlatform, string> = {
  android: 'https://play.google.com/store/apps/details?id=com.happproxy',
  ios: 'https://apps.apple.com/app/happ-proxy-utility/id6504287215',
  windows: `${HAPP_DESKTOP}/download/setup-Happ.x64.exe`,
  macos: `${HAPP_DESKTOP}/download/Happ.macOS.universal.dmg`,
  linux: HAPP_DESKTOP,
};

export function buildSubApps(platform: AppPlatform, subUrl: string, profileName: string) {
  const hiddifyImport = `hiddify://import/${subUrl}#${encodeURIComponent(profileName)}`;

  return [
    { name: 'Hiddify', url: hiddifyImport, downloadUrl: HIDDIFY_DOWNLOADS[platform] },
    { name: 'Hiddify Beta', url: hiddifyImport, downloadUrl: HIDDIFY_RELEASES },
    { name: 'Happ', url: `happ://add/${subUrl}`, downloadUrl: HAPP_DOWNLOADS[platform] },
  ];
}
