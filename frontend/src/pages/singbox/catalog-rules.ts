import type { RuleObject } from '@/schemas/routing';
import type { CatalogSelection } from '@/pages/xray/routing/catalog-rules';

type Rule = Record<string, unknown>;

export function nativeCatalogRule(selection: CatalogSelection, outbound: string): Rule {
  return {
    action: 'route',
    outbound,
    comment: selection.name,
    enabled: true,
    ...(selection.listId
      ? { routingListId: selection.listId }
      : { routingListCategory: selection.token?.replace(/^geosite:/, '') }),
  };
}

export function catalogAssignedRules(rules: Rule[]): RuleObject[] {
  return rules.map((r) => ({
    type: 'field',
    ...(typeof r.routingListId === 'number' ? { routingListId: r.routingListId } : {}),
    ...(typeof r.routingListCategory === 'string'
      ? {
          ruleTag: `catalog-geosite:${r.routingListCategory}`,
          domain: [`geosite:${r.routingListCategory}`],
        }
      : {}),
    outboundTag: typeof r.outbound === 'string' ? r.outbound : undefined,
    enabled: r.enabled !== false,
  }));
}

const MATCHERS = [
  'rules',
  'invert',
  'inbound',
  'ip_version',
  'network',
  'auth_user',
  'protocol',
  'client',
  'domain',
  'domain_suffix',
  'domain_keyword',
  'domain_regex',
  'source_ip_cidr',
  'source_ip_is_private',
  'ip_cidr',
  'ip_is_private',
  'source_port',
  'source_port_range',
  'port',
  'port_range',
  'process_name',
  'process_path',
  'process_path_regex',
  'package_name',
  'user',
  'user_id',
  'mode',
  'clash_mode',
  'wifi_ssid',
  'wifi_bssid',
  'rule_set',
  'source_mac_address',
  'interface_address',
  'default_interface_address',
  'network_type',
  'network_is_expensive',
  'network_is_constrained',
  'preferred_by',
  'routingListId',
  'routingListCategory',
];

export function putNativeCatalogRule(rules: Rule[], next: Rule): Rule[] {
  const same = (r: Rule) =>
    next.routingListId
      ? r.routingListId === next.routingListId
      : r.routingListCategory === next.routingListCategory;
  const existing = rules.findIndex(same);
  if (existing >= 0)
    return rules.flatMap((r, index) =>
      !same(r) ? [r] : index === existing ? [{ ...r, ...next }] : [],
    );
  const terminalActions = new Set(['route', 'reject']);
  const index = rules.findIndex(
    (r) =>
      r.enabled !== false &&
      (r.action === undefined || terminalActions.has(String(r.action))) &&
      !MATCHERS.some((key) => {
        const value = r[key];
        return Array.isArray(value)
          ? value.length > 0
          : value !== undefined && value !== '' && value !== false;
      }),
  );
  const position = index < 0 ? rules.length : index;
  return [...rules.slice(0, position), next, ...rules.slice(position)];
}
