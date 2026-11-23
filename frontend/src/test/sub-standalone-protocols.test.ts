import { describe, expect, it } from 'vitest';
import { isStandaloneLink, pingtunnelClientJSON } from '@/pages/sub/standaloneProtocols';

describe('standalone subscription protocols', () => {
  it('classifies external clients without mixing native core links', () => {
    for (const protocol of [
      'pingtunnel://',
      'fptn:',
      'openflux://',
      'vetroff://',
      'tt://',
      'sudoku://',
    ]) {
      expect(isStandaloneLink(protocol)).toBe(true);
    }
    for (const protocol of [
      'vless://',
      'vmess://',
      'ss://',
      'hysteria2://',
      'tuic://',
      'wg://',
      'tg://',
    ]) {
      expect(isStandaloneLink(protocol)).toBe(false);
    }
  });

  it('extracts the original Pingtunnel 3.x client JSON', () => {
    const cfg = {
      type: 'client',
      listen: '127.0.0.1:1080',
      server: 'vpn.example.org',
      socks5: true,
      password: 'secure-password',
      encrypt: 'chacha20',
      congestion: 'bb',
    };
    const encoded = btoa(JSON.stringify(cfg))
      .replace(/\+/g, '-')
      .replace(/\//g, '_')
      .replace(/=+$/, '');
    expect(JSON.parse(pingtunnelClientJSON(`pingtunnel://${encoded}#Tunnel`))).toEqual(cfg);
    expect(pingtunnelClientJSON('vless://something')).toBe('');
    expect(pingtunnelClientJSON('pingtunnel://bad!!!')).toBe('');
    const valid = { ...cfg, server: '127.example.org', password: 'тест' };
    const utf8 = new TextEncoder().encode(JSON.stringify(valid));
    const unicodeLink = btoa(String.fromCharCode(...utf8))
      .replaceAll('+', '-')
      .replaceAll('/', '_')
      .replace(/=+$/, '');
    expect(JSON.parse(pingtunnelClientJSON(`pingtunnel://${unicodeLink}`))).toEqual(valid);
    for (const broken of [
      { ...cfg, server: '::1' },
      { ...cfg, server: 'vpn..example.org' },
      { ...cfg, encrypt: 'aes192' },
      { ...cfg, encrypt: ['chacha20'] },
      { ...cfg, congestion: ['bb'] },
      { ...cfg, congestion: 'unknown' },
      { ...cfg, listen: 'not-a-port' },
      { ...cfg, listen: '127.0.0.1:0' },
      { ...cfg, listen: '127.0.0.1:wrong' },
    ]) {
      const raw = btoa(JSON.stringify(broken))
        .replaceAll('+', '-')
        .replaceAll('/', '_')
        .replace(/=+$/, '');
      expect(pingtunnelClientJSON(`pingtunnel://${raw}`)).toBe('');
    }
  });
});
