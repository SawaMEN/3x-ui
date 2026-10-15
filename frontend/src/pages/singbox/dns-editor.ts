import { z } from 'zod';
import { object, records, type ConfigObject } from './simple-settings';
import { referencePaths, replaceReferences } from './config-references';

export const DnsServerFormSchema = z.object({
  tag: z.string().trim().min(1),
  type: z.string().min(1),
  server: z.string(),
  server_port: z.number().int().min(1).max(65535).optional(),
  path: z.string(),
  server_name: z.string(),
  detour: z.string(),
});
export type DnsServerForm = z.infer<typeof DnsServerFormSchema>;
export function dnsServerForm(value: ConfigObject): DnsServerForm {
  return {
    tag: String(value.tag || ''),
    type: String(value.type || 'local'),
    server: String(value.server || ''),
    server_port: typeof value.server_port === 'number' ? value.server_port : undefined,
    path: String(value.path || ''),
    server_name: String(object(value.tls).server_name || ''),
    detour: String(value.detour || ''),
  };
}
export function buildDnsServer(original: ConfigObject, form: DnsServerForm): ConfigObject {
  const parsed = DnsServerFormSchema.parse(form);
  const next: ConfigObject = original.type === parsed.type ? { ...original } : {};
  next.tag = parsed.tag;
  next.type = parsed.type;
  const network = ['udp', 'tcp', 'tls', 'quic', 'https', 'h3'].includes(parsed.type);
  if (network) {
    let server = parsed.server.trim();
    const urlInput = /^https?:\/\//i.test(server);
    if (urlInput) {
      const url = new URL(server);
      if (url.protocol !== 'https:' || url.username || url.password)
        throw new Error('Для DNS через HTTPS нужен адрес https:// без логина и пароля.');
      next.type = 'https';
      server = url.hostname.replace(/^\[|\]$/g, '');
      next.server_port = url.port ? Number(url.port) : 443;
      next.path = url.pathname === '/' ? '/dns-query' : url.pathname + url.search;
    }
    if (!server || /[\s/]/.test(server)) throw new Error('Укажите IP-адрес или имя DNS-сервера.');
    next.server = server;
    if (parsed.server_port && !urlInput) next.server_port = parsed.server_port;
    if (!next.server_port)
      next.server_port = ['https', 'h3'].includes(String(next.type))
        ? 443
        : ['tls', 'quic'].includes(String(next.type))
          ? 853
          : 53;
    if (['tls', 'quic', 'https', 'h3'].includes(String(next.type))) {
      const tls: ConfigObject = { ...object(next.tls), enabled: true };
      if (parsed.server_name.trim()) tls.server_name = parsed.server_name.trim();
      else delete tls.server_name;
      next.tls = tls;
    }
    if (['https', 'h3'].includes(String(next.type)) && !urlInput)
      next.path = parsed.path.trim() || next.path || '/dns-query';
  }
  if (parsed.detour) next.detour = parsed.detour;
  else delete next.detour;
  return next;
}
export function putDnsServer(
  config: ConfigObject,
  index: number | null,
  next: ConfigObject,
): ConfigObject {
  const servers = records(object(config.dns).servers);
  if (servers.some((item, i) => i !== index && item.tag === next.tag))
    throw new Error('Название DNS-сервера уже используется.');
  const previous = index == null ? undefined : servers[index];
  const renamed =
    previous?.tag && previous.tag !== next.tag
      ? replaceReferences(config, 'dns', String(previous.tag), String(next.tag))
      : config;
  const dns = object(renamed.dns),
    updated = [...records(dns.servers)];
  if (index == null) updated.push(next);
  else updated[index] = next;
  return { ...renamed, dns: { ...dns, servers: updated } };
}
export function removeDnsServer(config: ConfigObject, index: number): ConfigObject {
  const dns = object(config.dns),
    servers = records(dns.servers),
    tag = String(servers[index]?.tag || '');
  const paths = referencePaths(config, 'dns', tag).filter(
    (path) => !path.startsWith(`dns.servers[${index}].`),
  );
  if (paths.length)
    throw new Error(`DNS-сервер используется: ${paths.join(', ')}. Сначала выберите другой DNS.`);
  return { ...config, dns: { ...dns, servers: servers.filter((_, i) => i !== index) } };
}
