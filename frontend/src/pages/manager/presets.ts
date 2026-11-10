import { createDefaultInboundSettings } from '@/lib/xray/inbound-defaults';
import { fillStreamDefaults } from '@/lib/xray/stream-defaults';
import { isClientAttachableProtocol } from '@/lib/inbounds/client-attachable';
import { Protocols } from '@/schemas/primitives';
import type { ManagerConfig } from './schema';

export const MANAGER_PRESETS = [
  'vless-tcp-reality',
  'vless-xhttp-reality',
  'vless-xhttp-get',
  'vless-grpc-reality',
  'vless-ws',
  'vless-tcp-tls',
  'vmess-tcp',
  'vmess-ws-tls',
  'shadowsocks-tcp',
  'trojan-tcp-reality',
  'trojan-tcp-tls',
  'hysteria2-udp',
  ...Object.values(Protocols).filter(
    (v) =>
      isClientAttachableProtocol(v) &&
      !['vless', 'vmess', 'shadowsocks', 'trojan', 'hysteria'].includes(v),
  ),
];

export function createManagerPreset(kind: string): ManagerConfig {
  const protocol =
    kind === 'hysteria2-udp'
      ? 'hysteria'
      : kind.startsWith('vless-')
        ? 'vless'
        : kind.startsWith('vmess-')
          ? 'vmess'
          : kind.startsWith('shadowsocks-')
            ? 'shadowsocks'
            : kind.startsWith('trojan-')
              ? 'trojan'
              : kind;
  const settings = createDefaultInboundSettings(protocol);
  if (!settings) throw new Error('Unsupported protocol: ' + protocol);
  const network = kind.includes('-xhttp-')
    ? 'xhttp'
    : kind.includes('-grpc-')
      ? 'grpc'
      : kind.includes('-ws')
        ? 'ws'
        : protocol === 'hysteria'
          ? 'hysteria'
          : 'tcp';
  const security = kind.endsWith('-reality')
    ? 'reality'
    : kind === 'vless-ws' || kind.endsWith('-tls') || protocol === 'hysteria'
      ? 'tls'
      : 'none';
  const stream = fillStreamDefaults({ network, security });
  if (security === 'reality')
    stream.realitySettings = {
      show: false,
      xver: 0,
      target: '',
      serverNames: [],
      privateKey: '',
      shortIds: [],
      settings: { publicKey: '', fingerprint: 'firefox', serverName: '', spiderX: '/' },
    };
  if (kind === 'vless-ws') stream.wsSettings = { path: '/ws', host: '' };
  if (kind === 'vless-xhttp-get')
    stream.xhttpSettings = {
      path: '/media/live/segment.ts',
      mode: 'packet-up',
      uplinkHTTPMethod: 'GET',
    };
  return {
    shareAddrStrategy: 'node',
    shareAddr: '',
    subSortIndex: 1,
    protocol,
    settings: JSON.stringify(settings),
    streamSettings: JSON.stringify(stream),
    sniffing: JSON.stringify({
      enabled: false,
      destOverride: ['http', 'tls'],
      metadataOnly: false,
      routeOnly: false,
    }),
    listen: '',
    disableFlow: false,
  };
}
