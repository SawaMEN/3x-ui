import { Wireguard } from '@/utils';
import { object, type ConfigObject } from './simple-settings';

const bytes = (length: number) => globalThis.crypto.getRandomValues(new Uint8Array(length));
const hex = (value: Uint8Array) =>
  Array.from(value, (byte) => byte.toString(16).padStart(2, '0')).join('');
const base64 = (value: Uint8Array) => btoa(String.fromCharCode(...value));
const password = () => hex(bytes(24));
function uuid(): string {
  const value = bytes(16);
  value[6] = (value[6] & 15) | 64;
  value[8] = (value[8] & 63) | 128;
  const raw = hex(value);
  return `${raw.slice(0, 8)}-${raw.slice(8, 12)}-${raw.slice(12, 16)}-${raw.slice(16, 20)}-${raw.slice(20)}`;
}
function privateKey(): string {
  const value = bytes(32);
  value[0] &= 248;
  value[31] = (value[31] & 127) | 64;
  return base64(value);
}
function sudokuKey(): string {
  // Canonical nonzero Edwards25519 scalar, little-endian, matching the
  // native Sudoku key parser. Rejection sampling avoids modular bias.
  const order = 2n ** 252n + 27742317777372353535851937790883648493n;
  for (;;) {
    const value = bytes(32);
    value[31] &= 31;
    const scalar = BigInt(`0x${hex(Uint8Array.from(value).reverse())}`);
    if (scalar > 0n && scalar < order) return hex(value);
  }
}

// Generate only missing fields. Existing server credentials and imported keys
// must not rotate when automatic tuning is applied again.
export function generateOutboundCredentials(
  value: ConfigObject,
  optionalAuth = false,
): ConfigObject {
  const next = structuredClone(value),
    type = String(next.type || '');
  const fill = (key: string, generate: () => unknown) => {
    if (next[key] === undefined || next[key] === null || next[key] === '') next[key] = generate();
  };
  if (['vless', 'vmess', 'tuic', 'vless-reverse', 'tunnel_client', 'tunnel_server'].includes(type))
    fill('uuid', uuid);
  if (
    ['naive', 'mieru', 'trusttunnel', 'fptn'].includes(type) ||
    (optionalAuth && ['http', 'socks', 'masque-client'].includes(type))
  )
    fill('username', () => `vpn_${hex(bytes(6))}`);
  if (
    ['naive', 'mieru', 'trusttunnel', 'fptn', 'trojan', 'anytls', 'hysteria2', 'tuic'].includes(
      type,
    ) ||
    (optionalAuth && ['http', 'socks', 'masque-client'].includes(type))
  )
    fill('password', password);
  if (type === 'hysteria2' && object(next.obfs).type && !object(next.obfs).password)
    next.obfs = { ...object(next.obfs), password: password() };
  if (type === 'hysteria') fill('auth_str', password);
  if (type === 'shadowtls' && Number(next.version ?? 3) >= 2) fill('password', password);
  if (type === 'shadowsocks' && next.method !== 'none')
    fill('password', () =>
      String(next.method).startsWith('2022-')
        ? base64(bytes(next.method === '2022-blake3-aes-128-gcm' ? 16 : 32))
        : password(),
    );
  if (type === 'snell') fill('psk', () => base64(bytes(32)));
  if (type === 'ssh' && !next.private_key && !next.private_key_path) {
    fill('user', () => `vpn_${hex(bytes(6))}`);
    fill('password', password);
  }
  if (['wireguard', 'wireguard_legacy', 'awg', 'tailcat'].includes(type))
    fill('private_key', privateKey);
  if (type === 'sudoku') fill('key', sudokuKey);
  if (type === 'openflux') fill('secret', password);
  if (type === 'pingtunnel')
    fill('key', () => {
      const value = new Uint32Array(1);
      globalThis.crypto.getRandomValues(value);
      return value[0] & 0x7fffffff;
    });
  if (type === 'gooserelay') fill('tunnel_key', () => hex(bytes(32)));
  if (type === 'tunnel_client') fill('key', () => base64(bytes(32)));
  if (type === 'tunnel_server' && (!Array.isArray(next.users) || !next.users.length))
    next.users = [{ uuid: uuid(), key: base64(bytes(32)) }];
  return next;
}

export function outboundCredentialSummary(value: ConfigObject): ConfigObject {
  const keys = [
    'username',
    'user',
    'password',
    'uuid',
    'psk',
    'key',
    'secret',
    'tunnel_key',
    'auth_str',
    'private_key',
    'users',
  ];
  const summary = Object.fromEntries(
    keys.filter((key) => value[key] !== undefined).map((key) => [key, value[key]]),
  );
  if (
    ['wireguard', 'wireguard_legacy', 'awg', 'tailcat'].includes(String(value.type)) &&
    typeof value.private_key === 'string'
  ) {
    try {
      summary.public_key = Wireguard.generateKeypair(value.private_key).publicKey;
    } catch {
      /* Imported keys are validated by the core. */
    }
  }
  return { type: value.type, ...summary };
}
