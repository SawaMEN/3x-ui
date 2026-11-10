import { protocolCapabilities } from '@/generated/protocol-capabilities';

export const NODE_ELIGIBLE_PROTOCOLS: Readonly<Record<string, true>> = Object.fromEntries(
  Object.entries(protocolCapabilities)
    .filter(([, v]) => v.nodeEligible)
    .map(([k]) => [k, true as const]),
);

export interface NodeCoreInfo {
  coreType?: string | null;
  runningCore?: string | null;
  protocolAvailability?: Record<string, { supported: boolean; ready: boolean }> | null;
}

function normalizeCore(value?: string | null): 'xray' | 'singbox' | '' {
  const normalized = (value ?? '').trim().toLowerCase().replaceAll('-', '');
  if (normalized === 'hiddifycore' || normalized === 'singboxupstream') return 'singbox';
  if (normalized === 'singbox') return 'singbox';
  if (normalized === 'xray') return 'xray';
  return '';
}

// Old nodes do not report coreType. Match the backend's compatibility fallback:
// a known configured core wins; otherwise a known running core is useful, and
// a completely unknown node remains Xray-compatible for legacy behaviour.
export function nodeCoreType(node: NodeCoreInfo): 'xray' | 'singbox' {
  return normalizeCore(node.coreType) || normalizeCore(node.runningCore) || 'xray';
}

export function nodeSupportsProtocol(node: NodeCoreInfo, protocol: string): boolean {
  const capability = protocolCapabilities[protocol as keyof typeof protocolCapabilities];
  if (!capability?.nodeEligible) return false;
  const reported = node.protocolAvailability?.[protocol];
  if (node.protocolAvailability) return reported?.supported === true && reported.ready;
  const raw = (node.coreType || node.runningCore || '').toLowerCase().replaceAll('-', '');
  if (raw === 'singboxupstream') return capability.upstream;
  return nodeCoreType(node) === 'xray' ? capability.xray : capability.native;
}
