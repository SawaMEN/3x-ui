import { HttpUtil } from '@/utils';
import type { ConfigObject } from './simple-settings';
import { uniqueTag } from './share-links';

// Audited against the stable kernel's include/registry.go at 3827d3a871dd.
// Native JSON remains open-ended so new releases do not require a panel update.
export const nativeOutboundTypes = [
  'direct',
  'bridge',
  'block',
  'selector',
  'urltest',
  'balancer',
  'socks',
  'http',
  'shadowsocks',
  'snell',
  'vmess',
  'trojan',
  'naive',
  'tor',
  'ssh',
  'shadowtls',
  'vless',
  'vless-reverse',
  'wireguard_legacy',
  'anytls',
  'psiphon',
  'mieru',
  'sudoku',
  'openflux',
  'fptn',
  'pingtunnel',
  'xray',
  'dnstt',
  'gooserelay',
  'masque',
  'trusttunnel',
  'hysteria',
  'hysteria2',
  'tuic',
  'tailcat',
];
export const nativeEndpointTypes = [
  'wireguard',
  'awg',
  'warp',
  'tailscale',
  'openconnect',
  'openvpn-client',
  'openvpn-server',
  'masque-client',
  'masque-server',
  'tunnel_client',
  'tunnel_server',
];
export const nativeServerTypes = [
  'socks',
  'http',
  'shadowsocks',
  'snell',
  'vmess',
  'trojan',
  'naive',
  'ssh',
  'shadowtls',
  'vless',
  'anytls',
  'mieru',
  'sudoku',
  'fptn',
  'trusttunnel',
  'hysteria',
  'hysteria2',
  'tuic',
  'wireguard_legacy',
  'warp',
  'masque-client',
  'openvpn-client',
  'openconnect',
  'pingtunnel',
];
export const isNativeEndpoint = (value: ConfigObject) =>
  nativeEndpointTypes.includes(String(value.type));
export interface NativeImportBundle {
  outbounds: ConfigObject[];
  endpoints: ConfigObject[];
}
export async function importNativeConnections(raw: string): Promise<ConfigObject[]> {
  const result = await HttpUtil.post<NativeImportBundle>('/panel/api/setting/singbox/import-link', {
    link: raw.trim(),
    batch: true,
  });
  if (!result?.success || !result.obj)
    throw new Error(result?.msg || 'Не удалось распознать подключение.');
  const list = [...(result.obj.outbounds || []), ...(result.obj.endpoints || [])];
  if (!list.length || list.some((item) => !item || typeof item.type !== 'string'))
    throw new Error('Импорт не содержит подключений.');
  return list;
}

export function renameImportedConnections(
  items: ConfigObject[],
  existing: string[],
): ConfigObject[] {
  const used = [...existing],
    replacements = new Map<string, string>();
  const renamed = items.map<ConfigObject>((item) => {
    const old = typeof item.tag === 'string' ? item.tag : String(item.type);
    const tag = uniqueTag(old, used);
    used.push(tag);
    if (replacements.has(old))
      throw new Error('Импорт содержит повторяющиеся названия подключений.');
    replacements.set(old, tag);
    return { ...structuredClone(item), tag };
  });
  return renamed.map((item) => rewriteNativeReferences(item, replacements) as ConfigObject);
}
export function nativeConnectionLink(value: ConfigObject): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value));
  return (
    'singbox://' +
    btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '')
  );
}

export function rewriteNativeReferences(
  value: unknown,
  names: ReadonlyMap<string, string>,
): unknown {
  if (Array.isArray(value)) return value.map((item) => rewriteNativeReferences(item, names));
  if (!value || typeof value !== 'object') return value;
  const next: ConfigObject = {};
  for (const [key, item] of Object.entries(value)) {
    if (
      ['detour', 'default', 'outbound', 'final', 'portal_tag', 'bridge_tag'].includes(key) &&
      typeof item === 'string'
    )
      next[key] = names.get(item) || item;
    else if (key === 'outbounds' && Array.isArray(item))
      next[key] = item.map((member) =>
        typeof member === 'string'
          ? names.get(member) || member
          : rewriteNativeReferences(member, names),
      );
    else if (key === 'headers') next[key] = item;
    else next[key] = rewriteNativeReferences(item, names);
  }
  return next;
}
