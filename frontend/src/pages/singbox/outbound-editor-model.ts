import { nativeOutboundTypes, nativeServerTypes } from './native-import';
import { outboundDefaults, type ConfigObject } from './simple-settings';
export const outboundProtocols = nativeOutboundTypes;
export const tlsProtocols = [
  'vless',
  'vmess',
  'trojan',
  'http',
  'hysteria',
  'hysteria2',
  'tuic',
  'anytls',
  'shadowtls',
  'naive',
  'trusttunnel',
  'masque-client',
];
export const realityProtocols = ['http', 'vmess', 'vless', 'trojan', 'anytls'];
export const requiredTlsProtocols = [
  'hysteria',
  'hysteria2',
  'tuic',
  'anytls',
  'shadowtls',
  'naive',
  'masque-client',
  'trusttunnel',
];
export const transportProtocols = ['vless', 'vmess', 'trojan'];
export const object = (v: unknown): ConfigObject =>
  v && typeof v === 'object' && !Array.isArray(v) ? (v as ConfigObject) : {};
export const text = (v: unknown): string => (typeof v === 'string' ? v : '');
export const strings = (v: unknown): string[] =>
  typeof v === 'string'
    ? [v]
    : Array.isArray(v)
      ? v.filter((s): s is string => typeof s === 'string')
      : [];
export function changeServer(value: ConfigObject, server: string): ConfigObject {
  const tls = object(value.tls);
  const next: ConfigObject = { ...value, server };
  if (tls.enabled && (!tls.server_name || tls.server_name === value.server))
    next.tls = { ...tls, server_name: server };
  return next;
}
export function changeSecurity(value: ConfigObject, mode: string): ConfigObject {
  const next = { ...value };
  if (mode === 'none') {
    delete next.tls;
    if (!next.encryption || next.encryption === 'none') delete next.flow;
    return next;
  }
  const tls: ConfigObject = { ...object(value.tls), enabled: true };
  if (!tls.server_name) tls.server_name = value.server || '';
  if (mode === 'reality') {
    tls.reality = { ...object(tls.reality), enabled: true };
    tls.utls = {
      ...object(tls.utls),
      enabled: true,
      fingerprint: text(object(tls.utls).fingerprint) || 'chrome',
    };
    delete tls.insecure;
  } else delete tls.reality;
  next.tls = tls;
  return next;
}
export function changeProtocol(value: ConfigObject, type: string): ConfigObject {
  const next = outboundDefaults(
    type,
    text(value.tag) === value.type ? type : text(value.tag),
    strings(value.outbounds),
  );
  if (!['bridge', 'block', 'selector', 'urltest', 'balancer'].includes(type) && value.detour)
    next.detour = value.detour;
  if (nativeServerTypes.includes(type)) {
    for (const key of ['server', 'detour']) if (value[key] !== undefined) next[key] = value[key];
    if (
      value.server_port !== undefined &&
      value.server_port !== outboundDefaults(text(value.type), '').server_port
    )
      next.server_port = value.server_port;
    if (type === 'mieru') delete next.server_port;
    if (['vless', 'vmess', 'tuic'].includes(type) && value.uuid) next.uuid = value.uuid;
    if (tlsProtocols.includes(type) && tlsProtocols.includes(text(value.type)) && value.tls)
      next.tls = structuredClone(value.tls);
    if (!realityProtocols.includes(type) && object(next.tls).reality) {
      const tls = { ...object(next.tls) };
      delete tls.reality;
      next.tls = tls;
    }
    if (
      transportProtocols.includes(type) &&
      transportProtocols.includes(text(value.type)) &&
      value.transport
    )
      next.transport = structuredClone(value.transport);
  }
  if (['hysteria', 'hysteria2', 'tuic', 'naive'].includes(type) && next.tls) {
    const tls = { ...object(next.tls) };
    delete tls.utls;
    delete tls.reality;
    next.tls = tls;
  }
  if (requiredTlsProtocols.includes(type) || type === 'trojan') return changeSecurity(next, 'tls');
  return next;
}
export function changeTransport(value: ConfigObject, type: string): ConfigObject {
  const next = { ...value };
  delete next.flow;
  if (type === 'tcp') {
    delete next.transport;
    return next;
  }
  const old = object(value.transport),
    transport: ConfigObject = { type };
  if (type === 'kcp') transport.legacy = true;
  if (['ws', 'http', 'httpupgrade', 'xhttp'].includes(type)) transport.path = text(old.path) || '/';
  const host = text(object(old.headers).Host) || text(old.host) || strings(old.host).join(',');
  if (host) {
    if (type === 'ws') transport.headers = { Host: host };
    else if (type === 'http')
      transport.host = host
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
    else if (['httpupgrade', 'xhttp'].includes(type)) transport.host = host;
  }
  if (type === 'grpc') transport.service_name = text(old.service_name);
  next.transport = transport;
  return next;
}
export function parseOutboundJson(raw: string): ConfigObject {
  const v: unknown = JSON.parse(raw);
  if (!v || typeof v !== 'object' || Array.isArray(v))
    throw new Error('JSON должен содержать одно исходящее подключение.');
  if (!text(object(v).type)) throw new Error('В JSON отсутствует поле type.');
  return v as ConfigObject;
}
