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
  'tg',
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
      config.password.length < 8 ||
      !('server' in config) ||
      typeof config.server !== 'string' ||
      !/^(?=.{1,253}$)[a-z0-9][a-z0-9.-]*[a-z0-9]$/i.test(config.server) ||
      config.server.includes('..') ||
      config.server === '0.0.0.0' ||
      config.server === '127.0.0.1' ||
      !('listen' in config) ||
      typeof config.listen !== 'string' ||
      !/^.+:\\d+$/.test(config.listen) ||
      !('encrypt' in config) ||
      !['chacha20', 'aes256', 'aes128'].includes(String(config.encrypt)) ||
      !('congestion' in config) ||
      !['bb', 'none'].includes(String(config.congestion))
    )
      return '';
    return JSON.stringify(config, null, 2);
  } catch {
    return '';
  }
}
