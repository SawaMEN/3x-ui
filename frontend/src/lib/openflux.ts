export const OPENFLUX_TRANSPORTS = [
  'direct',
  'yandex',
  'vyandex',
  'boards',
  'mailru',
  'cupsonline',
  'avito',
  'telemost',
] as const;
export type OpenFluxTransportType = (typeof OPENFLUX_TRANSPORTS)[number];
export type OpenFluxTransport = {
  type: OpenFluxTransportType;
  url: string;
  priority: number;
  dial?: string;
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
  }[type];
}
