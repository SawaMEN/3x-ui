export type ManagerClientState = 'active' | 'disabled' | 'expired' | 'quota';

export interface ManagerClientStatus {
  enabled: boolean;
  expiryTime: number;
  up: number;
  down: number;
  total: number;
}

export function getManagerClientState(
  client: ManagerClientStatus | undefined,
  now: number = Date.now(),
): ManagerClientState {
  if (!client) return 'active';
  if (!client.enabled) return 'disabled';
  if (client.expiryTime > 0 && client.expiryTime <= now) return 'expired';
  if (client.total > 0 && client.up + client.down >= client.total) return 'quota';
  return 'active';
}

export function buildManagerSubscriptionUrl(
  token: string,
  format: string,
  basePath: string,
  origin: string,
): string {
  const url = new URL(basePath.replace(/\/+$/, '') + '/bus/' + encodeURIComponent(token), origin);
  if (format && format !== 'base64') url.searchParams.set('format', format);
  return url.toString();
}

export function managerDate(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString();
}
