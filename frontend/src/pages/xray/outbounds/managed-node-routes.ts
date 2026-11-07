import type { RouteRecord, RoutingEngine } from '../routing/quick-routing';

export interface ManagedNodeRoute {
  tag: string;
  ruleCount: number;
}

// Only reference live, canonical node-route tags. Their credentials are created
// server-side at runtime and must never be copied into the editable template.
export function managedNodeRoutes(rules: RouteRecord[], engine: RoutingEngine): ManagedNodeRoute[] {
  const references = new Map<string, number>();
  const field = engine === 'xray' ? 'outboundTag' : 'outbound';
  for (const rule of rules) {
    if (rule.enabled === false || rule.action === 'reject') continue;
    const tag = rule[field];
    if (typeof tag !== 'string' || !/^node-route-[1-9][0-9]*$/.test(tag)) continue;
    const id = Number(tag.slice('node-route-'.length));
    if (!Number.isSafeInteger(id) || id <= 0) continue;
    references.set(tag, (references.get(tag) || 0) + 1);
  }
  return [...references].map(([tag, ruleCount]) => ({ tag, ruleCount }));
}
