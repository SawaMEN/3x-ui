import { formValuesToWirePayload, rawOutboundToFormValues } from './outbound-form-adapter';

type Raw = Record<string, unknown>;

function isRecord(value: unknown): value is Raw {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function firstMissingPath(source: unknown, target: unknown, path: string): string | null {
  if (source === target) return null;
  if (Array.isArray(source)) {
    if (!Array.isArray(target) || source.length > target.length) return path;
    for (let i = 0; i < source.length; i++) {
      const lost = firstMissingPath(source[i], target[i], `${path}[${i}]`);
      if (lost) return lost;
    }
    return null;
  }
  if (isRecord(source)) {
    if (!isRecord(target)) return path;
    for (const [key, value] of Object.entries(source)) {
      // The serializer intentionally omits empty optional fields.
      if (value === '' && target[key] === undefined) continue;
      const lost = firstMissingPath(value, target[key], path ? `${path}.${key}` : key);
      if (lost) return lost;
    }
    return null;
  }
  // Numeric strings are normalized into numbers by the form's port fields.
  if (typeof source === 'string' && typeof target === 'number' && source === String(target))
    return null;
  return path;
}

/**
 * VLESS supports two legacy input shapes. The visual form serializes the
 * first vnext server/user as flat settings; only allow that conversion if
 * there are no additional users, servers or unrepresented options.
 */
function normalizedVlessSource(raw: Raw): Raw | string {
  if (String(raw.protocol).toLowerCase() !== 'vless' || !isRecord(raw.settings)) return raw;
  const settings = raw.settings;
  if (!Array.isArray(settings.vnext)) return raw;
  if (settings.vnext.length !== 1 || !isRecord(settings.vnext[0])) return 'settings.vnext';
  const server = settings.vnext[0];
  const users = server.users;
  if (!Array.isArray(users) || users.length !== 1 || !isRecord(users[0]))
    return 'settings.vnext[0].users';
  const user = users[0];
  for (const key of Object.keys(server)) {
    if (!['address', 'port', 'users'].includes(key)) return `settings.vnext[0].${key}`;
  }
  for (const key of Object.keys(user)) {
    if (!['id', 'flow', 'encryption'].includes(key)) return `settings.vnext[0].users[0].${key}`;
  }
  const { vnext: _vnext, ...rest } = settings;
  return {
    ...raw,
    settings: {
      ...rest,
      address: server.address,
      port: server.port,
      id: user.id,
      ...(user.flow !== undefined ? { flow: user.flow } : {}),
      ...(user.encryption !== undefined ? { encryption: user.encryption } : {}),
    },
  };
}

/** Return the first field a Basic-form roundtrip would discard/change. */
export function outboundBasicConversionLoss(raw: Raw): string | null {
  const normalized = normalizedVlessSource(raw);
  if (typeof normalized === 'string') return normalized;
  const projected = formValuesToWirePayload(rawOutboundToFormValues(raw));
  return firstMissingPath(
    { ...normalized, protocol: String(raw.protocol ?? '').toLowerCase() },
    projected,
    '',
  );
}
