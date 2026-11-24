import { HysteriaObfsSchema } from '@/schemas/protocols/stream/hysteria';
import type { z } from 'zod';

type Raw = Record<string, unknown>;
type Obfs = z.infer<typeof HysteriaObfsSchema>;

function record(value: unknown): Raw | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as Raw) : undefined;
}

// Only convert the known legacy representation; preserve unfamiliar settings
// for the advanced editor rather than silently discarding them.
function legacyObfs(mask: unknown): Obfs | undefined {
  const item = record(mask);
  if (item?.type !== 'salamander') return undefined;
  const settings = record(item.settings);
  if (!settings || Object.keys(settings).some((key) => key !== 'password' && key !== 'packetSize'))
    return undefined;
  if (typeof settings.password !== 'string' || !settings.password.trim()) return undefined;
  if (settings.packetSize === undefined || settings.packetSize === '')
    return { type: 'salamander', password: settings.password };
  if (typeof settings.packetSize !== 'string' || !/^\d+-\d+$/.test(settings.packetSize))
    return undefined;
  const [minPacketSize, maxPacketSize] = settings.packetSize.split('-').map(Number);
  const parsed = HysteriaObfsSchema.safeParse({
    type: 'gecko',
    password: settings.password,
    minPacketSize,
    maxPacketSize,
  });
  return parsed.success ? parsed.data : undefined;
}

export function newHysteriaObfsPassword(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
    byte.toString(16).padStart(2, '0'),
  ).join('');
}

export function selectHysteriaObfs(
  kind: string,
  current: unknown,
  inbound: boolean,
): Obfs | undefined {
  if (kind !== 'salamander' && kind !== 'gecko') return undefined;
  const previous = record(current);
  const password = typeof previous?.password === 'string' ? previous.password : '';
  const next: Obfs = {
    type: kind,
    password: password || (inbound ? newHysteriaObfsPassword() : ''),
  };
  if (kind === 'gecko') {
    next.minPacketSize =
      previous?.type === 'gecko' && typeof previous.minPacketSize === 'number'
        ? previous.minPacketSize
        : 512;
    next.maxPacketSize =
      previous?.type === 'gecko' && typeof previous.maxPacketSize === 'number'
        ? previous.maxPacketSize
        : 1200;
  }
  return next;
}

// The native Hysteria2 editor owns BBR and bandwidth. Preserve unfamiliar
// legacy rate units as explicit overrides instead of guessing a conversion.
function legacyMbps(value: unknown): number | undefined {
  if (typeof value !== 'string') return undefined;
  const match = /^(\d+)\s*mbps$/i.exec(value.trim());
  if (!match) return undefined;
  const number = Number(match[1]);
  return Number.isSafeInteger(number) ? number : undefined;
}

function hydrateNativeQuic(stream: Raw): Raw {
  const hy = record(stream.hysteriaSettings) ?? {};
  if (hy.version === 1) return stream;
  const fm = record(stream.finalmask);
  const quic = record(fm?.quicParams);
  if (!fm || !quic) return stream;
  const native = { ...hy };
  const legacy = { ...quic };
  let changed = false;
  if (legacy.bbrProfile != null) {
    native.bbrProfile ??= legacy.bbrProfile;
    delete legacy.bbrProfile;
    changed = true;
  }
  for (const [field, oldField] of [
    ['up', 'brutalUp'],
    ['down', 'brutalDown'],
  ]) {
    const rate = legacyMbps(legacy[oldField]);
    if (rate === undefined) continue;
    native[field] ??= rate;
    delete legacy[oldField];
    changed = true;
  }
  return changed
    ? { ...stream, hysteriaSettings: native, finalmask: { ...fm, quicParams: legacy } }
    : stream;
}

function syncNativeQuic(stream: Raw): Raw {
  const hy = record(stream.hysteriaSettings);
  const fm = record(stream.finalmask);
  const quic = record(fm?.quicParams);
  if (!hy || hy.version === 1 || !fm || !quic) return stream;
  const legacy = { ...quic };
  if (legacy.congestion === 'bbr' && hy.bbrProfile != null) legacy.bbrProfile = hy.bbrProfile;
  if (legacy.congestion === 'brutal' || legacy.congestion === 'force-brutal') {
    for (const [field, oldField] of [
      ['up', 'brutalUp'],
      ['down', 'brutalDown'],
    ]) {
      if (legacy[oldField] == null && typeof hy[field] === 'number' && Number(hy[field]) > 0) {
        legacy[oldField] = `${hy[field]} mbps`;
      }
    }
  }
  return { ...stream, finalmask: { ...fm, quicParams: legacy } };
}

// The visual editor owns a single native value. A valid old mask is lifted
// with its original password and bounds, not regenerated on every edit.
export function hydrateHysteriaObfs(stream: Raw): Raw {
  stream = hydrateNativeQuic(stream);
  const hy = record(stream.hysteriaSettings) ?? {};
  if (hy.version === 1) return stream;
  const fm = record(stream.finalmask);
  if (!fm || !Array.isArray(fm.udp)) return stream;
  const masks = fm.udp;
  const legacy = masks.filter((mask) => record(mask)?.type === 'salamander');
  if (legacy.length !== 1) return stream;
  const converted = legacyObfs(legacy[0]);
  if (!converted) return stream;
  const native = HysteriaObfsSchema.safeParse(hy.obfs);
  if (hy.obfs != null && hy.obfs !== '' && !native.success) return stream;
  return {
    ...stream,
    hysteriaSettings: { ...hy, obfs: native.success ? native.data : converted },
    finalmask: { ...fm, udp: masks.filter((mask) => mask !== legacy[0]) },
  };
}

// Xray-compatible exports still need the legacy wire mask. Derive it from
// the one editable native setting; both representations are always identical.
export function syncHysteriaObfsForWire(stream: Raw): Raw {
  stream = syncNativeQuic(stream);
  const hy = record(stream.hysteriaSettings);
  if (!hy || hy.version === 1) return stream;
  const parsed = HysteriaObfsSchema.safeParse(hy.obfs);
  if (!parsed.success) return stream;
  const obfs = parsed.data;
  const fm = record(stream.finalmask) ?? {};
  if (fm.udp != null && !Array.isArray(fm.udp)) return stream;
  const masks: unknown[] = Array.isArray(fm.udp) ? fm.udp : [];
  const legacy = masks.filter((mask) => record(mask)?.type === 'salamander');
  if (legacy.length > 1 || legacy.some((mask) => !legacyObfs(mask))) return stream;
  const settings: Raw = { password: obfs.password };
  if (obfs.type === 'gecko')
    settings.packetSize = `${obfs.minPacketSize ?? 512}-${obfs.maxPacketSize ?? 1200}`;
  const mirror = { type: 'salamander', settings };
  return {
    ...stream,
    finalmask: {
      ...fm,
      udp: legacy.length
        ? masks.map((mask) => (mask === legacy[0] ? mirror : mask))
        : [...masks, mirror],
    },
  };
}
