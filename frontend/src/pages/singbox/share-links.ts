import { HttpUtil } from '@/utils';
import type { ConfigObject } from './simple-settings';

export function uniqueTag(preferred: string, tags: string[]): string {
  const base = preferred.trim() || 'proxy';
  let candidate = base;
  let suffix = 2;
  while (tags.includes(candidate)) candidate = `${base}-${suffix++}`;
  return candidate;
}

export function parseShareLink(raw: string): ConfigObject {
  if (/^(hy2|hysteria2):\/\//i.test(raw.trim())) return parseHysteria2(raw.trim());
  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    throw new Error('Некорректная ссылка подключения.');
  }
  const protocol = url.protocol.replace(':', '');
  if (!['vless', 'trojan', 'http', 'https', 'socks', 'socks5'].includes(protocol))
    throw new Error('Поддерживаются ссылки VLESS, Trojan, HTTP и SOCKS5.');
  const params = url.searchParams;
  for (const key of ['ech', 'pcs', 'pinSHA256', 'pqv', 'fm']) {
    if (params.get(key))
      throw new Error(`Параметр ${key} не поддерживается этим импортом sing-box.`);
  }
  if (
    params.get('vcn') &&
    params.get('vcn') !== (params.get('sni') || url.hostname.replace(/^\[|\]$/g, ''))
  )
    throw new Error('Отдельное имя проверки сертификата не поддерживается этим импортом.');
  const server = url.hostname.replace(/^\[|\]$/g, '');
  if (!server) throw new Error('В ссылке не указан сервер.');
  const server_port = Number(
    url.port || (protocol === 'http' ? 80 : protocol.startsWith('socks') ? 1080 : 443),
  );
  if (!Number.isInteger(server_port) || server_port < 1 || server_port > 65535)
    throw new Error('Порт должен быть от 1 до 65535.');
  let tag: string;
  let username: string;
  let password: string;
  try {
    tag = decodeURIComponent(url.hash.slice(1)) || server;
    username = decodeURIComponent(url.username);
    password = decodeURIComponent(url.password);
  } catch {
    throw new Error('Некорректное кодирование имени или пароля в ссылке.');
  }
  const next: ConfigObject = {
    type: protocol === 'https' ? 'http' : protocol === 'socks5' ? 'socks' : protocol,
    tag,
    server,
    server_port,
  };
  if (['http', 'https', 'socks', 'socks5'].includes(protocol)) {
    if (username) next.username = username;
    if (password) next.password = password;
    if (protocol === 'https') {
      const tls: ConfigObject = { enabled: true, server_name: params.get('sni') || server };
      if (params.get('alpn')) tls.alpn = params.get('alpn')!.split(',').filter(Boolean);
      if (['1', 'true'].includes(params.get('allowInsecure') || params.get('insecure') || ''))
        tls.insecure = true;
      next.tls = tls;
    }
    return next;
  }
  if (!username)
    throw new Error(protocol === 'vless' ? 'Не указан UUID.' : 'Не указан пароль Trojan.');
  if (protocol === 'vless') {
    if (params.get('encryption')) next.encryption = params.get('encryption');
    next.uuid = username;
    if (params.get('flow')) next.flow = params.get('flow');
  } else {
    const authority = raw.trim().split('://', 2)[1]?.split(/[/?#]/, 1)[0] ?? '';
    next.password = authority.slice(0, authority.lastIndexOf('@')).includes(':')
      ? `${username}:${password}`
      : username;
  }
  const security = params.get('security') || (protocol === 'trojan' ? 'tls' : 'none');
  if (!['none', 'tls', 'reality'].includes(security))
    throw new Error(`Неподдерживаемый режим безопасности: ${security}.`);
  if (security !== 'none') {
    const tls: ConfigObject = { enabled: true, server_name: params.get('sni') || server };
    if (params.get('alpn')) tls.alpn = params.get('alpn')!.split(',').filter(Boolean);
    if (['1', 'true'].includes(params.get('allowInsecure') || params.get('insecure') || ''))
      tls.insecure = true;
    const fingerprint = params.get('fp') || (security === 'reality' ? 'chrome' : '');
    if (fingerprint && fingerprint !== 'unsafe') tls.utls = { enabled: true, fingerprint };
    if (security === 'reality' && fingerprint === 'unsafe')
      throw new Error('Reality требует uTLS fingerprint.');
    if (security === 'reality') {
      if (!params.get('pbk')) throw new Error('В ссылке Reality отсутствует публичный ключ.');
      tls.reality = {
        enabled: true,
        public_key: params.get('pbk'),
        short_id: params.get('sid') || '',
      };
    }
    next.tls = tls;
  } else if (params.get('pbk') || params.get('sid')) {
    throw new Error('Для параметров Reality требуется security=reality.');
  }
  const network = params.get('type') || 'tcp';
  if (!['tcp', 'ws', 'http', 'grpc', 'httpupgrade'].includes(network))
    throw new Error(
      `Транспорт ${network} не поддерживается этим импортом sing-box. Настройте транспорт в JSON редакторе.`,
    );
  if (network === 'tcp' && params.get('headerType') && params.get('headerType') !== 'none')
    throw new Error('TCP-маскировка не поддерживается этим импортом. Настройте транспорт вручную.');
  if (network === 'grpc' && (params.get('mode') === 'multi' || params.get('authority')))
    throw new Error('gRPC multiMode и authority не поддерживаются этим импортом sing-box.');
  if (network !== 'tcp') {
    const transport: ConfigObject = { type: network };
    const host = params.get('host');
    if (['ws', 'http', 'httpupgrade'].includes(network) && params.get('path'))
      transport.path = params.get('path');
    if (network === 'http' && host) transport.host = host.split(',').filter(Boolean);
    if (network === 'ws' && host) transport.headers = { Host: host };
    if (network === 'httpupgrade' && host) transport.host = host;
    if (network === 'grpc' && params.get('serviceName'))
      transport.service_name = params.get('serviceName');
    next.transport = transport;
  }
  return next;
}

function parseHysteria2(raw: string): ConfigObject {
  const match = raw.match(/^(?:hy2|hysteria2):\/\/([^/?#]*)(.*)$/i)!;
  const authority = match[1];
  const at = authority.lastIndexOf('@');
  const auth = at < 0 ? '' : authority.slice(0, at);
  const hostPort = authority.slice(at + 1);
  const parts = hostPort.match(/^(\[[^\]]+\]|[^:]+)(?::(.+))?$/);
  if (!parts) throw new Error('Некорректный сервер Hysteria2.');
  const url = new URL(`https://${parts[1]}${match[2]}`);
  const params = url.searchParams;
  const allowed = new Set([
    'mport',
    'sni',
    'insecure',
    'allowInsecure',
    'alpn',
    'pinSHA256',
    'ech',
    'obfs',
    'obfs-password',
    'obfs-min-packet-size',
    'obfs-max-packet-size',
    'hop-interval',
    'security',
    'fp',
    'vcn',
    'minPacketSize',
    'maxPacketSize',
  ]);
  for (const key of params.keys())
    if (!allowed.has(key))
      throw new Error(`Параметр ${key} не поддерживается этим импортом Hysteria2.`);
  if (
    (params.has('obfs-password') ||
      params.has('obfs-min-packet-size') ||
      params.has('obfs-max-packet-size') ||
      params.has('minPacketSize') ||
      params.has('maxPacketSize')) &&
    !params.get('obfs')
  )
    throw new Error('Параметры маскировки требуют obfs.');
  if (params.get('security') && params.get('security') !== 'tls')
    throw new Error('Hysteria2 требует TLS.');
  if (
    params.get('vcn') &&
    params.get('vcn') !== (params.get('sni') || url.hostname.replace(/^\[|\]$/g, ''))
  )
    throw new Error('Отдельное имя проверки сертификата не поддерживается этим импортом.');
  // fp is a client compatibility hint: QUIC does not use the TCP uTLS fingerprint.
  const ports = (params.get('mport') || parts[2] || '443').split(',').map((part) => {
    if (!/^\d+(?:-\d+)?$/.test(part)) throw new Error('Некорректные порты Hysteria2.');
    const [first, last = first] = part.split('-').map(Number);
    if (first < 1 || last > 65535 || first > last) throw new Error('Некорректные порты Hysteria2.');
    return first === last ? String(first) : `${first}:${last}`;
  });
  const password = decodeURIComponent(auth);
  if (!password) throw new Error('Не указан пароль Hysteria2.');
  const tls: ConfigObject = {
    enabled: true,
    server_name: params.get('sni') || url.hostname.replace(/^\[|\]$/g, ''),
  };
  if (['1', 'true'].includes(params.get('insecure') || params.get('allowInsecure') || ''))
    tls.insecure = true;
  if (params.get('alpn')) tls.alpn = params.get('alpn')!.split(',').filter(Boolean);
  if (params.get('pinSHA256')) {
    tls.certificate_sha256 = params
      .get('pinSHA256')!
      .split(',')
      .map((pin) => {
        const hash = pin.replace(/:/g, '');
        if (!/^[a-f\d]{64}$/i.test(hash)) throw new Error('Некорректный pinSHA256.');
        return btoa(String.fromCharCode(...hash.match(/../g)!.map((byte) => parseInt(byte, 16))));
      });
  }
  if (params.get('ech')) {
    const ech = params.get('ech')!;
    try {
      atob(ech);
    } catch {
      throw new Error('Некорректный ECH config.');
    }
    tls.ech = {
      enabled: true,
      config: [`-----BEGIN ECH CONFIGS-----\n${ech}\n-----END ECH CONFIGS-----`],
    };
  }
  const next: ConfigObject = {
    type: 'hysteria2',
    tag: decodeURIComponent(url.hash.slice(1)) || tls.server_name,
    server: url.hostname.replace(/^\[|\]$/g, ''),
    password,
    tls,
  };
  if (ports.length === 1 && !ports[0].includes(':')) next.server_port = Number(ports[0]);
  else next.server_ports = ports.map((port) => (port.includes(':') ? port : `${port}:${port}`));
  if (params.has('hop-interval')) {
    const interval = params.get('hop-interval')!;
    if (!/^\d+(?:\.\d+)?(?:ms|s|m|h)$/.test(interval) || parseFloat(interval) <= 0)
      throw new Error('Некорректный интервал смены порта.');
    next.hop_interval = interval;
  }
  if (params.get('obfs')) {
    const type = params.get('obfs');
    if (!['salamander', 'gecko'].includes(type!))
      throw new Error('Неподдерживаемая маскировка Hysteria2.');
    const obfs: ConfigObject = { type, password: params.get('obfs-password') };
    if (!obfs.password) throw new Error('Не указан пароль маскировки.');
    for (const [key, field] of [
      ['obfs-min-packet-size', 'min_packet_size'],
      ['obfs-max-packet-size', 'max_packet_size'],
      ['minPacketSize', 'min_packet_size'],
      ['maxPacketSize', 'max_packet_size'],
    ]) {
      if (!params.has(key)) continue;
      const size = Number(params.get(key));
      if (type !== 'gecko' || !Number.isInteger(size) || size < 1 || size > 2048)
        throw new Error('Некорректный размер пакета Gecko.');
      if (obfs[field] !== undefined && obfs[field] !== size)
        throw new Error('Противоречивые параметры Gecko.');
      obfs[field] = size;
    }
    if (Number(obfs.min_packet_size || 1) > Number(obfs.max_packet_size || 2048))
      throw new Error('Некорректный диапазон пакетов Gecko.');
    next.obfs = obfs;
  }
  return next;
}

// Use the same server-side decoder as subscriptions for all other protocols.
export async function importOutboundLink(raw: string): Promise<ConfigObject> {
  const link = raw.trim();
  const result = await HttpUtil.post<ConfigObject>('/panel/api/setting/singbox/import-link', {
    link,
  });
  if (!result?.success || !result.obj || typeof result.obj.type !== 'string')
    throw new Error(result?.msg || 'Не удалось распознать ссылку подключения.');
  return result.obj;
}
