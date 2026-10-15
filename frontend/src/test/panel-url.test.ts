import { describe, expect, it } from 'vitest';
import { panelUrlAfterRestart } from '@/pages/settings/panel-url';

const initial = { webDomain: '', webPort: 80, webBasePath: '/', webCertFile: '', webKeyFile: '' };
describe('panel address after restart', () => {
  it('changes a direct hostname listener on the default HTTP port', () => {
    expect(
      panelUrlAfterRestart('http://panel.example/panel/settings#general', initial, {
        ...initial,
        webPort: 8080,
        webBasePath: 'new',
      }),
    ).toBe('http://panel.example:8080/new/panel/settings');
  });
  it('keeps the public HTTPS origin behind a reverse proxy', () => {
    const behindProxy = { ...initial, webPort: 2053 };
    expect(
      panelUrlAfterRestart('https://panel.example/old/panel/settings', behindProxy, {
        ...behindProxy,
        webBasePath: '/new/',
        webPort: 8080,
      }),
    ).toBe('https://panel.example/new/panel/settings');
  });
  it('handles direct TLS removal and IPv6', () => {
    const tls = { ...initial, webPort: 443, webCertFile: 'cert', webKeyFile: 'key' };
    expect(
      panelUrlAfterRestart('https://[::1]/panel/settings', tls, {
        ...initial,
        webPort: 8080,
      }),
    ).toBe('http://[::1]:8080/panel/settings');
  });
  it('applies domain validation and clears old queries', () => {
    expect(
      panelUrlAfterRestart('http://127.0.0.1/panel/settings?old=1', initial, {
        ...initial,
        webDomain: 'panel.example',
        webBasePath: '/secret/',
      }),
    ).toBe('http://panel.example/secret/panel/settings');
  });
});
