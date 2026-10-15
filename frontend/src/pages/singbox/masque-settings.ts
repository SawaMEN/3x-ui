import type { ConfigObject } from './simple-settings';

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

// This native outbound is Cloudflare WARP, not a generic MASQUE client.
// The core registers the Cloudflare device on startup and persists its profile
// via experimental.cache_file.store_masque_config.
export function warpMasqueDefaults(tag: string): ConfigObject {
  return {
    type: 'masque',
    tag,
    use_http2: false,
    use_ipv6: false,
    profile: { recreate: false },
  };
}
