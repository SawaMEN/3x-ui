import {
  hydrateHysteriaObfs,
  selectHysteriaObfs,
  syncHysteriaObfsForWire,
} from '@/lib/xray/hysteria-obfs';
import { describe, expect, it } from 'vitest';
import { fillStreamDefaults } from '@/lib/xray/stream-defaults';
import { formValuesToWirePayload, rawInboundToFormValues } from '@/lib/xray/inbound-form-adapter';
import { HysteriaStreamSettingsSchema } from '@/schemas/protocols/stream/hysteria';

describe('native Hysteria settings', () => {
  it('retains bandwidth, BBR, obfs and extension options when editing', () => {
    const settings = {
      version: 2,
      up: 100,
      down: 250,
      ignoreClientBandwidth: true,
      bbrProfile: 'aggressive',
      obfs: { type: 'gecko', password: 'secret', minPacketSize: 512, maxPacketSize: 1200 },
      hopInterval: '10s',
    };
    const stream = fillStreamDefaults({
      network: 'hysteria',
      security: 'tls',
      hysteriaSettings: settings,
    });
    expect(stream.hysteriaSettings).toMatchObject(settings);
    const form = rawInboundToFormValues({
      protocol: 'hysteria',
      port: 443,
      settings: JSON.stringify({ version: 2, clients: [] }),
      streamSettings: JSON.stringify(stream),
    });
    const wire = formValuesToWirePayload(form);
    expect(JSON.parse(wire.streamSettings).hysteriaSettings).toMatchObject(settings);
  });
  it('preserves version 1 instead of silently changing the protocol', () => {
    expect(
      HysteriaStreamSettingsSchema.parse({ version: 1, up: 100, down: 100, obfs: 'password' })
        .version,
    ).toBe(1);
  });
  it('rejects reversed Gecko packet bounds and invalid HTTP status codes', () => {
    expect(
      HysteriaStreamSettingsSchema.safeParse({
        obfs: { type: 'gecko', password: 'x', minPacketSize: 1200, maxPacketSize: 512 },
      }).success,
    ).toBe(false);
    expect(
      HysteriaStreamSettingsSchema.safeParse({ masquerade: { type: 'string', statusCode: 42 } })
        .success,
    ).toBe(false);
  });
});

it('normalizes native aliases so edited values take effect', () => {
  const normalized = HysteriaStreamSettingsSchema.parse({
    up_mbps: 100,
    downMbps: 200,
    bbr_profile: 'standard',
    obfs: { type: 'gecko', password: 'p', min_packet_size: 512, max_packet_size: 1200 },
  });
  expect(normalized).toMatchObject({
    up: 100,
    down: 200,
    bbrProfile: 'standard',
    obfs: { minPacketSize: 512, maxPacketSize: 1200 },
  });
  expect(normalized.up_mbps).toBeUndefined();
});
it('keeps invalid native settings visible instead of erasing credentials', () => {
  const stream = fillStreamDefaults({
    network: 'hysteria',
    hysteriaSettings: {
      obfs: { type: 'gecko', password: 'secret', minPacketSize: 1200, maxPacketSize: 512 },
    },
  });
  expect(stream.hysteriaSettings).toMatchObject({
    obfs: { password: 'secret', minPacketSize: 1200, maxPacketSize: 512 },
  });
});

describe('automatic Hysteria obfs', () => {
  const legacy = {
    network: 'hysteria',
    hysteriaSettings: { version: 2 },
    finalmask: {
      udp: [{ type: 'salamander', settings: { password: 'existing', packetSize: '600-1100' } }],
      quicParams: { congestion: 'bbr' },
    },
  };
  it('lifts legacy credentials without changing the source and mirrors them on save', () => {
    const form = hydrateHysteriaObfs(legacy);
    expect(form.hysteriaSettings).toMatchObject({
      obfs: { type: 'gecko', password: 'existing', minPacketSize: 600, maxPacketSize: 1100 },
    });
    expect(form.finalmask).toMatchObject({ udp: [], quicParams: { congestion: 'bbr' } });
    expect(syncHysteriaObfsForWire(form).finalmask).toEqual(legacy.finalmask);
    expect(legacy.finalmask.udp).toHaveLength(1);
  });
  it('preserves passwords across modes and generates only on inbound', () => {
    expect(selectHysteriaObfs('gecko', { type: 'salamander', password: 'existing' }, true)).toEqual(
      {
        type: 'gecko',
        password: 'existing',
        minPacketSize: 512,
        maxPacketSize: 1200,
      },
    );
    expect(selectHysteriaObfs('salamander', undefined, true)?.password).toMatch(/^[a-f0-9]{32}$/);
    expect(selectHysteriaObfs('salamander', undefined, false)?.password).toBe('');
    expect(selectHysteriaObfs('', undefined, true)).toBeUndefined();
  });
  it('keeps unsupported legacy settings and Hysteria1 unchanged', () => {
    const unsupported = {
      ...legacy,
      finalmask: {
        udp: [{ type: 'salamander', settings: { password: 'existing', unknown: true } }],
      },
    };
    expect(hydrateHysteriaObfs(unsupported)).toBe(unsupported);
    const v1 = { ...legacy, hysteriaSettings: { version: 1, obfs: 'existing' } };
    expect(hydrateHysteriaObfs(v1)).toBe(v1);
  });
});

it('uses a single native editor for Hysteria2 BBR and legacy Mbps rates', () => {
  const form = hydrateHysteriaObfs({
    hysteriaSettings: { version: 2, up: 75 },
    finalmask: {
      quicParams: {
        congestion: 'brutal',
        bbrProfile: 'standard',
        brutalUp: '60 mbps',
        brutalDown: '100 mbps',
      },
    },
  });
  expect(form.hysteriaSettings).toMatchObject({
    version: 2,
    up: 75,
    down: 100,
    bbrProfile: 'standard',
  });
  expect(form.finalmask).toEqual({ quicParams: { congestion: 'brutal' } });
  expect(syncHysteriaObfsForWire(form).finalmask).toEqual({
    quicParams: { congestion: 'brutal', brutalUp: '75 mbps', brutalDown: '100 mbps' },
  });
  const legacy = {
    hysteriaSettings: { version: 2 },
    finalmask: { quicParams: { brutalUp: '1 gbps' } },
  };
  expect(hydrateHysteriaObfs(legacy)).toBe(legacy);
});
