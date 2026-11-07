// Health checks for the saved sing-box editor configuration. The runtime
// additionally injects managed node-route outbounds for referenced rules.
type JsonObject = Record<string, unknown>;
type ConfigMap = JsonObject;

function asObject(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as JsonObject) : {};
}

function asObjectArray(value: unknown): JsonObject[] {
  return Array.isArray(value) ? value.map(asObject) : [];
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : [];
}

// Managed node-route outbounds are generated only for active routing rules
// when sing-box builds its runtime configuration. They are intentionally absent
// from the editable outbound list (which never exposes their credentials).
function isRuntimeNodeRouteTag(tag: string): boolean {
  if (!/^node-route-[1-9][0-9]*$/.test(tag)) return false;
  return Number.isSafeInteger(Number(tag.slice('node-route-'.length)));
}

export function singBoxHealthIssues(config: ConfigMap, subscriptionTags: string[] = []) {
  const issues: string[] = [];
  const outbounds = asObjectArray(config.outbounds);
  const tags = [...outbounds, ...asObjectArray(config.endpoints)]
    .map((item) => asString(item.tag))
    .filter(Boolean);
  const seen = new Set<string>();

  tags.forEach((tag) => {
    if (seen.has(tag)) issues.push('Дублирующийся outbound tag: ' + tag);
    seen.add(tag);
  });

  const tagSet = new Set([...tags, ...subscriptionTags]);
  outbounds.forEach((item) => {
    const type = asString(item.type);
    const name = asString(item.tag) || 'без tag';
    if (
      ['vless', 'vmess', 'trojan', 'shadowsocks', 'http', 'socks', 'hysteria2', 'tuic'].includes(
        type,
      )
    ) {
      if (!asString(item.server)) issues.push(type + ' "' + name + '": не указан сервер');
      if (
        !asNumber(item.server_port) &&
        !(type === 'hysteria2' && asStringArray(item.server_ports).length)
      )
        issues.push(type + ' "' + name + '": не указан порт');
    }
    if (['vless', 'vmess'].includes(type) && !asString(item.uuid)) {
      issues.push(type + ' "' + name + '": не указан UUID');
    }
    if (['selector', 'urltest', 'balancer'].includes(type)) {
      asStringArray(item.outbounds).forEach((target) => {
        if (!tagSet.has(target))
          issues.push(type + ' "' + name + '": отсутствует outbound "' + target + '"');
      });
    }
  });

  const route = asObject(config.route);
  if (asString(route.final) && !tagSet.has(asString(route.final))) {
    issues.push('route.final ссылается на отсутствующий outbound "' + asString(route.final) + '"');
  }

  const rules = Array.isArray(route.rules) ? route.rules : [];
  rules.forEach((raw, index) => {
    const rule = asObject(raw);
    if (rule.enabled === false) return;
    const destination = asString(rule.outbound);
    if (destination && !tagSet.has(destination) && !isRuntimeNodeRouteTag(destination)) {
      issues.push(
        'Route rule #' +
          (index + 1) +
          ' ссылается на отсутствующий outbound "' +
          destination +
          '"',
      );
    }
  });

  return Array.from(new Set(issues));
}
