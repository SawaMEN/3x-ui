import type { DomainFrontConfig } from '@/generated/zod';
import type { AllSetting } from '@/models/setting';

export function supportsManagedSubscriptionListener(
  config: DomainFrontConfig | undefined,
  settings: AllSetting | undefined,
): boolean {
  return Boolean(
    config?.enabled &&
    settings?.subEnable &&
    !/^(?:127\.|::1$)/.test(settings.subListen) &&
    config.domains.some(
      (domain) =>
        domain.enabled &&
        domain.name === config.subscriptionDomain &&
        ['direct', 'sub_link_only'].includes(domain.mode),
    ),
  );
}

export function automaticSubscriptionOrigin(
  config: DomainFrontConfig | undefined,
  settings?: AllSetting,
): string {
  if (!config?.enabled || !config.subscriptionDomain) return '';
  const port =
    supportsManagedSubscriptionListener(config, settings) &&
    config.publicPort === config.httpsPort &&
    settings?.subDomain.trim().toLowerCase().replace(/\.$/, '') === config.subscriptionDomain
      ? settings.subPort
      : config.publicPort;
  return `https://${config.subscriptionDomain}${port === 443 ? '' : `:${port}`}`;
}

export function subscriptionCaddyConfig(
  settings: Pick<
    AllSetting,
    'subPublicURL' | 'subPort' | 'subDomain' | 'subCertFile' | 'subKeyFile' | 'subAllowedDomains'
  >,
): string {
  let url: URL;
  try {
    url = new URL(settings.subPublicURL);
  } catch {
    return '';
  }
  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.pathname !== '/' ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  )
    return '';
  const hostname = /^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/i;
  if (!hostname.test(url.hostname)) return '';
  const aliases = settings.subAllowedDomains.split(/[,;\s]+/).filter(Boolean);
  if (aliases.some((value) => !hostname.test(value))) return '';
  const sites = [
    url.origin,
    ...aliases.map((alias) => `${url.protocol}//${alias}${url.port ? ':' + url.port : ''}`),
  ];
  const tls = Boolean(settings.subCertFile && settings.subKeyFile);
  const upstream = `${tls ? 'https' : 'http'}://127.0.0.1:${settings.subPort}`;
  const serverName = settings.subDomain || url.hostname;
  if (tls && !hostname.test(serverName)) return '';
  const transport = tls
    ? `\n        transport http {\n            tls_server_name ${serverName}\n        }`
    : '';
  return `${[...new Set(sites)].join(', ')} {\n    header {\n        Cache-Control "private, no-store, max-age=0"\n        CDN-Cache-Control "no-store"\n        Cloudflare-CDN-Cache-Control "no-store"\n    }\n    reverse_proxy ${upstream} {${transport}\n        header_up Host {http.request.host}\n    }\n}`;
}
