import { object, records, type ConfigObject } from './simple-settings';

const PREFIX = 'panel-inbound:';

function isUnconditionalRoute(rule: ConfigObject): boolean {
  if (rule.enabled === false || !rule.outbound || (rule.action && rule.action !== 'route')) {
    return false;
  }
  return Object.entries(rule).every(
    ([key, value]) =>
      ['action', 'enabled', 'outbound', 'comment'].includes(key) ||
      value == null ||
      value === '' ||
      (Array.isArray(value) && value.length === 0),
  );
}

export function nativeInboundRouteTarget(config: ConfigObject, tag: string): string {
  const rule = records(object(config.route).rules).find(
    (row) =>
      row.comment === PREFIX + tag &&
      row.enabled !== false &&
      Array.isArray(row.inbound) &&
      row.inbound.length === 1 &&
      row.inbound[0] === tag,
  );
  return typeof rule?.outbound === 'string' ? rule.outbound : '';
}

export function setNativeInboundRouteTarget(
  config: ConfigObject,
  tag: string,
  outbound: string,
): ConfigObject {
  if (!tag.trim()) return config;
  const route = object(config.route);
  const marker = PREFIX + tag;
  const rules = records(route.rules);
  const previous = rules.findIndex((rule) => rule.comment === marker);
  const next = rules.filter((rule) => rule.comment !== marker);
  if (outbound) {
    const fallback = next.findIndex(isUnconditionalRoute);
    // An existing binding may have been moved below a catch-all by another
    // editor. Reposition it before that fallback on every update.
    const index =
      fallback >= 0
        ? previous >= 0
          ? Math.min(previous, fallback)
          : fallback
        : previous >= 0
          ? Math.min(previous, next.length)
          : next.length;
    next.splice(index, 0, { action: 'route', comment: marker, inbound: [tag], outbound });
  }
  return { ...config, route: { ...route, rules: next } };
}
