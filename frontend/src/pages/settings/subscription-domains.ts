import type { DomainFrontConfig } from '@/generated/zod';
import type { AllSetting } from '@/models/setting';

export function configuredSubscriptionDomain(config: DomainFrontConfig | undefined) {
  if (!config?.enabled || !config.subscriptionDomain) return undefined;
  return config.domains.find(
    (domain) =>
      domain.enabled &&
      domain.name === config.subscriptionDomain &&
      ['direct', 'sub_link_only', 'cdn', 'relay'].includes(domain.mode),
  );
}

export function automaticSubscriptionOrigin(
  config: DomainFrontConfig | undefined,
  settings?: AllSetting,
): string {
  const domain = configuredSubscriptionDomain(config);
  if (!config || !domain) return '';
  let port = config.publicPort;
  if (
    settings?.subEnable &&
    !/^(?:127\.|::1$)/.test(settings.subListen) &&
    ['direct', 'sub_link_only'].includes(domain.mode)
  ) {
    if (
      config.publicPort === config.httpsPort &&
      settings.subDomain.trim().toLowerCase().replace(/\.$/, '') === domain.name
    ) {
      port = settings.subPort;
    } else {
      try {
        const saved = new URL(settings.subPublicURL);
        if (
          saved.protocol === 'https:' &&
          saved.hostname.replace(/\.$/, '') === domain.name &&
          Number(saved.port || 443) === settings.subPort
        ) {
          port = settings.subPort;
        }
      } catch {
        // There is no direct listener override.
      }
    }
  }
  return `https://${domain.name}${port === 443 ? '' : `:${port}`}`;
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
