import { describe, expect, it } from 'vitest';
import { basicRule, setBasicRule } from '@/pages/singbox/native-routing-basic';

const config = {
  route: {
    final: 'proxy',
    rules: [{ domain: ['custom.test'], outbound: 'proxy' }, { outbound: 'direct' }],
  },
};
describe('basic native routing rules', () => {
  it('keeps manual rules and priority when changing a basic rule', () => {
    const first = setBasicRule(config, 'blocked-domains', {
      action: 'reject',
      domain_suffix: ['ads.test'],
    });
    const changed = setBasicRule(first, 'blocked-domains', {
      action: 'reject',
      domain_suffix: ['other.test'],
    });
    expect(changed.route).toMatchObject({
      final: 'proxy',
      rules: [{ domain_suffix: ['other.test'], action: 'reject' }, ...config.route.rules],
    });
    expect(basicRule(changed, 'blocked-domains').domain_suffix).toEqual(['other.test']);
  });
  it('removes an empty preset entirely instead of creating a catch-all rule', () => {
    const first = setBasicRule(config, 'torrent', { action: 'reject', protocol: ['bittorrent'] });
    expect(setBasicRule(first, 'torrent', { action: 'reject', protocol: [] })).toEqual(config);
    expect(
      setBasicRule(config, 'direct-domains', { outbound: 'direct', domain_suffix: [] }),
    ).toEqual(config);
  });
});
