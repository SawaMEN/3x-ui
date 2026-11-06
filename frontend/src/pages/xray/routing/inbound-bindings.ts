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
  const next = rules.filter((rule) => rule.comment !== marker);
  if (!outboundTag) return next;
  const fallback = next.findIndex(isCatchAll);
  const index =
    previous >= 0 ? Math.min(previous, next.length) : fallback >= 0 ? fallback : next.length;
  next.splice(index, 0, {
    type: 'field',
    comment: marker,
    inboundTag: [inboundTag],
    outboundTag,
  });
  return next;
}
