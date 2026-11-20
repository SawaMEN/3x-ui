export const MAX_RANDOM_CIDR_COUNT = 100;

export function cleanCIDR(s: string): string {
  const clean = s.trim();
  const bracketed = /^\[([^\]]+)\]((?:\/\d+)?)$/.exec(clean);
  return bracketed ? bracketed[1] + bracketed[2] : clean;
}

export function parseIPv4(ipStr: string): number | null {
  const parts = ipStr.trim().split('.');
  if (parts.length !== 4) return null;
  let u32 = 0;
  for (const part of parts) {
    if (!/^(0|[1-9]\d{0,2})$/.test(part)) return null;
    const n = Number(part);
    if (n < 0 || n > 255) return null;
    u32 = ((u32 << 8) | n) >>> 0;
  }
  return u32;
}

export function formatIPv4(val: number): string {
  return `${(val >>> 24) & 255}.${(val >>> 16) & 255}.${(val >>> 8) & 255}.${val & 255}`;
}

export function parseIPv6(ipStr: string): bigint | null {
  let s = ipStr.trim().toLowerCase();
  if (s.startsWith('[') && s.endsWith(']')) {
    s = s.slice(1, -1);
  }
  if (s.includes('.')) {
    const lastColon = s.lastIndexOf(':');
    if (lastColon < 0) return null;
    const tail = s.slice(lastColon + 1);
    if (tail !== tail.trim()) return null;
    const ipv4 = parseIPv4(tail);
    if (ipv4 === null) return null;
    s = `${s.slice(0, lastColon + 1)}${(ipv4 >>> 16).toString(16)}:${(ipv4 & 0xffff).toString(16)}`;
  }
  const doubleColonCount = (s.match(/::/g) ?? []).length;
  if (doubleColonCount > 1) return null;

  let leftParts: string[] = [];
  let rightParts: string[] = [];

  if (s.includes('::')) {
    const [left, right] = s.split('::');
    leftParts = left ? left.split(':') : [];
    rightParts = right ? right.split(':') : [];
  } else {
    leftParts = s.split(':');
  }

  const totalParts = leftParts.length + rightParts.length;
  if (s.includes('::')) {
    if (totalParts > 7) return null;
  } else {
    if (totalParts !== 8) return null;
  }

  const missingZeros = 8 - totalParts;
  const allParts = [...leftParts, ...Array(missingZeros).fill('0'), ...rightParts];

  let result = 0n;
  for (const part of allParts) {
    if (!/^[0-9a-f]{1,4}$/.test(part)) return null;
    result = (result << 16n) | BigInt(parseInt(part, 16));
  }
  return result;
}

export function formatIPv6(val: bigint): string {
  if (val >> 32n === 0xffffn) {
    return `::ffff:${formatIPv4(Number(val & 0xffffffffn))}`;
  }
  const parts: number[] = [];
  for (let i = 7; i >= 0; i--) {
    parts.push(Number((val >> BigInt(i * 16)) & 0xffffn));
  }

  let bestStart = -1;
  let bestLen = 0;
  let currentStart = -1;
  let currentLen = 0;

  for (let i = 0; i < 8; i++) {
    if (parts[i] === 0) {
      if (currentStart === -1) {
        currentStart = i;
        currentLen = 1;
      } else {
        currentLen++;
      }
    } else {
      if (currentLen > bestLen) {
        bestLen = currentLen;
        bestStart = currentStart;
      }
      currentStart = -1;
      currentLen = 0;
    }
  }
  if (currentLen > bestLen) {
    bestLen = currentLen;
    bestStart = currentStart;
  }

  if (bestLen >= 2) {
    const left = parts
      .slice(0, bestStart)
      .map((p) => p.toString(16))
      .join(':');
    const right = parts
      .slice(bestStart + bestLen)
      .map((p) => p.toString(16))
      .join(':');
    return `${left}::${right}`;
  }

  return parts.map((p) => p.toString(16)).join(':');
}

function parseCIDR(input: string): { ip: bigint; prefix: number; bits: number } | null {
  const clean = cleanCIDR(input);
  // Only normalized, bracket-free IP literals are valid at this point.
  if (clean.includes('[') || clean.includes(']')) return null;
  const slash = clean.indexOf('/');
  if (slash < 0) return null;
  const address = clean.slice(0, slash);
  const prefixText = clean.slice(slash + 1);
  if (!/^\d+$/.test(prefixText) || address !== address.trim()) return null;
  const prefix = Number(prefixText);
  const ipv4 = parseIPv4(address);
  if (ipv4 !== null) {
    return prefix <= 32 ? { ip: BigInt(ipv4), prefix, bits: 32 } : null;
  }
  const ipv6 = parseIPv6(address);
  return ipv6 !== null && prefix <= 128 ? { ip: ipv6, prefix, bits: 128 } : null;
}

export function isCIDR(s: string): boolean {
  return parseCIDR(s) !== null;
}

export function randomIPsFromCIDR(cidrStr: string, count: number): string[] {
  const network = parseCIDR(cidrStr);
  if (!network) return [];
  count = Math.min(Math.max(Math.trunc(count) || 1, 1), MAX_RANDOM_CIDR_COUNT);

  const hostBits = BigInt(network.bits - network.prefix);
  const base = (network.ip >> hostBits) << hostBits;
  const isIPv4 = network.bits === 32;
  const format = isIPv4 ? (ip: bigint) => formatIPv4(Number(ip)) : formatIPv6;
  if (hostBits === 0n || (hostBits === 1n && count === 1)) return [format(base)];
  if (hostBits === 1n) return [format(base), format(base + 1n)];

  const usableCount = (1n << hostBits) - (isIPv4 ? 2n : 1n);
  if (usableCount < BigInt(count)) count = Number(usableCount);

  // Store only the offsets moved by the partial Fisher-Yates shuffle.
  const replacements = new Map<bigint, bigint>();
  const result = [];
  for (let remaining = usableCount; result.length < count; remaining--) {
    const index = randomBigIntInRange(1n, remaining);
    const offset = replacements.get(index) ?? index;
    replacements.set(index, replacements.get(remaining) ?? remaining);
    replacements.delete(remaining);
    result.push(format(base + offset));
  }
  return result;
}

function randomBigIntInRange(min: bigint, max: bigint): bigint {
  const range = max - min + 1n;
  if (range === 1n) return min;
  const bits = (range - 1n).toString(2).length;
  const bytes = Math.ceil(bits / 8);
  const buf = new Uint8Array(bytes);
  const mask = (1n << BigInt(bits)) - 1n;

  while (true) {
    if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
      crypto.getRandomValues(buf);
    } else {
      for (let i = 0; i < bytes; i++) {
        buf[i] = Math.floor(Math.random() * 256);
      }
    }
    let val = 0n;
    for (let i = 0; i < bytes; i++) {
      val = (val << 8n) | BigInt(buf[i]);
    }
    val &= mask;
    if (val < range) {
      return min + val;
    }
  }
}
