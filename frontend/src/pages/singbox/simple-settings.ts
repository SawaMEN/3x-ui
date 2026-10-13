import { masqueClientDefaults } from './masque-settings';
import { isNativeEndpoint, nativeServerTypes } from './native-import';
export type ConfigObject = Record<string, unknown>;

export function object(value: unknown): ConfigObject {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as ConfigObject)
    : {};
}

export function records(value: unknown): ConfigObject[] {
  return Array.isArray(value) ? value.map(object) : [];
}

export const DNS_PRESETS = {
  system: { type: 'local' },
  cloudflare: {
    type: 'https',
    server: '1.1.1.1',
    server_port: 443,
    path: '/dns-query',
    tls: { enabled: true, server_name: 'cloudflare-dns.com' },
  },
  quad9: {
    type: 'https',
    server: '9.9.9.9',
    server_port: 443,
    path: '/dns-query',
    tls: { enabled: true, server_name: 'dns.quad9.net' },
  },
} satisfies Record<string, ConfigObject>;

export type DnsPreset = keyof typeof DNS_PRESETS;

function stableJson(value: unknown): string {
  if (Array.isArray(value)) return '[' + value.map(stableJson).join(',') + ']';
  if (value !== null && typeof value === 'object') {
    return (
      '{' +
      Object.keys(value)
        .sort()
        .map((key) => JSON.stringify(key) + ':' + stableJson(object(value)[key]))
        .join(',') +
      '}'
    );
  }
  return JSON.stringify(value) ?? 'null';
}

function matchesPreset(server: ConfigObject, preset: ConfigObject): boolean {
  const { tag: _tag, ...settings } = server;
  return stableJson(settings) === stableJson(preset);
}

export function currentDnsPreset(config: ConfigObject): DnsPreset | 'custom' {
  const dns = object(config.dns);
  const servers = records(dns.servers);
  const selected =
    servers.find((server) => server.tag === dns.final) ?? (dns.final ? undefined : servers[0]);
  return (
    (Object.keys(DNS_PRESETS) as DnsPreset[]).find(
      (key) => selected && matchesPreset(selected, DNS_PRESETS[key]),
    ) ?? 'custom'
  );
}

export function applyDnsPreset(config: ConfigObject, key: DnsPreset): ConfigObject {
  const dns = object(config.dns);
  const servers = records(dns.servers);
  const preset = DNS_PRESETS[key];
  const existing = servers.find(
    (server) => typeof server.tag === 'string' && server.tag && matchesPreset(server, preset),
  );
  let tag = existing?.tag as string | undefined;
  if (!tag) {
    const base = `panel-dns-${key}`;
    tag = base;
    let suffix = 2;
    while (servers.some((server) => server.tag === tag)) tag = `${base}-${suffix++}`;
  }
  const route = object(config.route);
  const resolver = route.default_domain_resolver;
  const previousTag = dns.final || servers[0]?.tag;
  const resolverObject = object(resolver);
  const nextResolver =
    !resolver || resolver === previousTag
      ? tag
      : resolverObject.server && resolverObject.server === previousTag
        ? { ...resolverObject, server: tag }
        : resolver;
  return {
    ...config,
    dns: { ...dns, servers: existing ? servers : [...servers, { ...preset, tag }], final: tag },
    route: {
      ...route,
      default_domain_resolver: nextResolver,
    },
  };
}

export function optimisticCacheEnabled(dns: ConfigObject): boolean {
  return dns.optimistic === true || object(dns.optimistic).enabled === true;
}

export function setDnsCache(config: ConfigObject, enabled: boolean): ConfigObject {
  const dns: ConfigObject = { ...object(config.dns), disable_cache: !enabled };
  // Optimistic caching cannot be used without the normal DNS cache.
  if (!enabled && optimisticCacheEnabled(dns)) dns.optimistic = false;
  return { ...config, dns };
}

export function setOptimisticCache(config: ConfigObject, enabled: boolean): ConfigObject {
  const dns = object(config.dns);
  const optimistic = object(dns.optimistic);
  return {
    ...config,
    dns: {
      ...dns,
      optimistic: Object.keys(optimistic).length ? { ...optimistic, enabled } : enabled,
      ...(enabled ? { disable_cache: false, disable_expire: false } : {}),
    },
  };
}

export function setLogLevel(config: ConfigObject, level: string): ConfigObject {
  const log = object(config.log);
  return {
    ...config,
    log: {
      ...log,
      disabled: level === 'off',
      ...(level !== 'off' ? { level } : {}),
    },
  };
}

export function setDefaultOutbound(config: ConfigObject, tag: string): ConfigObject {
  const route = { ...object(config.route) };
  if (tag) route.final = tag;
  else delete route.final;
  return { ...config, route };
}

export function outboundDefaults(type: string, tag: string, group: string[] = []): ConfigObject {
  const base = { type, tag };
  if (type === 'masque-client') return masqueClientDefaults(tag);
  if (type === 'masque')
    return { ...base, use_http2: false, use_ipv6: false, profile: { recreate: false } };
  if (type === 'selector') return { ...base, outbounds: group };
  if (type === 'balancer') return { ...base, outbounds: group, strategy: 'round-robin' };
  if (type === 'urltest')
    return {
      ...base,
      outbounds: group,
      url: 'https://www.gstatic.com/generate_204',
      interval: '3m',
      tolerance: 50,
    };
  if (!nativeServerTypes.includes(type)) return base;
  return {
    ...base,
    ...(type === 'mieru'
      ? { portBindings: [{ protocol: 'TCP', port: 443 }] }
      : {
          server_port: type === 'ssh' ? 22 : type === 'socks' ? 1080 : type === 'http' ? 80 : 443,
        }),
    ...(type === 'vmess' ? { security: 'auto' } : {}),
    ...(type === 'shadowsocks' ? { method: 'aes-128-gcm' } : {}),
    ...(type === 'shadowtls' ? { version: 3 } : {}),
    ...(type === 'snell' ? { version: 4 } : {}),
    ...(type === 'hysteria' ? { up_mbps: 100, down_mbps: 100 } : {}),
    ...(['trojan', 'hysteria', 'hysteria2', 'tuic', 'anytls', 'shadowtls', 'naive'].includes(type)
      ? { tls: { enabled: true } }
      : {}),
  };
}

export function outboundErrors(value: ConfigObject, otherTags: string[]): string[] {
  const errors: string[] = [];
  const tag = typeof value.tag === 'string' ? value.tag.trim() : '';
  const type = typeof value.type === 'string' ? value.type : '';
  if (!tag) errors.push('Укажите название подключения.');
  else if (otherTags.includes(tag))
    errors.push('Это название уже используется другим подключением или туннелем.');
  if (!type) errors.push('Выберите протокол.');
  if (type === 'masque-client') {
    if (typeof value.server !== 'string' || !value.server.trim())
      errors.push('Укажите MASQUE-сервер.');
    if (
      !Number.isInteger(value.server_port) ||
      Number(value.server_port) < 1 ||
      Number(value.server_port) > 65535
    )
      errors.push('Порт MASQUE должен быть от 1 до 65535.');
    if (![0, 1, 2, 3].includes(Number(value.version || 0))) errors.push('Выберите версию HTTP.');
    if (object(value.tls).enabled !== true) errors.push('MASQUE требует TLS.');
    if (String(value.username || '').includes(':'))
      errors.push('Имя пользователя MASQUE не может содержать двоеточие.');
    if (
      value.mtu &&
      (!Number.isInteger(value.mtu) || Number(value.mtu) < 1280 || Number(value.mtu) > 65535)
    )
      errors.push('MTU MASQUE должен быть от 1280 до 65535.');
  }
  if (isNativeEndpoint(value)) return errors;
  if (['dns', 'tun', 'redirect', 'tproxy'].includes(type))
    errors.push('Этот тип не поддерживается как исходящее подключение текущим sing-box.');
  if (
    [
      'vless',
      'vmess',
      'trojan',
      'shadowsocks',
      'http',
      'socks',
      'hysteria2',
      'tuic',
      'shadowtls',
      'ssh',
      'anytls',
      'naive',
      'snell',
      'hysteria',
    ].includes(type)
  ) {
    if (typeof value.server !== 'string' || !value.server.trim()) errors.push('Укажите сервер.');
    if (
      !(
        type === 'hysteria2' &&
        (Array.isArray(value.server_ports) || typeof value.server_ports === 'string') &&
        (typeof value.server_ports === 'string' ? [value.server_ports] : value.server_ports)
          .length > 0 &&
        (typeof value.server_ports === 'string' ? [value.server_ports] : value.server_ports).every(
          (port) =>
            /^\d+(?::\d+)?$/.test(String(port)) &&
            String(port)
              .split(':')
              .every((n) => Number(n) >= 1 && Number(n) <= 65535) &&
            Number(String(port).split(':')[0]) <= Number(String(port).split(':').at(-1)),
        )
      ) &&
      (!Number.isInteger(value.server_port) ||
        Number(value.server_port) < 1 ||
        Number(value.server_port) > 65535)
    )
      errors.push('Порт должен быть целым числом от 1 до 65535.');
  }
  if (
    ['vless', 'vmess', 'tuic'].includes(type) &&
    (typeof value.uuid !== 'string' ||
      !/^[\da-f]{8}(-[\da-f]{4}){3}-[\da-f]{12}$/i.test(value.uuid))
  )
    errors.push('Укажите корректный UUID.');
  if (type === 'mieru') {
    if (typeof value.server !== 'string' || !value.server.trim()) errors.push('Укажите сервер.');
    if (
      typeof value.username !== 'string' ||
      typeof value.password !== 'string' ||
      !value.username ||
      !value.password ||
      new TextEncoder().encode(value.username).length > 64 ||
      new TextEncoder().encode(value.password).length > 64
    )
      errors.push('Mieru требует логин и пароль длиной 1–64 байта.');
    if (
      value.mtu &&
      (!Number.isInteger(value.mtu) || Number(value.mtu) < 1280 || Number(value.mtu) > 1500)
    )
      errors.push('MTU Mieru должен быть от 1280 до 1500.');
    if (
      value.multiplexing &&
      !['DEFAULT', 'OFF', 'LOW', 'MIDDLE', 'HIGH'].includes(
        String(value.multiplexing).replace(/^MULTIPLEXING_/, ''),
      )
    )
      errors.push('Неподдерживаемый режим multiplexing Mieru.');
    if (
      value.handshake_mode &&
      !['DEFAULT', 'STANDARD', 'NO_WAIT'].includes(
        String(value.handshake_mode).replace(/^HANDSHAKE_/, ''),
      )
    )
      errors.push('Неподдерживаемый handshake Mieru.');
    const bindings = Array.isArray(value.portBindings) ? value.portBindings : [];
    if (
      !bindings.length ||
      bindings.some((item) => {
        const binding = object(item);
        if (!['TCP', 'UDP'].includes(String(binding.protocol).toUpperCase())) return true;
        if (binding.portRange) {
          const range = String(binding.portRange);
          const [first, last] = range.split('-').map(Number);
          return (
            Boolean(binding.port) ||
            !/^\d+-\d+$/.test(range) ||
            first < 1 ||
            last > 65535 ||
            first > last
          );
        }
        return (
          !Number.isInteger(binding.port) ||
          Number(binding.port) < 1 ||
          Number(binding.port) > 65535
        );
      })
    )
      errors.push('Укажите корректные TCP/UDP порты Mieru.');
    if (
      value.server_port &&
      (bindings.length !== 1 || object(bindings[0]).port !== value.server_port)
    )
      errors.push('server_port не соответствует portBindings Mieru.');
  }
  if (['trojan', 'shadowsocks', 'hysteria2', 'anytls'].includes(type) && !value.password)
    errors.push('Укажите пароль.');
  if (type === 'shadowsocks' && !value.method) errors.push('Выберите метод шифрования.');
  if (
    ['hysteria', 'hysteria2', 'tuic', 'shadowtls', 'anytls', 'naive'].includes(type) &&
    object(value.tls).enabled !== true
  )
    errors.push('Для этого протокола требуется TLS.');
  const tls = object(value.tls);
  if (['hysteria', 'hysteria2', 'tuic', 'naive'].includes(type) && object(tls.utls).enabled)
    errors.push('Этот протокол не поддерживает uTLS.');
  if (object(tls.reality).enabled && (tls.enabled !== true || !object(tls.reality).public_key))
    errors.push('Reality требует TLS и публичный ключ.');
  if (
    type === 'vless' &&
    value.flow &&
    tls.enabled !== true &&
    (!value.encryption || value.encryption === 'none')
  )
    errors.push('Vision требует TLS, Reality или шифрование VLESS.');
  if (type === 'snell' && (!value.psk || ![4, 6].includes(Number(value.version))))
    errors.push('Snell требует PSK и версию 4 или 6.');
  if (
    type === 'hysteria' &&
    ((!value.up && Number(value.up_mbps || 0) <= 0) ||
      (!value.down && Number(value.down_mbps || 0) <= 0))
  )
    errors.push('Укажите скорость отправки и загрузки Hysteria.');
  if (['selector', 'urltest', 'balancer'].includes(type)) {
    const members = Array.isArray(value.outbounds) ? value.outbounds : [];
    if (!members.length) errors.push('Выберите подключения для группы.');
    if (members.some((member) => member === tag || !otherTags.includes(String(member))))
      errors.push(
        'Группа должна ссылаться на существующие подключения и не может ссылаться на себя.',
      );
  }
  return errors;
}
