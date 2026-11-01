import { describe, expect, it } from 'vitest';
import { catalogRule, putCatalogRule } from '@/pages/xray/routing/catalog-rules';
import { RuleObjectSchema, type RuleObject } from '@/schemas/routing';
import {
  nativeCatalogRule,
  putNativeCatalogRule,
  catalogAssignedRules,
} from '@/pages/singbox/catalog-rules';

describe('routing list catalog', () => {
  it('preserves the list reference through schema parsing', () => {
    const rule = catalogRule({ name: 'My list', listId: 7 }, 'vpn');
    expect(RuleObjectSchema.parse(rule).routingListId).toBe(7);
    expect(rule.domain).toBeUndefined();
    expect(rule.ip).toBeUndefined();
  });

  it('inserts before a catch-all, retaining service and specific rules', () => {
    const rules: RuleObject[] = [
      { type: 'field', inboundTag: ['api'], outboundTag: 'api' },
      { type: 'field', ip: ['geoip:private'], outboundTag: 'blocked' },
      { type: 'field', routingListId: 2, outboundTag: 'other' },
      { type: 'field', outboundTag: 'direct' },
    ];
    const next = catalogRule({ name: 'YouTube', token: 'geosite:youtube' }, 'vpn');
    expect(putCatalogRule(rules, next)).toEqual([...rules.slice(0, 3), next, rules[3]]);
    expect(rules).toHaveLength(4);
  });

  it('updates an existing list route in place and keeps its client scope', () => {
    const rules: RuleObject[] = [
      { type: 'field', routingListId: 7, outboundTag: 'old', user: ['alice'] },
      { type: 'field', outboundTag: 'direct' },
    ];
    const next = putCatalogRule(rules, catalogRule({ name: 'My list', listId: 7 }, 'new'));
    expect(next).toHaveLength(2);
    expect(next[0]).toMatchObject({ outboundTag: 'new', user: ['alice'], routingListId: 7 });
  });

  it('ignores disabled catch-all rules when choosing insertion position', () => {
    const rules: RuleObject[] = [{ type: 'field', enabled: false, outboundTag: 'blocked' }];
    const next = catalogRule({ name: 'My list', listId: 7 }, 'vpn');
    expect(putCatalogRule(rules, next)).toEqual([...rules, next]);
  });

  it('keeps native sniff/DNS actions first and inserts before a terminal default', () => {
    const rules = [
      { action: 'sniff' },
      { action: 'hijack-dns', protocol: ['dns'] },
      { action: 'route', domain_suffix: ['private.example'], outbound: 'direct' },
      { action: 'route', outbound: 'direct' },
    ];
    const next = nativeCatalogRule({ name: 'YouTube', token: 'geosite:youtube' }, 'vpn');
    expect(putNativeCatalogRule(rules, next)).toEqual([...rules.slice(0, 3), next, rules[3]]);
    expect(catalogAssignedRules([next])[0]).toMatchObject({
      domain: ['geosite:youtube'],
      ruleTag: 'catalog-geosite:youtube',
      outboundTag: 'vpn',
    });
  });

  it('updates native custom list targets without copying their contents', () => {
    const old = { routingListId: 7, outbound: 'old', inbound: ['client'] };
    const next = nativeCatalogRule({ name: 'My list', listId: 7 }, 'vpn');
    expect(putNativeCatalogRule([old], next)).toEqual([{ ...old, ...next }]);
    expect(next.domain_suffix).toBeUndefined();
    expect(next.ip_cidr).toBeUndefined();
  });
});
