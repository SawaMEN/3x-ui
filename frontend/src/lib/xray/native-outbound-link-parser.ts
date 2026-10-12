/** Core-native share URI import. No Xray form conversion for these protocols. */
type Raw = Record<string, unknown>;

const SCHEMES: Record<string, string> = {
  anytls: 'anytls',
  tuic: 'tuic',
  hysteria: 'hysteria',
  hy: 'hysteria',
  shadowtls: 'shadowtls',
  mieru: 'mieru',
  mierus: 'mieru',
  naive: 'naive',
  'naive+https': 'naive',
  ssh: 'ssh',
  socks: 'socks',
  socks5: 'socks',
  socks5h: 'socks',
  http: 'http',
  https: 'http',
};
const first = (p: URLSearchParams, ...keys: string[]) =>
  keys.map((k) => p.get(k)).find((v) => v !== null && v !== '') ?? undefined;
function number(p: URLSearchParams, ...keys: string[]): number | undefined {
  const value = first(p, ...keys);
  if (!value || !/^\d+(?:\.\d+)?$/.test(value)) return undefined;
  const n = Number(value);
  return Number.isFinite(n) ? n : undefined;
}
function bool(p: URLSearchParams, ...keys: string[]): boolean | undefined {
  const v = first(p, ...keys)?.toLowerCase();
  if (v === undefined) return undefined;
  if (['true', '1', 'yes'].includes(v)) return true;
  if (['false', '0', 'no'].includes(v)) return false;
  return undefined;
}
function tlsOptions(p: URLSearchParams, enabled: boolean): Raw | undefined {
  const key = first(p, 'pbk', 'public_key');
  const sni = first(p, 'sni', 'server_name', 'peer');
  const insecure = bool(p, 'allowInsecure', 'insecure', 'skip-cert-verify');
  const fp = first(p, 'fp', 'fingerprint');
  const alpn = first(p, 'alpn');
  const ech = first(p, 'ech');
  if (!enabled && !key && !sni && insecure === undefined && !fp && !alpn && !ech) return undefined;
  const tls: Raw = { enabled: enabled || !!key };
  if (sni) tls.server_name = sni;
  if (insecure !== undefined) tls.insecure = insecure;
  if (alpn) tls.alpn = alpn.split(',').filter(Boolean);
  if (fp) tls.utls = { enabled: true, fingerprint: fp };
  if (ech) tls.ech = { enabled: true, config: [ech] };
  if (key)
    tls.reality = { enabled: true, public_key: key, short_id: first(p, 'sid', 'short_id') ?? '' };
  return tls;
}
function extraOptions(p: URLSearchParams): Raw | null {
  const extra = first(p, 'extra');
  if (!extra) return {};
  try {
    const obj: unknown = JSON.parse(extra);
    return obj && typeof obj === 'object' && !Array.isArray(obj) ? (obj as Raw) : null;
  } catch {
    return null;
  }
}
export function parseNativeOutboundLink(link: string): Raw | null {
  let url: URL;
  try {
    url = new URL(link.trim());
  } catch {
    return null;
  }
  const scheme = url.protocol.slice(0, -1).toLowerCase();
  const protocol = SCHEMES[scheme];
  if (!protocol || !url.hostname) return null;
  // Remote HTTPS subscription URLs are not HTTP proxy share links.
  if (['http', 'https'].includes(scheme) && url.pathname !== '/') return null;
  const defaultPort =
    scheme === 'ssh' ? 22 : protocol === 'socks' ? 1080 : scheme === 'http' ? 8080 : 443;
  const port = Number(url.port || defaultPort);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return null;
  let username: string, password: string, remark: string;
  try {
    username = decodeURIComponent(url.username);
    password = decodeURIComponent(url.password);
    remark = decodeURIComponent(url.hash.slice(1));
  } catch {
    return null;
  }
  const p = url.searchParams;
  const extra = extraOptions(p);
  if (extra === null) return null;
  const settings: Raw = { server: url.hostname.replace(/^\[|\]$/g, ''), server_port: port };
  const security = first(p, 'security');
  const secureScheme = ['https', 'naive+https', 'mierus'].includes(scheme);
  const nativeTls = ['anytls', 'tuic', 'hysteria', 'naive', 'shadowtls'].includes(protocol);
  const enabled =
    security === 'tls' ||
    security === 'reality' ||
    secureScheme ||
    (security !== 'none' && nativeTls);
  const tls = tlsOptions(p, enabled);
  if (tls && protocol !== 'ssh' && protocol !== 'mieru') settings.tls = tls;
  switch (protocol) {
    case 'anytls':
      settings.password = username || password || first(p, 'password') || '';
      for (const key of ['idle_session_check_interval', 'idle_session_timeout']) {
        const value = first(p, key);
        if (value) settings[key] = value;
      }
      if (number(p, 'min_idle_session') !== undefined)
        settings.min_idle_session = number(p, 'min_idle_session');
      break;
    case 'tuic':
      settings.uuid = username;
      settings.password = password || first(p, 'password') || '';
      settings.congestion_control = first(p, 'congestion_control', 'congestion') ?? 'bbr';
      settings.udp_relay_mode = first(p, 'udp_relay_mode', 'udp_relay') ?? 'native';
      if (bool(p, 'zero_rtt_handshake') !== undefined)
        settings.zero_rtt_handshake = bool(p, 'zero_rtt_handshake');
      if (first(p, 'heartbeat')) settings.heartbeat = first(p, 'heartbeat');
      break;
    case 'hysteria':
      settings.auth_str = username || password || first(p, 'auth', 'auth_str') || '';
      if (number(p, 'up_mbps', 'upmbps', 'up') !== undefined)
        settings.up_mbps = number(p, 'up_mbps', 'upmbps', 'up');
      if (number(p, 'down_mbps', 'downmbps', 'down') !== undefined)
        settings.down_mbps = number(p, 'down_mbps', 'downmbps', 'down');
      if (first(p, 'obfs')) settings.obfs = first(p, 'obfs');
      break;
    case 'shadowtls':
      settings.version = number(p, 'version') ?? 3;
      settings.password = username || password || first(p, 'password') || '';
      break;
    case 'mieru':
      delete settings.server_port;
      settings.username = username;
      settings.password = password;
      settings.portBindings = [
        { protocol: (first(p, 'protocol', 'transport') ?? 'TCP').toUpperCase(), port },
      ];
      if (first(p, 'multiplexing')) settings.multiplexing = first(p, 'multiplexing');
      if (first(p, 'handshake_mode')) settings.handshake_mode = first(p, 'handshake_mode');
      break;
    case 'naive':
    case 'http':
    case 'socks':
      settings.username = username;
      settings.password = password;
      if (protocol === 'socks') settings.version = '5';
      break;
    case 'ssh':
      settings.user = username;
      if (password) settings.password = password;
      break;
  }
  return {
    protocol: 'singbox:' + protocol,
    tag: remark || protocol + '-out',
    settings: { ...settings, ...extra },
  };
}
