import type { NodeRecord } from '@/schemas/node';
import { nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { createManagerPreset, MANAGER_PRESETS } from './presets';
import { newEntry } from './schema';
import {
  managerConfigWithTLSCertificate,
  managerTLSCertificateForSni,
} from './certificate-recipes';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import type { Subscription } from './schema';

export type CatalogGroup = 'reality' | 'tls' | 'classic' | 'udp' | 'special';
export type SniMode = 'reality' | 'tls' | 'none';

export interface CatalogItem {
  id: string;
  protocol: string;
  group: CatalogGroup;
  name: string;
  description: string;
  nameRu: string;
  descriptionRu: string;
  sniMode: SniMode;
}

const known: Record<string, Omit<CatalogItem, 'id' | 'protocol' | 'sniMode'>> = {
  'vless-tcp-reality': {
    group: 'reality',
    name: 'VLESS · TCP · REALITY',
    nameRu: 'VLESS · TCP · REALITY',
    description: 'Standard REALITY transport without a TLS certificate on the server.',
    descriptionRu: 'REALITY поверх TCP: не нужен TLS-сертификат на сервере.',
  },
  'vless-xhttp-reality': {
    group: 'reality',
    name: 'VLESS · XHTTP · REALITY',
    nameRu: 'VLESS · XHTTP · REALITY',
    description: 'HTTP transport with REALITY. Check core and client compatibility.',
    descriptionRu: 'HTTP-транспорт с REALITY; требуется совместимое ядро и клиент.',
  },
  'vless-grpc-reality': {
    group: 'reality',
    name: 'VLESS · gRPC · REALITY',
    nameRu: 'VLESS · gRPC · REALITY',
    description: 'gRPC transport over REALITY.',
    descriptionRu: 'gRPC-транспорт с маскировкой REALITY.',
  },
  'trojan-tcp-reality': {
    group: 'reality',
    name: 'Trojan · TCP · REALITY',
    nameRu: 'Trojan · TCP · REALITY',
    description: 'Trojan with REALITY; requires compatible core/client.',
    descriptionRu: 'Trojan с REALITY, требуется совместимое ядро и клиент.',
  },
  'vless-ws': {
    group: 'tls',
    name: 'VLESS · WebSocket · TLS',
    nameRu: 'VLESS · WebSocket · TLS',
    description: 'WebSocket TLS transport. Needs a valid certificate on the selected node.',
    descriptionRu: 'WebSocket с TLS. Нужен действительный сертификат на каждой выбранной ноде.',
  },
  'vless-tcp-tls': {
    group: 'tls',
    name: 'VLESS · TCP · TLS',
    nameRu: 'VLESS · TCP · TLS',
    description: 'Direct TLS connection. Configure a certificate in Advanced.',
    descriptionRu: 'Прямое TLS-подключение; сертификат настраивается в расширенных параметрах.',
  },
  'trojan-tcp-tls': {
    group: 'tls',
    name: 'Trojan · TCP · TLS',
    nameRu: 'Trojan · TCP · TLS',
    description: 'Trojan over TLS with a trusted certificate.',
    descriptionRu: 'Trojan поверх TLS с доверенным сертификатом.',
  },
  'vmess-ws-tls': {
    group: 'tls',
    name: 'VMess · WebSocket · TLS',
    nameRu: 'VMess · WebSocket · TLS',
    description: 'VMess over WebSocket and TLS.',
    descriptionRu: 'VMess поверх WebSocket и TLS.',
  },
  'vless-xhttp-get': {
    group: 'classic',
    name: 'VLESS · XHTTP GET',
    nameRu: 'VLESS · XHTTP GET',
    description: 'XHTTP GET transport; review security settings before publishing.',
    descriptionRu: 'XHTTP GET; проверьте защиту транспорта перед публикацией.',
  },
  'vmess-tcp': {
    group: 'classic',
    name: 'VMess · TCP',
    nameRu: 'VMess · TCP',
    description: 'Legacy VMess. Not a substitute for transport encryption.',
    descriptionRu: 'Классический VMess. Не заменяет шифрование транспорта.',
  },
  'shadowsocks-tcp': {
    group: 'classic',
    name: 'Shadowsocks 2022',
    nameRu: 'Shadowsocks 2022',
    description: 'Encrypted TCP/UDP proxy.',
    descriptionRu: 'Зашифрованный прокси TCP/UDP.',
  },
  'hysteria2-udp': {
    group: 'udp',
    name: 'Hysteria2 · UDP',
    nameRu: 'Hysteria2 · UDP',
    description: 'QUIC-based protocol; UDP and TLS certificate are required.',
    descriptionRu: 'Протокол на основе QUIC; необходимы UDP и TLS-сертификат.',
  },
  tuic: {
    group: 'udp',
    name: 'TUIC',
    nameRu: 'TUIC',
    description: 'QUIC-based proxy; review the TLS settings.',
    descriptionRu: 'Прокси на основе QUIC; проверьте настройки TLS.',
  },
  wireguard: {
    group: 'udp',
    name: 'WireGuard',
    nameRu: 'WireGuard',
    description: 'VPN tunnel using UDP.',
    descriptionRu: 'VPN-туннель на UDP.',
  },
  amneziawg: {
    group: 'udp',
    name: 'AmneziaWG',
    nameRu: 'AmneziaWG',
    description: 'WireGuard-derived VPN requiring a compatible client.',
    descriptionRu: 'VPN на базе WireGuard; нужен совместимый клиент.',
  },
  naive: {
    group: 'tls',
    name: 'NaiveProxy',
    nameRu: 'NaiveProxy',
    description: 'HTTPS proxy; server certificate/host are configured in Advanced.',
    descriptionRu: 'HTTPS-прокси; сертификат и адрес настраиваются дополнительно.',
  },
  anytls: {
    group: 'tls',
    name: 'AnyTLS',
    nameRu: 'AnyTLS',
    description: 'TLS-based proxy with native core support.',
    descriptionRu: 'TLS-прокси с поддержкой нативного ядра.',
  },
  shadowtls: {
    group: 'tls',
    name: 'ShadowTLS',
    nameRu: 'ShadowTLS',
    description: 'TLS-looking transport; review upstream settings.',
    descriptionRu: 'Транспорт с имитацией TLS; проверьте настройки upstream.',
  },
};

function classify(group: CatalogGroup, id: string): CatalogGroup {
  if (known[id]) return known[id].group;
  if (['masque', 'wireguard', 'amneziawg', 'tuic'].includes(id)) return 'udp';
  return group;
}

export const catalogGroups: { id: CatalogGroup; name: string; nameRu: string }[] = [
  { id: 'reality', name: 'REALITY', nameRu: 'REALITY' },
  { id: 'tls', name: 'TLS and HTTPS', nameRu: 'TLS и HTTPS' },
  { id: 'classic', name: 'Classic and HTTP transports', nameRu: 'Классические и HTTP-транспорты' },
  { id: 'udp', name: 'UDP and VPN', nameRu: 'UDP и VPN' },
  { id: 'special', name: 'Additional protocols', nameRu: 'Дополнительные протоколы' },
];

export const managerCatalog: CatalogItem[] = MANAGER_PRESETS.map((id) => {
  const configuration = createManagerPreset(id);
  const stream = JSON.parse(configuration.streamSettings) as { security?: string };
  const sniMode: SniMode =
    stream.security === 'reality' ? 'reality' : stream.security === 'tls' ? 'tls' : 'none';
  const info = known[id] ?? {
    group: 'special' as CatalogGroup,
    name: id,
    nameRu: id,
    description: 'Additional protocol: configure its required options before deploying.',
    descriptionRu: 'Дополнительный протокол: перед созданием проверьте его обязательные настройки.',
  };
  return {
    ...info,
    id,
    protocol: configuration.protocol,
    group: classify(info.group, id),
    sniMode,
  };
});

export const MAX_MANAGER_ENTRIES = 100;

export interface RecipeOptions {
  presets: string[];
  nodeIds: number[];
  nodes: NodeRecord[];
  domains: string[];
  sniByPreset: Record<string, string>;
  targetByPreset: Record<string, string>;
  verifiedSniByNode: Record<number, string>;
  backupSniByNode?: Record<number, string[]>;
  managedCertificates?: ManagedSniDomain[];
  // Optional node-level TLS hostname override; "auto" means pick the first
  // valid managed certificate on that node, without inheriting protocol SNI.
  tlsSniByNode?: Record<number, string>;
  // Explicit opt-in: create draft TLS recipes without managed certificates
  // and require the administrator to configure each inbound before preflight.
  allowUnconfiguredTLS?: boolean;
}

export interface RecipeResult {
  entries: Subscription['entries'];
  skipped: string[];
  tooMany: boolean;
}

// A recipe is generated for every supported (protocol, node) pair. A node's
// reported protocol availability takes precedence over local capability guesses.
// Unsupported pairs are skipped rather than generating broken remote configs.
export function buildManagerRecipes(options: RecipeOptions): RecipeResult {
  const entries: Subscription['entries'] = [];
  const skipped: string[] = [];
  for (const nodeId of options.nodeIds) {
    const node = options.nodes.find((item) => item.id === nodeId);
    const nodeName = nodeId === 0 ? 'Local' : node?.name || `#${nodeId}`;
    for (const id of options.presets) {
      const preset = managerCatalog.find((item) => item.id === id);
      if (!preset) {
        skipped.push(`${id} · ${nodeName}: unknown preset`);
        continue;
      }
      if (
        nodeId !== 0 &&
        (!node ||
          !node.enable ||
          node.status === 'offline' ||
          !nodeSupportsProtocol(node, preset.protocol))
      ) {
        skipped.push(`${preset.name} · ${nodeName}: unsupported or offline`);
        continue;
      }
      const configuredSni =
        preset.sniMode === 'tls' && options.tlsSniByNode?.[nodeId] !== undefined
          ? options.tlsSniByNode[nodeId]
          : options.sniByPreset[id] || (preset.sniMode === 'reality' ? 'auto' : '');
      const customTarget = (options.targetByPreset[id] || '').trim();
      const customTargetHost = customTarget.split(':')[0];
      // The certificate presented by a camouflage target normally covers its
      // hostname. Do not randomly choose an unrelated SNI when a target is set.
      const sni =
        preset.sniMode === 'reality'
          ? configuredSni === 'auto'
            ? customTargetHost ||
              options.verifiedSniByNode[nodeId] ||
              (options.domains.length ? 'random' : '')
            : configuredSni
          : preset.sniMode === 'tls'
            ? configuredSni === 'auto'
              ? ''
              : configuredSni
            : '';
      const certificate =
        preset.sniMode === 'tls'
          ? managerTLSCertificateForSni(options.managedCertificates ?? [], nodeId, configuredSni)
          : null;
      if (preset.sniMode === 'tls' && !certificate && !options.allowUnconfiguredTLS) {
        skipped.push(`${preset.name} · ${nodeName}: no certificate for SNI on this node`);
        continue;
      }
      const config = createManagerPreset(id);
      const securedConfig = certificate
        ? managerConfigWithTLSCertificate(config, certificate)
        : config;
      const entry = {
        ...newEntry(),
        mode: 'generate' as const,
        preset: id,
        config: securedConfig,
        templateId: 0,
        nodeId,
        port: 'random',
        sni: certificate?.name ?? sni,
        sniBackups:
          preset.sniMode === 'reality' && !customTarget
            ? (options.backupSniByNode?.[nodeId] ?? []).filter((host) => host !== sni).slice(0, 6)
            : undefined,
        realityTarget:
          preset.sniMode === 'reality' ? (options.targetByPreset[id] || '').trim() : undefined,
        label: `${preset.name} · ${nodeName}`,
        flag: '',
      };
      entries.push(entry);
    }
  }
  return {
    entries: entries.length > MAX_MANAGER_ENTRIES ? [] : entries,
    skipped,
    tooMany: entries.length > MAX_MANAGER_ENTRIES,
  };
}
