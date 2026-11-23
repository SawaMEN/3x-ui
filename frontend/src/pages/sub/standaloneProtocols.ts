import { parseIPv4 } from '@/lib/hosts/host-cidr';

// These protocols ship native clients/sidecars rather than standard
// Xray or sing-box connection profiles. Keep this set aligned with
// internal/sub/standalone_links.go.
const standaloneSchemes = new Set([
  'pingtunnel',
  'fptn',
  'openflux',
  'vetroff',
  'tt',
  'sudoku',
  'naive',
  'naive+https',
  'naive+quic',
]);

export function isStandaloneLink(link: string): boolean {
  const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(link.trim())?.[1]?.toLowerCase();
  return !!scheme && standaloneSchemes.has(scheme);
}

// The panel's pingtunnel:// envelope contains an upstream-native client JSON.
// Do not present it as an importable Xray or sing-box configuration.
export function pingtunnelClientJSON(link: string): string {
  if (!link.startsWith('pingtunnel://')) return '';
  try {
    const raw = link.slice('pingtunnel://'.length).split('#')[0];
    const b64 = raw.replace(/-/g, '+').replace(/_/g, '/');
    const binary = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4));
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    const config: unknown = JSON.parse(new TextDecoder().decode(bytes));
    if (
      !config ||
      typeof config !== 'object' ||
      !('type' in config) ||
      config.type !== 'client' ||
      !('socks5' in config) ||
      config.socks5 !== true ||
      !('password' in config) ||
      typeof config.password !== 'string' ||
      new TextEncoder().encode(config.password).length < 8 ||
      !('server' in config) ||
      typeof config.server !== 'string' ||
      !validPingtunnelServer(config.server) ||
      !('listen' in config) ||
      typeof config.listen !== 'string' ||
      !validPingtunnelListen(config.listen) ||
      !('encrypt' in config) ||
      typeof config.encrypt !== 'string' ||
      !['chacha20', 'aes256', 'aes128'].includes(config.encrypt) ||
      !('congestion' in config) ||
      typeof config.congestion !== 'string' ||
      !['bb', 'none'].includes(config.congestion)
    )
      return '';
    return JSON.stringify(config, null, 2);
  } catch {
    return '';
  }
}

function validPingtunnelServer(server: string): boolean {
  if (server.length === 0 || server.length > 253 || /[/:@?#[\]\\\s]/.test(server)) return false;
  const hostname = server.toLowerCase().replace(/[.]$/, '');
  if (hostname === '' || hostname === 'localhost' || hostname.endsWith('.localhost')) {
    return false;
  }
  const ip = parseIPv4(hostname);
  if (ip !== null && (ip === 0 || ip >>> 24 === 127)) return false;
  return hostname
    .split('.')
    .every(
      (label) =>
        label.length > 0 && label.length <= 63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(label),
    );
}

function validPingtunnelListen(listen: string): boolean {
  const match = /^(?:\[[^\]]+\]|[^:]+):([0-9]+)$/.exec(listen);
  if (!match) return false;
  const port = Number(match[1]);
  return Number.isInteger(port) && port >= 1 && port <= 65535;
}
