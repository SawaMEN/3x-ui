import { describe, expect, it } from 'vitest';

import { parseOutboundLink } from '@/lib/xray/outbound-link-parser';
import { Base64 } from '@/utils';

describe('outbound import endpoint validation', () => {
  it.each([
    'vless://UserABC@server.example:443#Name',
    'trojan://PasswordABC@server.example:443#Name',
    'snell://KeyABC@server.example:443?version=4#Name',
  ])('preserves credentials when normalizing scheme: %s', (raw) => {
    const separator = raw.indexOf(':');
    const upper = raw.slice(0, separator).toUpperCase() + raw.slice(separator);
    expect(parseOutboundLink(upper)).toEqual(parseOutboundLink(raw));
  });

  it.each([
    'vless://@server.example',
    'trojan://@server.example',
    'wireguard://@server.example?publickey=peer',
    'wireguard://key@server.example',
    'vmess://' + Base64.encode(JSON.stringify({ port: 443, id: 'user' })),
    'vmess://' + Base64.encode(JSON.stringify({ port: 443, add: 'server.example' })),
  ])('rejects missing required endpoint credentials: %s', (raw) => {
    expect(parseOutboundLink(raw)).toBeNull();
  });
});
