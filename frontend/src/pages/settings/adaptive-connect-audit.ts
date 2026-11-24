import type { InboundOption } from '@/schemas/client';
import type { SubBalancer } from '@/schemas/subBalancer';

export type AuditIssue =
  | 'disabled'
  | 'static-strategy'
  | 'insufficient-members'
  | 'unavailable-members'
  | 'shared-node'
  | 'node-diversity-unverified'
  | 'single-transport';

const supported = new Set(['shadowsocks', 'vless', 'vmess', 'trojan', 'hysteria', 'wireguard']);

export function auditAdaptiveGroups(groups: SubBalancer[], inbounds: InboundOption[]) {
  const lookup = new Map(inbounds.map((item) => [item.id, item]));
  return groups.map((group) => {
    const ids = [...new Set(group.inboundIds)];
    const members = ids
      .map((id) => lookup.get(id))
      .filter(
        (item): item is InboundOption =>
          !!item && item.enable === true && supported.has(item.protocol ?? ''),
      );
    const nodes = new Set(members.map((item) => item.nodeId ?? 0));
    const transports = new Set(
      members.map((item) => {
        if (item.protocol === 'wireguard' || item.protocol === 'hysteria') return 'udp';
        return item.network || 'unknown';
      }),
    );
    const issues: AuditIssue[] = [];
    if (!group.enabled) issues.push('disabled');
    if (!['leastPing', 'leastLoad'].includes(group.strategy)) issues.push('static-strategy');
    if (members.length < 2) issues.push('insufficient-members');
    if (members.length !== ids.length) issues.push('unavailable-members');
    if (members.length > 1 && nodes.size === 1) issues.push('shared-node');
    if (nodes.size > 1) issues.push('node-diversity-unverified');
    if (members.length > 1 && (transports.size === 1 || transports.has('unknown')))
      issues.push('single-transport');
    return { id: group.id, name: group.remark, members: members.length, nodes: nodes.size, issues };
  });
}
