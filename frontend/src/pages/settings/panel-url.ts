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
): string {
  const url = new URL(current);
  const publicPort = Number(url.port || (url.protocol === 'https:' ? 443 : 80));
  const initialProtocol = initial.webCertFile && initial.webKeyFile ? 'https:' : 'http:';
  const direct = publicPort === initial.webPort && url.protocol === initialProtocol;
  const hostname = saved.webDomain.trim() || url.hostname;
  url.hostname = hostname.includes(':') && !hostname.startsWith('[') ? `[${hostname}]` : hostname;
  if (direct) {
    url.protocol = saved.webCertFile && saved.webKeyFile ? 'https:' : 'http:';
    url.port = String(saved.webPort);
  }
  const path = (saved.webBasePath || '/').replace(/^\/+|\/+$/g, '');
  url.pathname = `/${path ? `${path}/` : ''}panel/settings`;
  url.search = '';
  url.hash = '';
  return url.href;
}
