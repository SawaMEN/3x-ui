import type { RuleObject } from '@/schemas/routing';

const PREFIX = 'panel-inbound:';

// These are fallback routes for an inbound, not replacements for domain/IP rules.
// Keep manually configured specific rules first.
function isCatchAll(rule: RuleObject): boolean {
  if (rule.enabled === false) return false;
  if (!rule.outboundTag && !rule.balancerTag) return false;
  const matchers = [
    'domain',
    'ip',
    'port',
    'sourcePort',
    'localPort',
    'network',
    'sourceIP',
    'localIP',
    'user',
    'vlessRoute',
    'inboundTag',
    'protocol',
    'attrs',
    'process',
    'localOS',
    'routingListId',
  ] as const;
  return matchers.every((key) => {
    const value = rule[key];
    return (
      value == null ||
      value === '' ||
      (Array.isArray(value) && value.length === 0) ||
      (typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length === 0)
    );
  });
}

export function inboundRouteTarget(rules: RuleObject[], inboundTag: string): string | undefined {
  return rules.find(
    (rule) =>
      rule.comment === PREFIX + inboundTag &&
      rule.enabled !== false &&
      rule.inboundTag?.length === 1 &&
      rule.inboundTag[0] === inboundTag,
  )?.outboundTag;
}

export function setInboundRouteTarget(
  rules: RuleObject[],
  inboundTag: string,
  outboundTag: string,
): RuleObject[] {
  if (!inboundTag.trim()) return rules;
  const marker = PREFIX + inboundTag;
  // Reconcile only panel-owned entries; user-authored rules are untouched.
  const previous = rules.findIndex((rule) => rule.comment === marker);
  // The quick editor can also own an inbound-only fallback. When this basic
  // binding is changed, take the listener out of its old quick route instead
  // of leaving an earlier rule that silently overrides the new destination.
  const next = rules.flatMap((rule) => {
    if (rule.comment === marker) return [];
    if (
      outboundTag &&
      typeof rule.ruleTag === 'string' &&
      rule.ruleTag.startsWith('panel-quick:inbound:') &&
      Array.isArray(rule.inboundTag) &&
      rule.inboundTag.includes(inboundTag)
    ) {
      const remaining = rule.inboundTag.filter((tag) => tag !== inboundTag);
      return remaining.length ? [{ ...rule, inboundTag: remaining }] : [];
    }
    return [rule];
  });
  if (!outboundTag) return next;
  const fallback = next.findIndex(isCatchAll);
  // A stale managed rule may already sit after a catch-all. Reposition it
  // before that fallback on every edit, otherwise Xray never reaches it.
  const index =
    fallback >= 0
      ? previous >= 0
        ? Math.min(previous, fallback)
        : fallback
      : previous >= 0
        ? Math.min(previous, next.length)
        : next.length;
  next.splice(index, 0, {
    type: 'field',
    comment: marker,
    inboundTag: [inboundTag],
    outboundTag,
  });
  return next;
}
