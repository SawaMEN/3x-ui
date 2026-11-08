import type { TelegramProxyProfile, TelegramSubscription } from '@/generated/types';

export function telegramProfiles(
  data: Partial<TelegramSubscription>,
  links: string[],
): TelegramProxyProfile[] {
  const candidates = [
    ...(data.proxies ?? []),
    ...(data.personal ? [data.personal] : []),
    ...(data.webProxyEnabled && data.webProxy ? [{ link: data.webProxy, name: 'Web Proxy' }] : []),
    ...links.map((link) => ({ link })),
  ];
  const result: TelegramProxyProfile[] = [];
  const seen = new Set<string>();
  for (const profile of candidates) {
    try {
      const url = new URL(profile.link);
      if (url.protocol !== 'tg:' || !['proxy', 'webproxy'].includes(url.hostname)) continue;
      const host = url.searchParams.get('server');
      const secret = url.searchParams.get('secret');
      const port = Number(url.searchParams.get('port'));
      if (
        !host ||
        !secret ||
        (url.hostname === 'proxy' && (!Number.isInteger(port) || port < 1 || port > 65535))
      )
        continue;
      const key = `${url.hostname}|${host.toLowerCase()}|${port}|${secret}`;
      if (seen.has(key)) continue;
      seen.add(key);
      result.push({ ...profile, host, port, tls: secret.startsWith('ee') });
    } catch {
      /* Unrelated VPN links and malformed endpoints do not become Telegram actions. */
    }
  }
  return result;
}
