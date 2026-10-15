import { normalizePath } from './uriPath';
type PanelAddress = {
  webDomain: string;
  webPort: number;
  webBasePath: string;
  webCertFile: string;
  webKeyFile: string;
};

export function panelUrlAfterRestart(
  current: string,
  initial: PanelAddress,
  saved: PanelAddress,
  portOverride = 0,
): string {
  const url = new URL(current);
  const publicPort = Number(url.port || (url.protocol === 'https:' ? 443 : 80));
  const initialProtocol = initial.webCertFile && initial.webKeyFile ? 'https:' : 'http:';
  const direct = publicPort === initial.webPort && url.protocol === initialProtocol;
  const hostname = saved.webDomain.trim() || url.hostname;
  url.hostname = hostname.includes(':') && !hostname.startsWith('[') ? `[${hostname}]` : hostname;
  if (direct) {
    url.protocol = saved.webCertFile && saved.webKeyFile ? 'https:' : 'http:';
    url.port = String(portOverride || saved.webPort);
  }
  url.pathname = `${normalizePath(saved.webBasePath)}panel/settings`;
  url.search = '';
  url.hash = '';
  return url.href;
}

export async function waitForPanelRestart(
  target: string,
  current: string,
  instance: string,
): Promise<boolean> {
  const crossOrigin = new URL(target).origin !== new URL(current).origin;
  const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));
  if (!instance) await sleep(3500);
  let failures = 0;
  for (let attempt = 0; attempt < 40; attempt++) {
    try {
      const response = await fetch(crossOrigin ? current : target, {
        cache: 'no-store',
        signal: AbortSignal.timeout(1500),
      });
      failures = 0;
      const next = response.headers.get('X-Panel-Instance');
      if (instance && next && next !== instance) return true;
      if (!instance && response.ok) return true;
    } catch {
      failures++;
      if (crossOrigin && failures >= 2) {
        await sleep(1500);
        return true;
      }
    }
    await sleep(500);
  }
  return false;
}
