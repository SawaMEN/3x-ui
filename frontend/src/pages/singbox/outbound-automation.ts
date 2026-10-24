import { generateOutboundCredentials } from './outbound-credentials';
import registry from './native-outbound-fields.json';
import { outboundDefaults, object, type ConfigObject } from './simple-settings';
import { changeServer, requiredTlsProtocols } from './outbound-editor-model';
import { patchOption, type OptionSchema } from './protocol-option-model';

// Let the installed core choose optional tuning. Only fill required client
// defaults; existing credentials, peer keys, custom routing and transport remain intact.
export function autoConfigureOutbound(value: ConfigObject): ConfigObject {
  const type = String(value.type || '');
  const schema = (registry.protocols as unknown as Record<string, OptionSchema>)[type];
  if (!schema) return structuredClone(value);
  let next = structuredClone(value);
  const defaults = outboundDefaults(type, String(value.tag || `${type}-out`));
  if (type === 'mieru' && (!Array.isArray(next.portBindings) || !next.portBindings.length)) {
    next.portBindings = [
      { protocol: 'TCP', port: typeof next.server_port === 'number' ? next.server_port : 443 },
    ];
  }
  for (const [key, fallback] of Object.entries(defaults)) {
    if (key === 'type' || key === 'tag' || key === 'tls' || !(key in schema)) continue;
    if (key === 'server_port' && Array.isArray(next.server_ports) && next.server_ports.length)
      continue;
    if (next[key] === undefined || next[key] === null || next[key] === '') next[key] = fallback;
  }
  if (type === 'snell' && ![4, 6].includes(Number(next.version)))
    next = patchOption(next, 'version', 6, schema);
  if (requiredTlsProtocols.includes(type) || type === 'trojan') {
    next.tls = { ...object(next.tls), enabled: true };
  }
  if (typeof next.server === 'string') next = changeServer(next, next.server.trim());
  return generateOutboundCredentials(next);
}
