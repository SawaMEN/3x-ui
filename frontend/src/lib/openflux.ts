export const OPENFLUX_TRANSPORTS = ['direct', 'yandex', 'vyandex', 'boards', 'mailru'] as const;
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
  } catch {
    // Incomplete URLs stay editable.
  }
  return undefined;
}
