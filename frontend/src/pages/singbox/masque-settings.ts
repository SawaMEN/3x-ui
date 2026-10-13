import type { ConfigObject } from './simple-settings';
import { uniqueTag } from './share-links';

const object = (value: unknown): ConfigObject =>
  value && typeof value === 'object' && !Array.isArray(value) ? (value as ConfigObject) : {};
export const MASQUE_PATH = '/.well-known/masque/ip/{target}/{ipproto}/';
export function masqueClientDefaults(tag: string): ConfigObject {
  return {
    type: 'masque-client',
    tag,
    server_port: 443,
    version: 3,
    path: MASQUE_PATH,
    mtu: 1280,
    system: false,
    tls: { enabled: true },
  };
}
export function addWarpMasque(config: ConfigObject): ConfigObject {
  const outbounds = Array.isArray(config.outbounds) ? config.outbounds : [];
  const endpoints = Array.isArray(config.endpoints) ? config.endpoints : [];
  const tags = [...outbounds, ...endpoints].map((value) => String(object(value).tag || ''));
  const tag = uniqueTag('warp-masque', tags);
  const experimental = object(config.experimental),
    cache = object(experimental.cache_file);
  return {
    ...config,
    outbounds: [
      ...outbounds,
      { type: 'masque', tag, use_http2: false, use_ipv6: false, profile: { recreate: false } },
    ],
    experimental: {
      ...experimental,
      cache_file: { ...cache, enabled: true, store_masque_config: true },
    },
  };
}
