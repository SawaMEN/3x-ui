export const OPENFLUX_TRANSPORTS = [
  'direct',
  'yandex',
  'vyandex',
  'boards',
  'mailru',
  'cupsonline',
  'avito',
  'telemost',
  'oneme',
] as const;
export type OpenFluxTransportType = (typeof OPENFLUX_TRANSPORTS)[number];
export type OpenFluxTransport = {
  type: OpenFluxTransportType;
  url: string;
  priority: number;
  dial?: string;
  token?: string;
  uid?: string;
};

export function validOpenFluxChannelURL(value: string): boolean {
  if (value.trim() !== value || /[\r\n\0#;]/.test(value)) return false;
  try {
    const url = new URL(value);
    return url.protocol === 'https:' && !!url.hostname && !url.username && !url.password;
  } catch {
    return false;
  }
}

export function suggestOpenFluxTransport(value: string): OpenFluxTransportType | undefined {
  try {
    const url = new URL(value);
    if (url.hostname === 'docs.yandex.ru' || url.hostname === 'docs.yandex.com') return 'vyandex';
    if (url.hostname === 'boards.yandex.ru') return 'boards';
    if (url.hostname === 'docs.mail.ru') return 'mailru';
    if (url.hostname === 'cups.online') return 'cupsonline';
    if (url.hostname === 'code.avito.ru') return 'avito';
    if (url.hostname === 'telemost.yandex.ru' || url.hostname === 'telemost.yandex.com')
      return 'telemost';
  } catch {
    // Incomplete URLs stay editable.
  }
  return undefined;
}

export function openFluxTransportLabel(type: OpenFluxTransportType): string {
  return {
    direct: 'Direct (TCP)',
    yandex: 'Yandex Docs (WebSocket)',
    vyandex: 'Yandex Docs (Volga)',
    boards: 'Yandex Boards',
    mailru: 'Mail.ru Docs',
    cupsonline: 'Cups.online',
    avito: 'Avito Code',
    telemost: 'Yandex Telemost',
    oneme: 'Web MAX (WebRTC)',
  }[type];
}

export function validOpenFluxMAXToken(value: string): boolean {
  return (
    value.length > 0 &&
    new TextEncoder().encode(value).length <= 16384 &&
    !/[\x00-\x20\x7f-\uffff#;]/.test(value)
  );
}

export function validOpenFluxMAXUID(value: string): boolean {
  return /^[0-9]{1,19}$/.test(value) && BigInt(value) > 0n && BigInt(value) <= 9223372036854775807n;
}

export function openFluxChannelValid(value: OpenFluxTransport, client = false): boolean {
  if (value.type === 'direct') return !client || !!value.dial?.trim();
  if (value.type === 'oneme')
    return (
      validOpenFluxMAXToken(value.token ?? '') &&
      (client || value.uid ? validOpenFluxMAXUID(value.uid ?? '') : true)
    );
  return validOpenFluxChannelURL(value.url ?? '');
}

// Server credentials are intentionally not an input to the client bundle.
export function openFluxMAXClientFiles(
  secret: string,
  context: string,
  serverID: string,
  clientToken: string,
  instructions: string,
): Record<string, string> | undefined {
  if (
    secret.length < 16 ||
    secret.trim() !== secret ||
    /[\r\n\0]/.test(secret) ||
    !context ||
    context.trim() !== context ||
    /[\r\n\0]/.test(context) ||
    !validOpenFluxMAXUID(serverID) ||
    !validOpenFluxMAXToken(clientToken)
  )
    return;
  const quotedContext = "'" + context.replaceAll("'", "'\"'\"'") + "'";
  return {
    'secret.txt': secret,
    'openflux-client.conf': `[Interface]\nRole = client\nInbound = socks5\nSocks5 = 127.0.0.1:1080\nCodec = batched\nEncryptionKeyFile = secret.txt\nSessionContext = ${/[#;]/.test(context) ? '' : context}\n\n[Transport "oneme"]\nType = oneme\nPriority = 100\nToken = ${clientToken}\nUID = ${serverID}\n`,
    'README.txt':
      instructions +
      '\n\nopenflux --config openflux-client.conf --negotiate --session-context ' +
      quotedContext +
      '\n',
  };
}

export function openFluxMAXChannels(settings: unknown): { present: boolean; only: boolean } {
  try {
    const parsed = typeof settings === 'string' ? JSON.parse(settings) : settings;
    const transports: unknown =
      parsed && typeof parsed === 'object' ? parsed.transports : undefined;
    if (!Array.isArray(transports)) return { present: false, only: false };
    const max = transports.filter((entry) => entry?.type === 'oneme');
    return { present: max.length > 0, only: max.length > 0 && max.length === transports.length };
  } catch {
    return { present: false, only: false };
  }
}
