type Rule = Record<string, unknown>;

export function routePorts(rule: Rule, source = false): string {
  const prefix = source ? 'source_port' : 'port';
  return [rule[prefix], rule[`${prefix}_range`]]
    .flatMap((value) => (Array.isArray(value) ? value : value == null ? [] : [value]))
    .join(', ');
}

export function prepareRouteRule(rule: Rule): Rule {
  const next = { ...rule };
  for (const prefix of ['port', 'source_port']) {
    const ports: number[] = [];
    const ranges: string[] = [];
    for (const token of routePorts(rule, prefix === 'source_port')
      .split(/[\s,]+/)
      .filter(Boolean)) {
      if (/^\d+$/.test(token) && Number(token) >= 1 && Number(token) <= 65535) {
        ports.push(Number(token));
      } else {
        const range = token.match(/^(\d*)[:-](\d*)$/);
        const start = range?.[1] ? Number(range[1]) : 1;
        const end = range?.[2] ? Number(range[2]) : 65535;
        if (!range || !(range[1] || range[2]) || start < 1 || end > 65535 || start > end)
          throw new Error(`Некорректный порт или диапазон: ${token}`);
        ranges.push(`${range[1]}:${range[2]}`);
      }
    }
    delete next[prefix];
    delete next[`${prefix}_range`];
    if (ports.length) next[prefix] = [...new Set(ports)];
    if (ranges.length) next[`${prefix}_range`] = [...new Set(ranges)];
  }
  const action = typeof next.action === 'string' ? next.action : 'route';
  if (action === 'route' && !next.outbound) throw new Error('Выберите исходящее подключение.');
  if (action !== 'route') delete next.outbound;
  next.action = action;
  return next;
}

export function routeRuleSummary(rule: Rule): string {
  const labels: Record<string, string> = {
    domain: 'Домен',
    domain_suffix: 'Суффикс',
    domain_keyword: 'Ключевое слово',
    domain_regex: 'Регулярное выражение',
    ip_cidr: 'IP',
    source_ip_cidr: 'IP источника',
    protocol: 'Протокол',
    network: 'Сеть',
    inbound: 'Входящее',
    rule_set: 'Набор правил',
  };
  const pieces = Object.entries(labels).flatMap(([key, label]) => {
    const value = rule[key];
    const text = Array.isArray(value) ? value.join(', ') : typeof value === 'string' ? value : '';
    return text ? [`${label}: ${text}`] : [];
  });
  if (routePorts(rule)) pieces.push(`Порты: ${routePorts(rule)}`);
  if (routePorts(rule, true)) pieces.push(`Порты источника: ${routePorts(rule, true)}`);
  if (rule.ip_is_private) pieces.push('Частные IP');
  if (rule.source_ip_is_private) pieces.push('Частные IP источника');
  if (rule.type === 'logical') pieces.push(`Логическое правило: ${rule.mode || 'and'}`);
  const summary = pieces.join(' · ') || 'Любой трафик';
  return rule.invert ? `НЕ (${summary})` : summary;
}
