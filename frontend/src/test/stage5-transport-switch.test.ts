import { describe, expect, it } from 'vitest';
import { normalizeStreamSettingsForWire } from '@/lib/xray/stream-wire-normalize';

describe('transport switches on the save path', () => {
  const networks = [
    'tcp',
    'raw',
    'ws',
    'http',
    'grpc',
    'httpupgrade',
    'xhttp',
    'xdrive',
    'kcp',
    'quic',
    'hysteria',
  ];
  for (const side of ['inbound', 'outbound'] as const) {
    for (const network of networks) {
      it(`${side}/${network} discards inactive subforms without changing the form`, () => {
        const stream: Record<string, unknown> = { network, futureOption: { enabled: true } };
        for (const n of networks) stream[`${n}Settings`] = { path: `/${n}` };
        const before = JSON.stringify(stream);
        const wire = normalizeStreamSettingsForWire(stream, { side });
        for (const n of networks) {
          const active =
            n === network || (['tcp', 'raw'].includes(network) && ['tcp', 'raw'].includes(n));
          expect(Object.hasOwn(wire, `${n}Settings`)).toBe(active);
        }
        expect(wire.futureOption).toEqual({ enabled: true });
        expect(JSON.stringify(stream)).toBe(before);
      });
    }
  }
});
