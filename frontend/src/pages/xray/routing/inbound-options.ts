import type { RoutingEngine } from './quick-routing';

// Only native sing-box listeners are safe fallbacks when the editor snapshot
// lacks tags. External protocol services need a generated core relay first.
const NATIVE_PROTOCOLS = new Set([
  'vmess',
  'vless',
  'trojan',
  'shadowsocks',
  'wireguard',
  'hysteria',
  'http',
  'mixed',
  'tunnel',
  'tun',
  'tuic',
  'naive',
  'masque',
  'snell',
  'anytls',
  'shadowtls',
]);

interface InboundOption {
  tag?: string;
  protocol?: string;
  enable?: boolean;
  nodeId?: number | null;
}

export function selectableRoutingInboundTags(
  engine: RoutingEngine,
  configTags: string[],
  available: InboundOption[],
): string[] {
  const tags = new Set(configTags.map((tag) => tag.trim()).filter(Boolean));
  for (const inbound of available) {
    const tag = inbound.tag?.trim();
    if (!tag || inbound.enable === false || inbound.nodeId != null) continue;
    if (engine === 'sing-box' && !NATIVE_PROTOCOLS.has(inbound.protocol?.toLowerCase() || '')) {
      continue;
    }
    tags.add(tag);
  }
  return [...tags];
}
