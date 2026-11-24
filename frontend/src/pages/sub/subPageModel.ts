const DAY_MS = 86_400_000;

type ProfileLinks = {
  links?: string[];
  telegramRemarks?: Record<string, string>;
  subUrl?: string;
  subJsonUrl?: string;
  subClashUrl?: string;
  subFormatMode?: string;
};

export function sameProfileLinks(left: ProfileLinks, right: ProfileLinks): boolean {
  if (
    left.subUrl !== right.subUrl ||
    left.subJsonUrl !== right.subJsonUrl ||
    left.subClashUrl !== right.subClashUrl ||
    left.subFormatMode !== right.subFormatMode
  )
    return false;
  const a = left.links ?? [];
  const b = right.links ?? [];
  if (a.length !== b.length || a.some((link, i) => link !== b[i])) return false;
  const remarks = left.telegramRemarks ?? {};
  const next = right.telegramRemarks ?? {};
  return (
    Object.keys(remarks).length === Object.keys(next).length &&
    Object.keys(remarks).every((key) => Object.hasOwn(next, key) && remarks[key] === next[key])
  );
}

export function parseSubscriptionURL(raw: string, base?: string): URL | undefined {
  if (!raw) return undefined;
  try {
    const url = new URL(raw, base);
    return url.protocol === 'http:' || url.protocol === 'https:' ? url : undefined;
  } catch {
    return undefined;
  }
}

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
const HIDDIFY_LATEST = `${HIDDIFY_RELEASES}/latest`;
const HAPP_DESKTOP = 'https://github.com/Happ-proxy/happ-desktop/releases/latest';
const HIDDIFY_APP_STORE = 'https://apps.apple.com/app/hiddify-proxy-vpn/id6596777532';
const HAPP_APP_STORE = 'https://apps.apple.com/app/happ-proxy-utility/id6504287215';
const AMNEZIAWG_APP_STORE = 'https://apps.apple.com/app/amneziawg/id6478942365';

interface AppDownload {
  label: string;
  href: string;
}

const HIDDIFY_DOWNLOADS: Record<AppPlatform, AppDownload[]> = {
  android: [
    { label: 'Google Play', href: 'https://play.google.com/store/apps/details?id=app.hiddify.com' },
  ],
  ios: [{ label: 'App Store', href: `${HIDDIFY_APP_STORE}?platform=iphone` }],
  windows: [
    { label: 'Microsoft Store', href: 'https://apps.microsoft.com/detail/9ndgwm32bnxk' },
    { label: '.exe', href: `${HIDDIFY_LATEST}/download/Hiddify-Windows-Setup-x64.exe` },
  ],
  macos: [
    { label: 'App Store', href: `${HIDDIFY_APP_STORE}?platform=mac` },
    { label: '.dmg', href: `${HIDDIFY_LATEST}/download/Hiddify-MacOS.dmg` },
  ],
  linux: [{ label: 'GitHub', href: HIDDIFY_LATEST }],
};

const HAPP_DOWNLOADS: Record<AppPlatform, AppDownload[]> = {
  android: [
    { label: 'Google Play', href: 'https://play.google.com/store/apps/details?id=com.happproxy' },
  ],
  ios: [{ label: 'App Store', href: `${HAPP_APP_STORE}?platform=iphone` }],
  windows: [{ label: 'GitHub', href: HAPP_DESKTOP }],
  macos: [
    { label: 'App Store', href: `${HAPP_APP_STORE}?platform=mac` },
    { label: '.dmg', href: `${HAPP_DESKTOP}/download/Happ.macOS.universal.dmg` },
  ],
  linux: [{ label: 'GitHub', href: HAPP_DESKTOP }],
};

const AMNEZIAWG_DOWNLOADS: Record<AppPlatform, AppDownload[]> = {
  android: [
    { label: 'Google Play', href: 'https://play.google.com/store/apps/details?id=org.amnezia.awg' },
  ],
  ios: [{ label: 'App Store', href: `${AMNEZIAWG_APP_STORE}?platform=iphone` }],
  windows: [
    {
      label: 'GitHub',
      href: 'https://github.com/amnezia-vpn/amneziawg-windows-client/releases/latest',
    },
  ],
  macos: [{ label: 'App Store', href: `${AMNEZIAWG_APP_STORE}?platform=mac` }],
  linux: [{ label: 'GitHub', href: 'https://github.com/amnezia-vpn/amneziawg-tools' }],
};

export function buildSubApps(platform: AppPlatform, subUrl: string, profileName: string) {
  const hiddifyUrl = parseSubscriptionURL(subUrl);
  if (hiddifyUrl) {
    hiddifyUrl.searchParams.delete('format');
    hiddifyUrl.hash = '';
  }
  const hiddifyImport = hiddifyUrl
    ? `hiddify://import/?url=${encodeURIComponent(hiddifyUrl.href)}#${encodeURIComponent(profileName)}`
    : undefined;

  return [
    { name: 'Hiddify', url: hiddifyImport, downloads: HIDDIFY_DOWNLOADS[platform] },
    {
      name: 'Hiddify Beta',
      url: hiddifyImport,
      downloads: [{ label: 'GitHub', href: HIDDIFY_RELEASES }],
    },
    {
      name: 'Happ',
      url: hiddifyUrl ? `happ://add/${subUrl}` : undefined,
      downloads: HAPP_DOWNLOADS[platform],
    },
    { name: 'AmneziaWG', url: undefined, downloads: AMNEZIAWG_DOWNLOADS[platform] },
  ];
}
