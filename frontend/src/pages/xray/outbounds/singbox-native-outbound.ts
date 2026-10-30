export type NativeSingBoxOutbound = {
  protocol: string;
  tag: string;
  settings: Record<string, unknown>;
};

const DEFAULT_PROBE_URL = 'https://www.gstatic.com/generate_204';

export function buildNativeSingBoxOutbound(protocol: string, tag = ''): NativeSingBoxOutbound {
  const normalized = protocol.trim().toLowerCase();
  const nativeType = normalized.replace(/^singbox:/, '');
  let settings: Record<string, unknown>;

  switch (nativeType) {
    case 'socks':
      settings = { server: '', server_port: 1080, version: '5' };
      break;
    case 'http':
      settings = { server: '', server_port: 8080 };
      break;
    case 'shadowsocks':
      settings = { server: '', server_port: 443, method: '2022-blake3-aes-128-gcm', password: '' };
      break;
    case 'vmess':
      settings = { server: '', server_port: 443, uuid: '', security: 'auto' };
      break;
    case 'vless':
      settings = { server: '', server_port: 443, uuid: '' };
      break;
    case 'trojan':
      settings = {
        server: '',
        server_port: 443,
        password: '',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'hysteria':
      settings = {
        server: '',
        server_port: 443,
        up_mbps: 100,
        down_mbps: 100,
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'bridge':
      settings = { interface: '' };
      break;
    case 'shadowtls':
      settings = {
        server: '',
        server_port: 443,
        version: 3,
        password: '',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'tuic':
      settings = {
        server: '',
        server_port: 443,
        uuid: '',
        password: '',
        congestion_control: 'bbr',
        udp_relay_mode: 'native',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'hysteria2':
      settings = {
        server: '',
        server_port: 443,
        password: '',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'anytls':
      settings = {
        server: '',
        server_port: 443,
        password: '',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'fptn':
      settings = { token: '', mtu: 1400, sni: 'www.bing.com', bypass: 'sni-spoofing' };
      break;
    case 'vetroff':
      settings = { link: '', enableUDP: false, udpStream: false };
      break;
    case 'openflux':
      settings = {
        secret: '',
        context: '',
        transports: [{ type: 'direct', priority: 100, dial: '' }],
      };
      break;
    case 'snell':
      settings = { server: '', server_port: 443, version: 4, psk: '' };
      break;
    case 'ssh':
      settings = { server: '', server_port: 22, user: '' };
      break;
    case 'selector':
      settings = { outbounds: [], default: '' };
      break;
    case 'urltest':
      settings = { outbounds: [], url: DEFAULT_PROBE_URL, interval: '3m' };
      break;
    case 'mieru':
      settings = {
        server: '',
        username: '',
        password: '',
        portBindings: [{ protocol: 'TCP', port: 443 }],
        mtu: 1400,
        multiplexing: 'MIDDLE',
        handshake_mode: 'STANDARD',
      };
      break;
    case 'naive':
      settings = {
        server: '',
        server_port: 443,
        username: '',
        password: '',
        tls: { enabled: true, server_name: '' },
      };
      break;
    case 'tor':
    default:
      settings = {};
      break;
  }

  return {
    protocol: normalized,
    tag: tag.trim() || `${nativeType}-out`,
    settings,
  };
}

// Accept an outbound copied directly from the official sing-box docs. The
// panel stores a shared Xray-shaped wrapper, so native { type, tag, ... }
// objects are converted to { protocol: "singbox:<type>", tag, settings }.
// The type is intentionally not checked against a panel-side allow-list:
// installed sing-box is the source of truth, so future/custom outbound types
// can be pasted in JSON mode without waiting for a panel release. Known
// removed/invalid types are still rejected by the backend validator.
// Existing panel wrappers without a native `type` are only normalized to
// lowercase and otherwise preserved.
export function normalizeOutboundJsonForPanel(
  raw: Record<string, unknown>,
): Record<string, unknown> {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw))
    throw new Error('Outbound must be an object');
  if ('type' in raw && (typeof raw.type !== 'string' || !raw.type.trim()))
    throw new Error('Invalid native outbound type');
  const type = typeof raw.type === 'string' ? raw.type.trim().toLowerCase() : '';
  if (type) {
    const { type: _type, tag, ...settings } = raw;
    return {
      protocol: `singbox:${type}`,
      tag,
      settings,
    };
  }

  const protocol = typeof raw.protocol === 'string' ? raw.protocol.trim().toLowerCase() : '';
  if (protocol) return { ...raw, protocol };

  return raw;
}

// Normalize a whole JSON import before mutation so a malformed row cannot
// leave a partially imported list. Missing tags receive collision-free names.
export function normalizeOutboundJsonImport(
  value: unknown,
  existingTags: string[] = [],
): Record<string, unknown>[] {
  const object =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  const list = Array.isArray(value)
    ? value
    : object && Array.isArray(object.outbounds)
      ? object.outbounds
      : object && ('type' in object || 'protocol' in object)
        ? [object]
        : null;
  if (!list || list.length === 0) throw new Error('Import requires at least one outbound');
  const used = new Set(existingTags);
  return list.map((item, index) => {
    if (!item || typeof item !== 'object' || Array.isArray(item))
      throw new Error(`Outbound ${index + 1} must be an object`);
    const normalized = normalizeOutboundJsonForPanel(item as Record<string, unknown>);
    if (typeof normalized.protocol !== 'string' || !normalized.protocol)
      throw new Error(`Outbound ${index + 1} has no protocol`);
    if (
      normalized.settings != null &&
      (typeof normalized.settings !== 'object' || Array.isArray(normalized.settings))
    )
      throw new Error(`Outbound ${index + 1} settings must be an object`);
    let tag = normalized.tag;
    if (tag == null || tag === '') {
      const base = `${normalized.protocol.replace(/^singbox:/, '')}-out`;
      tag = base;
      for (let suffix = 2; used.has(tag as string); suffix++) tag = `${base}-${suffix}`;
    }
    if (typeof tag !== 'string' || !tag.trim() || tag !== tag.trim())
      throw new Error(`Outbound ${index + 1} has invalid tag`);
    if (used.has(tag)) throw new Error(`Duplicate outbound tag: ${tag}`);
    used.add(tag);
    return { ...normalized, tag };
  });
}
