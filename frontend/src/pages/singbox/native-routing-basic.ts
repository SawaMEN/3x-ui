import { object, records, type ConfigObject } from './simple-settings';

export const basicRouteFields = [
  {
    id: 'blocked-domains',
    title: 'Заблокированные домены',
    field: 'domain_suffix',
    action: 'reject',
  },
  { id: 'blocked-ips', title: 'Заблокированные IP / CIDR', field: 'ip_cidr', action: 'reject' },
  { id: 'direct-domains', title: 'Домены напрямую', field: 'domain_suffix', action: 'route' },
  { id: 'direct-ips', title: 'IP / CIDR напрямую', field: 'ip_cidr', action: 'route' },
] as const;

export function basicRule(config: ConfigObject, id: string): ConfigObject {
  return (
    records(object(config.route).rules).find((rule) => rule.comment === `panel-basic:${id}`) || {}
  );
}

export function setBasicRule(config: ConfigObject, id: string, fields: ConfigObject): ConfigObject {
  const route = object(config.route);
  const marker = `panel-basic:${id}`;
  const rules = records(route.rules);
  const index = rules.findIndex((rule) => rule.comment === marker);
  const active = Object.entries(fields).some(
    ([key, value]) =>
      ['domain_suffix', 'ip_cidr', 'protocol'].includes(key) &&
      Array.isArray(value) &&
      value.length > 0,
  );
  const next = rules.filter((rule) => rule.comment !== marker);
  if (active) next.splice(index < 0 ? 0 : index, 0, { ...fields, comment: marker });
  return { ...config, route: { ...route, rules: next } };
}

export function basicRuleTitle(comment: string): string {
  if (comment === 'panel-basic:torrent') return 'Блокировать BitTorrent';
  if (comment.startsWith('panel-quick:')) {
    const kind = comment.split(':')[1];
    const labels: Record<string, string> = {
      inbound: 'Быстрый маршрут · Входящие',
      domain: 'Быстрый маршрут · Домены',
      ip: 'Быстрый маршрут · IP / CIDR',
      port: 'Быстрый маршрут · Порты',
    };
    return labels[kind] || 'Быстрый маршрут';
  }
  return basicRouteFields.find((field) => comment === `panel-basic:${field.id}`)?.title || comment;
}
