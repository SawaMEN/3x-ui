export type ProxyLinkProtocol = 'vless' | 'vmess' | 'trojan' | 'ss' | 'http' | 'https' | 'socks' | 'socks5';

export interface InspectedProxyLink {
  protocol: ProxyLinkProtocol;
  name: string;
  host: string;
  port: number | null;
  credential: string;
  username: string;
  password: string;
  network: string;
  security: string;
  sni: string;
  path: string;
  hostHeader: string;
  fingerprint: string;
  publicKey: string;
  shortId: string;
  params: Record<string, string>;
  issues: string[];
  raw: string;
}

type JsonRecord = Record<string, unknown>;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringField(record: JsonRecord, key: string): string {
  const value = record[key];
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '';
}

function safeDecodeURIComponent(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function decodeBase64Utf8(value: string): string {
  const normalized = value.trim().replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
  let binary: string;
  try {
    binary = atob(padded);
  } catch {
    throw new Error('Invalid base64 payload');
  }
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

function parsePort(value: string): number | null {
  if (!value) return null;
  const port = Number(value);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null;
  return port;
}

function paramsFromSearch(searchParams: URLSearchParams): Record<string, string> {
  const result: Record<string, string> = {};
  searchParams.forEach((value, key) => {
    result[key] = value;
  });
  return result;
}

function nameFromHash(hash: string): string {
  return safeDecodeURIComponent(hash.replace(/^#/, ''));
}

function commonIssues(host: string, port: number | null): string[] {
  const issues: string[] = [];
  if (!host) issues.push('Missing server address');
  if (port === null) issues.push('Missing or invalid server port');
  return issues;
}

function parseUrlLink(raw: string, protocol: Exclude<ProxyLinkProtocol, 'vmess' | 'ss'>): InspectedProxyLink {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new Error(`Invalid ${protocol.toUpperCase()} URI`);
  }

  const params = paramsFromSearch(parsed.searchParams);
  const port = parsePort(parsed.port);
  const username = safeDecodeURIComponent(parsed.username);
  const password = safeDecodeURIComponent(parsed.password);
  const issues = commonIssues(parsed.hostname, port);

  if (protocol === 'vless' && !UUID_RE.test(username)) issues.push('VLESS credential is not a valid UUID');
  if (protocol === 'trojan' && !username) issues.push('Missing Trojan password');

  return {
    protocol,
    name: nameFromHash(parsed.hash),
    host: parsed.hostname,
    port,
    credential: username,
    username,
    password,
    network: params.type ?? '',
    security: params.security ?? (protocol === 'https' ? 'tls' : ''),
    sni: params.sni ?? params.serverName ?? '',
    path: params.path ?? '',
    hostHeader: params.host ?? '',
    fingerprint: params.fp ?? '',
    publicKey: params.pbk ?? '',
    shortId: params.sid ?? '',
    params,
    issues,
    raw,
  };
}

function parseVmess(raw: string): InspectedProxyLink {
  const encoded = raw.slice('vmess://'.length).split('#', 1)[0].trim();
  let decoded: unknown;
  try {
    decoded = JSON.parse(decodeBase64Utf8(encoded));
  } catch (error) {
    throw new Error(`Invalid VMess payload: ${error instanceof Error ? error.message : 'unknown error'}`);
  }
  if (!isRecord(decoded)) throw new Error('Invalid VMess payload: expected a JSON object');

  const host = stringField(decoded, 'add');
  const port = parsePort(stringField(decoded, 'port'));
  const credential = stringField(decoded, 'id');
  const issues = commonIssues(host, port);
  if (!UUID_RE.test(credential)) issues.push('VMess credential is not a valid UUID');

  const params: Record<string, string> = {};
  for (const [key, value] of Object.entries(decoded)) {
    if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
      params[key] = String(value);
    }
  }

  return {
    protocol: 'vmess',
    name: stringField(decoded, 'ps'),
    host,
    port,
    credential,
    username: credential,
    password: '',
    network: stringField(decoded, 'net'),
    security: stringField(decoded, 'tls'),
    sni: stringField(decoded, 'sni'),
    path: stringField(decoded, 'path'),
    hostHeader: stringField(decoded, 'host'),
    fingerprint: stringField(decoded, 'fp'),
    publicKey: stringField(decoded, 'pbk'),
    shortId: stringField(decoded, 'sid'),
    params,
    issues,
    raw,
  };
}

function splitOnce(value: string, separator: string): [string, string] {
  const index = value.indexOf(separator);
  if (index < 0) return [value, ''];
  return [value.slice(0, index), value.slice(index + separator.length)];
}

function parseShadowsocks(raw: string): InspectedProxyLink {
  const body = raw.slice('ss://'.length);
  const [withoutHash, encodedName] = splitOnce(body, '#');
  const [payload, query] = splitOnce(withoutHash, '?');
  const name = safeDecodeURIComponent(encodedName);

  let credentials = '';
  let authority = '';
  const at = payload.lastIndexOf('@');
  if (at >= 0) {
    const userInfo = payload.slice(0, at);
    authority = payload.slice(at + 1).replace(/\/$/, '');
    if (userInfo.includes(':')) {
      credentials = safeDecodeURIComponent(userInfo);
    } else {
      credentials = decodeBase64Utf8(userInfo);
    }
  } else {
    const legacy = decodeBase64Utf8(payload);
    const legacyAt = legacy.lastIndexOf('@');
    if (legacyAt < 0) throw new Error('Invalid Shadowsocks payload: missing server address');
    credentials = legacy.slice(0, legacyAt);
    authority = legacy.slice(legacyAt + 1);
  }

  const colon = credentials.indexOf(':');
  if (colon < 1) throw new Error('Invalid Shadowsocks payload: missing cipher or password');
  const method = credentials.slice(0, colon);
  const password = credentials.slice(colon + 1);

  let endpoint: URL;
  try {
    endpoint = new URL(`ss://x@${authority}`);
  } catch {
    throw new Error('Invalid Shadowsocks server address');
  }
  const port = parsePort(endpoint.port);
  const params = paramsFromSearch(new URLSearchParams(query));
  const issues = commonIssues(endpoint.hostname, port);
  if (!method) issues.push('Missing Shadowsocks cipher');
  if (!password) issues.push('Missing Shadowsocks password');

  return {
    protocol: 'ss',
    name,
    host: endpoint.hostname,
    port,
    credential: `${method}:${password}`,
    username: method,
    password,
    network: '',
    security: '',
    sni: '',
    path: '',
    hostHeader: '',
    fingerprint: '',
    publicKey: '',
    shortId: '',
    params: { method, ...params },
    issues,
    raw,
  };
}

export function inspectProxyLink(input: string): InspectedProxyLink {
  const raw = input.trim();
  if (!raw) throw new Error('Paste a proxy URI first');
  const scheme = raw.match(/^([a-z][a-z0-9+.-]*):\/\//i)?.[1]?.toLowerCase();

  switch (scheme) {
    case 'vless':
    case 'trojan':
    case 'http':
    case 'https':
    case 'socks':
    case 'socks5':
      return parseUrlLink(raw, scheme);
    case 'vmess':
      return parseVmess(raw);
    case 'ss':
      return parseShadowsocks(raw);
    default:
      throw new Error(`Unsupported proxy URI scheme: ${scheme || 'unknown'}`);
  }
}
