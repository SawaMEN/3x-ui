// A small, deterministic rule compiler shared by Xray and sing-box/Hiddify.
// The quick editor only manages rules it created. Everything else is kept
// byte-for-byte, in its original order, including service, DNS and balancer rules.
export type RoutingEngine = 'xray' | 'sing-box';
export type QuickMatch = 'inbound' | 'domain' | 'ip' | 'port';
export type QuickDomainMode = 'suffix' | 'exact';
export type RouteRecord = Record<string, unknown>;

export interface QuickRouteDraft {
  match: QuickMatch;
  inboundTags: string[];
  values: string;
  domainMode: QuickDomainMode;
  target: string;
}

export interface CompiledQuickRoute {
  id: string;
  label: string;
  rule: RouteRecord;
  criteria: string[];
}

export const QUICK_PREFIX = 'panel-quick:';
const ROUTE_MATCH_FIELDS = [
  'domain', 'domain_suffix', 'domain_keyword', 'domain_regex', 'ip', 'ip_cidr',
  'port', 'port_range', 'inbound', 'inboundTag', 'sourceIP', 'source_ip_cidr',
  'sourcePort', 'source_port', 'user', 'auth_user', 'protocol', 'network',
  'routingListId', 'routingListCategory', 'rule_set', 'attrs', 'process',
  'process_name', 'process_path', 'localIP', 'localPort', 'localOS', 'vlessRoute',
  'client', 'ip_version', 'invert', 'rules', 'mode', 'action',
  'source_port_range', 'ip_is_private', 'source_ip_is_private', 'source_mac_address',
  'process_path_regex', 'package_name', 'user_id', 'clash_mode',
  'wifi_ssid', 'wifi_bssid', 'interface_address', 'default_interface_address',
  'network_type', 'network_is_expensive', 'network_is_constrained', 'preferred_by',
  'local_ip', 'source_ip_cidr', 'http_client',
] as const;

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort();
}

function splitValues(values: string): string[] {
  return uniqueSorted(values.split(/[,;\n]+/));
}

function validDomain(value: string): boolean {
  if (value.length > 253 || value.includes('..')) return false;
  return value.split('.').every((part) =>
    part.length > 0 && part.length <= 63 && /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/i.test(part));
}

function validIP(value: string): boolean {
  const [host, cidr, extra] = value.split('/');
  if (!host || extra !== undefined) return false;
  if (host.includes(':')) {
    // URL's IPv6 host parser rejects malformed compressed IPv6 literals.
    // It is available in the browser and Node test runtime.
    try {
      new URL('http://[' + host + ']/');
    } catch {
      return false;
    }
    return cidr === undefined || /^\d+$/.test(cidr) && +cidr <= 128;
  }
  const parts = host.split('.');
  return parts.length === 4 &&
    parts.every((part) => /^\d{1,3}$/.test(part) && +part <= 255) &&
    (cidr === undefined || /^\d+$/.test(cidr) && +cidr <= 32);
}

function validPort(value: string): boolean {
  const parts = value.split('-');
  if (parts.length > 2 || parts.some((part) => !/^\d{1,5}$/.test(part))) return false;
  const start = Number(parts[0]);
  const end = Number(parts[1] ?? parts[0]);
  return start >= 1 && end >= start && end <= 65535;
}

function fingerprint(value: string): string {
  let hash = 2166136261;
  for (let i = 0; i < value.length; i++) {
    hash ^= value.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

export function isQuickRule(rule: RouteRecord, engine: RoutingEngine): boolean {
  const marker = engine === 'xray' ? rule.ruleTag : rule.comment;
  return typeof marker === 'string' && marker.startsWith(QUICK_PREFIX);
}

export function compileQuickRoute(
  draft: QuickRouteDraft,
  engine: RoutingEngine,
  outbounds: string[],
  inbounds: string[],
): CompiledQuickRoute {
  const inboundTags = uniqueSorted(draft.inboundTags);
  if (inboundTags.some((tag) => !inbounds.includes(tag)))
    throw new Error('Выбранное входящее подключение больше не существует.');
  if (draft.match === 'inbound' && inboundTags.length === 0)
    throw new Error('Выберите хотя бы одно входящее подключение.');

  const target = draft.target.trim();
  if (!target || (target !== '@reject' && !outbounds.includes(target)))
    throw new Error('Выберите существующее исходящее подключение.');
  if (target === '@reject' && engine === 'xray')
    throw new Error('Для блокировки в Xray выберите исходящее «blocked».');

  const criteria: string[] = draft.match === 'inbound' ? [] : splitValues(draft.values);
  if (draft.match !== 'inbound' && criteria.length === 0)
    throw new Error('Укажите хотя бы одно условие маршрутизации.');

  if (draft.match === 'domain') {
    for (const value of criteria) {
      if (!validDomain(value.replace(/^\*\./, '')))
        throw new Error('Некорректный домен: ' + value + '. Для IDN используйте Punycode.');
    }
  }
  if (draft.match === 'ip') {
    for (const value of criteria) {
      if (!validIP(value)) throw new Error('Некорректный IP или CIDR: ' + value);
    }
  }
  if (draft.match === 'port') {
    for (const value of criteria) {
      if (!validPort(value)) throw new Error('Некорректный порт или диапазон: ' + value);
    }
  }

  const normalized = draft.match === 'domain'
    ? uniqueSorted(criteria.map((entry) => entry.replace(/^\*\./, '').toLowerCase()))
    : criteria;
  const identifier = JSON.stringify([draft.match, draft.domainMode, inboundTags, normalized]);
  const id = QUICK_PREFIX + draft.match + ':' + fingerprint(identifier);
  const label = draft.match === 'inbound' ? 'Входящие' :
    draft.match === 'domain' ? 'Домены' : draft.match === 'ip' ? 'IP / CIDR' : 'Порты';

  const rule: RouteRecord = engine === 'xray'
    ? { type: 'field', enabled: true, ruleTag: id, comment: label }
    : { enabled: true, action: target === '@reject' ? 'reject' : 'route', comment: id };
  if (inboundTags.length) {
    if (engine === 'xray') rule.inboundTag = inboundTags;
    else rule.inbound = inboundTags;
  }
  if (draft.match === 'domain') {
    if (engine === 'xray')
      rule.domain = normalized.map((value) => (draft.domainMode === 'exact' ? 'full:' : 'domain:') + value);
    else rule[draft.domainMode === 'exact' ? 'domain' : 'domain_suffix'] = normalized;
  } else if (draft.match === 'ip') {
    rule[engine === 'xray' ? 'ip' : 'ip_cidr'] = normalized;
  } else if (draft.match === 'port') {
    if (engine === 'xray') rule.port = normalized.join(',');
    else {
      const singles = normalized.filter((value) => !value.includes('-')).map(Number);
      const ranges = normalized.filter((value) => value.includes('-')).map((value) => value.replace('-', ':'));
      if (singles.length) rule.port = singles;
      if (ranges.length) rule.port_range = ranges;
    }
  }
  if (engine === 'xray') rule.outboundTag = target;
  else if (target !== '@reject') rule.outbound = target;
  return { id, label, rule, criteria: normalized };
}

function isUnconditional(rule: RouteRecord, engine: RoutingEngine): boolean {
  if (rule.enabled === false || rule.type === 'logical') return false;
  if (engine === 'xray' && rule.type && rule.type !== 'field') return false;
  const fields = ROUTE_MATCH_FIELDS.filter((key) =>
    key !== 'action' && key !== 'mode');
  return !fields.some((key) => {
    const value = rule[key];
    if (Array.isArray(value)) return value.length > 0;
    return value !== undefined && value !== null && value !== '' && value !== false;
  });
}

export function putQuickRoute(
  rules: RouteRecord[],
  next: CompiledQuickRoute,
  engine: RoutingEngine,
): { rules: RouteRecord[]; replaced: boolean; shadowed: boolean } {
  const marker = (rule: RouteRecord) =>
    engine === 'xray' ? rule.ruleTag : rule.comment;
  const existing = rules.findIndex((rule) => marker(rule) === next.id);
  const updated = [...rules];
  if (existing !== -1) {
    updated[existing] = { ...updated[existing], ...next.rule };
    // A native reject has no outbound. Do not retain an old destination when
    // converting a previously-routed managed rule into a reject.
    if (engine === 'sing-box' && next.rule.action === 'reject') {
      delete updated[existing].outbound;
    }
    // An existing managed rule keeps its current priority and extra fields.
    return { rules: updated, replaced: true, shadowed: false };
  }
  // Never move API, DNS, existing overrides, user rules or auto-generated
  // loopbacks. Insert immediately before a terminal catch-all, when present.
  const catchAll = updated.findIndex((rule) => isUnconditional(rule, engine));
  const position = catchAll < 0 ? updated.length : catchAll;
  updated.splice(position, 0, next.rule);
  return {
    rules: updated,
    replaced: false,
    shadowed: position > 0 && updated.slice(0, position).some((rule) =>
      rule.enabled !== false && isUnconditional(rule, engine)),
  };
}

export function removeQuickRoute(rules: RouteRecord[], marker: string, engine: RoutingEngine): RouteRecord[] {
  if (!marker.startsWith(QUICK_PREFIX)) return rules;
  return rules.filter((rule) => (engine === 'xray' ? rule.ruleTag : rule.comment) !== marker);
}

export function setQuickRouteEnabled(rules: RouteRecord[], marker: string, enabled: boolean, engine: RoutingEngine): RouteRecord[] {
  if (!marker.startsWith(QUICK_PREFIX)) return rules;
  return rules.map((rule) => (engine === 'xray' ? rule.ruleTag : rule.comment) === marker
    ? { ...rule, enabled } : rule);
}

// Change the destination of an existing managed rule without rebuilding its
// conditions, moving its priority, or modifying unrelated configuration.
export function setQuickRouteTarget(
  rules: RouteRecord[], marker: string, target: string,
  engine: RoutingEngine, outbounds: string[],
): RouteRecord[] {
  if (!marker.startsWith(QUICK_PREFIX) || !target ||
      (target !== '@reject' && !outbounds.includes(target)) ||
      (engine === 'xray' && target === '@reject')) return rules;
  return rules.map((rule) => {
    const key = engine === 'xray' ? rule.ruleTag : rule.comment;
    if (key !== marker) return rule;
    if (engine === 'xray') return { ...rule, outboundTag: target };
    const next = { ...rule };
    if (target === '@reject') {
      next.action = 'reject';
      delete next.outbound;
    } else {
      next.action = 'route';
      next.outbound = target;
    }
    return next;
  });
}

export interface RoutingWarning { code: 'missing-target' | 'catch-all' | 'duplicate' | 'unconditional'; detail: string; }

export function diagnoseRouting(rules: RouteRecord[], engine: RoutingEngine, outbounds: string[]): RoutingWarning[] {
  const issues: RoutingWarning[] = [];
  const seen = new Set<string>();
  let unconditional = -1;
  for (let index = 0; index < rules.length; index++) {
    const rule = rules[index];
    if (rule.enabled === false) continue;
    const target = engine === 'xray' ? rule.outboundTag : rule.outbound;
    if (typeof target === 'string' && target !== '' && target !== 'api' && !outbounds.includes(target)) {
      // Subscription and dynamic outbounds are supplied by the panel as well.
      issues.push({ code: 'missing-target', detail: `#${index + 1}: не найдено исходящее «${target}»` });
    }
    const mark = engine === 'xray' ? rule.ruleTag : rule.comment;
    if (typeof mark === 'string' && mark.startsWith(QUICK_PREFIX)) {
      if (seen.has(mark)) issues.push({ code: 'duplicate', detail: `#${index + 1}: дублирующееся быстрое правило` });
      seen.add(mark);
    }
    if (unconditional >= 0 && isQuickRule(rule, engine)) {
      issues.push({ code: 'catch-all', detail: `#${index + 1}: правило перекрыто правилом без условий (#${unconditional + 1})` });
    }
    if (unconditional < 0 && isUnconditional(rule, engine)) unconditional = index;
  }
  if (unconditional >= 0 && unconditional < rules.length - 1)
    issues.push({ code: 'unconditional', detail: `#${unconditional + 1}: правило без условий может перекрывать последующие правила` });
  return issues;
}
