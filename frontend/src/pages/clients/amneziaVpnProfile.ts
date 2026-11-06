// AmneziaVPN 5.0.3.0 ImportController and qrCodeUtils wire format.
// Profiles contain connection credentials only. No SSH or panel credentials.
export function amneziaProfile(config: string, description: string) {
  const fields: Record<string, string> = {};
  for (const line of config.split('\n')) {
    const match = /^([A-Za-z0-9]+)\s*=\s*(.*)$/.exec(line.trim());
    if (match) fields[match[1]] = match[2];
  }
  const endpoint = /^(\[[^\]]+\]|[^:]+):(\d+)$/.exec(fields.Endpoint ?? '');
  if (!endpoint || !fields.PrivateKey || !fields.PublicKey || !fields.Address)
    throw new Error('Invalid AWG connection');
  const host = endpoint[1].replace(/^\[|\]$/g, '');
  const lastConfig: Record<string, unknown> = {
    config,
    hostName: host,
    port: Number(endpoint[2]),
    client_priv_key: fields.PrivateKey,
    client_ip: fields.Address,
    server_pub_key: fields.PublicKey,
    allowed_ips: (fields.AllowedIPs ?? '')
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean),
    mtu: fields.MTU,
    persistent_keep_alive: fields.PersistentKeepalive,
  };
  if (fields.PresharedKey) lastConfig.psk_key = fields.PresharedKey;
  for (const [key, value] of Object.entries(fields)) {
    if (
      /^(Jc|Jmin|Jmax|S[1-4]|H[1-4]|I[1-5]|HeaderProtectionKey|ContentPaddingAddition|RekeyAfterTime|RekeyTimeout|RejectAfterTime|KeepaliveTimeout|MaxHandshakeAttempts|RandomTrailers|DisableCookies)$/.test(
        key,
      )
    )
      lastConfig[key] = value;
  }
  const dns = (fields.DNS ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
  return {
    hostName: host,
    description,
    defaultContainer: 'amnezia-awg',
    containers: [
      {
        container: 'amnezia-awg',
        awg: {
          last_config: JSON.stringify(lastConfig),
          isThirdPartyConfig: true,
          port: endpoint[2],
          transport_proto: 'udp',
        },
      },
    ],
    ...(dns[0] ? { dns1: dns[0] } : {}),
    ...(dns[1] ? { dns2: dns[1] } : {}),
  };
}

function base64Url(bytes: Uint8Array): string {
  // Avoid spreading a potentially large array into a function's arguments.
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export function amneziaQrChunks(data: Uint8Array): string[] {
  const size = 850;
  const total = Math.ceil(data.length / size);
  if (total < 1 || total > 255) throw new Error('Profile is too large');
  return Array.from({ length: total }, (_, index) => {
    const part = data.subarray(index * size, (index + 1) * size);
    // QDataStream: int16 magic, uint8 count/index, uint32 QByteArray size.
    const framed = new Uint8Array(8 + part.length);
    const view = new DataView(framed.buffer);
    view.setInt16(0, 1984);
    view.setUint8(2, total);
    view.setUint8(3, index);
    view.setUint32(4, part.length);
    framed.set(part, 8);
    return base64Url(framed);
  });
}

export async function buildAmneziaVpnProfile(config: string, description: string) {
  const json = new TextEncoder().encode(JSON.stringify(amneziaProfile(config, description)));
  let payload: Uint8Array = json;
  if (typeof CompressionStream !== 'undefined') {
    const compressed = new Uint8Array(
      await new Response(
        new Blob([json]).stream().pipeThrough(new CompressionStream('deflate')),
      ).arrayBuffer(),
    );
    // qCompress adds the uncompressed size in big endian before zlib data.
    payload = new Uint8Array(4 + compressed.length);
    new DataView(payload.buffer).setUint32(0, json.length);
    payload.set(compressed, 4);
  }
  // ImportController supports both qCompressed JSON and plain JSON.
  return { link: `vpn://${base64Url(payload)}`, qr: amneziaQrChunks(payload) };
}
