import { object, records, type ConfigObject } from './simple-settings';
import { isNativeEndpoint } from './native-import';

type ReferenceKind = 'connection' | 'dns';

function references(config: ConfigObject, kind: ReferenceKind, tag: string, replacement: string) {
  const paths: string[] = [];
  const visit = (value: unknown, path: string): unknown => {
    if (Array.isArray(value)) return value.map((item, i) => visit(item, `${path}[${i}]`));
    if (!value || typeof value !== 'object') return value;
    const result: ConfigObject = {};
    for (const [key, item] of Object.entries(value)) {
      const nextPath = path ? `${path}.${key}` : key;
      if (key === 'headers') {
        result[key] = item;
        continue;
      }
      const connection =
        kind === 'connection' &&
        (['outbound', 'detour', 'download_detour', 'portal_tag', 'bridge_tag'].includes(key) ||
          nextPath === 'route.final' ||
          (key === 'default' && object(value).type === 'selector'));
      const dns =
        kind === 'dns' &&
        (nextPath === 'dns.final' ||
          (key === 'server' && /^dns\.rules\[/.test(path)) ||
          ['domain_resolver', 'default_domain_resolver'].includes(key) ||
          (key === 'server' && /(?:^|\.)(?:default_)?domain_resolver$/.test(path)));
      if ((connection || dns) && item === tag) {
        paths.push(nextPath);
        result[key] = replacement;
      } else if (kind === 'connection' && key === 'outbounds' && Array.isArray(item)) {
        result[key] = item.map((member, i) => {
          if (member === tag) {
            paths.push(`${nextPath}[${i}]`);
            return replacement;
          }
          return visit(member, `${nextPath}[${i}]`);
        });
      } else result[key] = visit(item, nextPath);
    }
    return result;
  };
  return { config: visit(config, '') as ConfigObject, paths };
}

export function referencePaths(config: ConfigObject, kind: ReferenceKind, tag: string): string[] {
  return tag ? references(config, kind, tag, tag).paths : [];
}

export function replaceReferences(
  config: ConfigObject,
  kind: ReferenceKind,
  from: string,
  to: string,
): ConfigObject {
  return references(config, kind, from, to).config;
}

export function putConnection(
  config: ConfigObject,
  index: number | null,
  next: ConfigObject,
  extra: ConfigObject[] = [],
): ConfigObject {
  const all = [...records(config.outbounds), ...records(config.endpoints)];
  const previous = index == null ? undefined : all[index];
  const tag = String(next.tag || '').trim();
  if (!tag || all.some((item, i) => i !== index && item.tag === tag))
    throw new Error('Название подключения должно быть уникальным.');
  const renamed =
    previous?.tag && previous.tag !== tag
      ? replaceReferences(config, 'connection', String(previous.tag), tag)
      : config;
  const outbounds = [...records(renamed.outbounds)],
    endpoints = [...records(renamed.endpoints)];
  const sameKind = previous && isNativeEndpoint(previous) === isNativeEndpoint(next);
  const oldList = index != null && index >= outbounds.length ? endpoints : outbounds;
  const oldIndex = index != null && index >= outbounds.length ? index - outbounds.length : index;
  if (index != null && oldIndex != null) {
    if (sameKind) oldList[oldIndex] = { ...next, tag };
    else oldList.splice(oldIndex, 1);
  }
  if (!sameKind) (isNativeEndpoint(next) ? endpoints : outbounds).push({ ...next, tag });
  for (const item of extra) (isNativeEndpoint(item) ? endpoints : outbounds).push(item);
  return { ...renamed, outbounds, endpoints };
}

export function removeConnection(config: ConfigObject, index: number): ConfigObject {
  const outbounds = records(config.outbounds),
    endpoints = records(config.endpoints);
  const item = [...outbounds, ...endpoints][index];
  if (!item) return config;
  const ownPath =
    index < outbounds.length ? `outbounds[${index}]` : `endpoints[${index - outbounds.length}]`;
  const paths = referencePaths(config, 'connection', String(item.tag)).filter(
    (path) => !path.startsWith(ownPath + '.'),
  );
  if (paths.length)
    throw new Error(
      `Подключение используется: ${paths.join(', ')}. Сначала выберите другой выход в этих настройках.`,
    );
  return index < outbounds.length
    ? { ...config, outbounds: outbounds.filter((_, i) => i !== index) }
    : { ...config, endpoints: endpoints.filter((_, i) => i !== index - outbounds.length) };
}
