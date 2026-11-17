import type { RuleObject } from '@/schemas/routing';

export interface CatalogSelection {
  name: string;
  token?: string;
  listId?: number;
}

export function catalogRule(selection: CatalogSelection, outboundTag: string): RuleObject {
  if (!outboundTag || (!selection.token && !selection.listId)) {
    throw new Error('Select a list and an outbound');
  }
  return {
    type: 'field',
    enabled: true,
    comment: selection.name,
    outboundTag,
    ...(selection.listId
      ? { routingListId: selection.listId, ruleTag: `catalog-list-${selection.listId}` }
      : { domain: [selection.token!], ruleTag: `catalog-${selection.token}` }),
  };
}

const MATCHERS = [
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

export function isCatchAll(rule: RuleObject): boolean {
  return (
    rule.enabled !== false &&
    !MATCHERS.some((key) => {
      const value = rule[key];
      if (Array.isArray(value)) return value.length > 0;
      if (typeof value === 'object' && value) return Object.keys(value).length > 0;
      return value !== undefined && value !== '';
    })
  );
}

// Keep existing specific rules and service routes ahead of catalog additions.
// Reapplying a category updates its target in place rather than duplicating it.
export function putCatalogRule(rules: RuleObject[], next: RuleObject): RuleObject[] {
  const same = (rule: RuleObject) =>
    next.routingListId ? rule.routingListId === next.routingListId : rule.ruleTag === next.ruleTag;
  const existing = rules.findIndex(same);
  if (existing >= 0) {
    return rules.flatMap((rule, index) =>
      !same(rule) ? [rule] : index === existing ? [{ ...rule, ...next }] : [],
    );
  }
  const beforeDefault = rules.findIndex(isCatchAll);
  const position = beforeDefault < 0 ? rules.length : beforeDefault;
  return [...rules.slice(0, position), next, ...rules.slice(position)];
}
