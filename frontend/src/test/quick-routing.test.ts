import { describe, expect, it } from 'vitest';
import {
  compileQuickRoute,
  diagnoseRouting,
  putQuickRoute,
  removeQuickRoute,
  setQuickRouteEnabled,
  type QuickRouteDraft,
  type RouteRecord,
  type RoutingEngine,
} from '@/pages/xray/routing/quick-routing';

const OUTBOUNDS = ['direct', 'proxy', 'blocked'];
const INBOUNDS = ['in-a', 'in-b', 'api'];
const draft = (changes: Partial<QuickRouteDraft> = {}): QuickRouteDraft => ({
  match: 'inbound',
  inboundTags: ['in-b', 'in-a'],
  values: '',
  domainMode: 'suffix',
  target: 'proxy',
  ...changes,
});

describe.each<RoutingEngine>(['xray', 'sing-box'])('quick routing on %s', (engine) => {
  it('compiles a scoped inbound route with canonical order', () => {
    const rule = compileQuickRoute(draft(), engine, OUTBOUNDS, INBOUNDS);
    const sorted = engine === 'xray' ? rule.rule.inboundTag : rule.rule.inbound;
    expect(sorted).toEqual(['in-a', 'in-b']);
    expect(engine === 'xray' ? rule.rule.outboundTag : rule.rule.outbound).toBe('proxy');
    const repeat = compileQuickRoute(draft({ inboundTags: ['in-a', 'in-b'] }), engine, OUTBOUNDS, INBOUNDS);
    expect(repeat.id).toBe(rule.id);
  });

  it('upserts existing managed route instead of creating duplicates', () => {
    const first = compileQuickRoute(draft(), engine, OUTBOUNDS, INBOUNDS);
    const changed = compileQuickRoute(draft({ target: 'direct' }), engine, OUTBOUNDS, INBOUNDS);
    const original = putQuickRoute([], first, engine);
    const modified = putQuickRoute(original.rules, changed, engine);
    expect(original.replaced).toBe(false);
    expect(modified.replaced).toBe(true);
    expect(modified.rules).toHaveLength(1);
    expect(engine === 'xray' ? modified.rules[0].outboundTag : modified.rules[0].outbound).toBe('direct');
    expect(original.rules[0]).toEqual(first.rule);
  });

  it('keeps service rules and catch-all in place while inserting an exception', () => {
    const apiRule: RouteRecord = engine === 'xray'
      ? { type: 'field', inboundTag: ['api'], outboundTag: 'api' }
      : { inbound: ['api'], action: 'route', outbound: 'direct' };
    const fallback: RouteRecord = engine === 'xray'
      ? { type: 'field', outboundTag: 'direct' }
      : { action: 'route', outbound: 'direct' };
    const original = [apiRule, fallback];
    const result = putQuickRoute(original, compileQuickRoute(draft(), engine, OUTBOUNDS, INBOUNDS), engine);
    expect(result.rules).toHaveLength(3);
    expect(result.rules[0]).toEqual(apiRule);
    expect(result.rules[2]).toEqual(fallback);
    expect(original).toEqual([apiRule, fallback]);
  });

  it('compiles domain suffix, exact match and IP into native core syntax', () => {
    const suffix = compileQuickRoute(
      draft({ match: 'domain', values: '*.Example.com, example.org', inboundTags: [] }),
      engine, OUTBOUNDS, INBOUNDS,
    );
    if (engine === 'xray') expect(suffix.rule.domain).toEqual(['domain:example.com', 'domain:example.org']);
    else expect(suffix.rule.domain_suffix).toEqual(['example.com', 'example.org']);
    const exact = compileQuickRoute(
      draft({ match: 'domain', values: 'example.com', domainMode: 'exact' }),
      engine, OUTBOUNDS, INBOUNDS,
    );
    if (engine === 'xray') expect(exact.rule.domain).toEqual(['full:example.com']);
    else expect(exact.rule.domain).toEqual(['example.com']);
    const ip = compileQuickRoute(draft({ match: 'ip', values: '192.0.2.0/24' }), engine, OUTBOUNDS, INBOUNDS);
    expect(ip.rule[engine === 'xray' ? 'ip' : 'ip_cidr']).toEqual(['192.0.2.0/24']);
  });

  it('compiles ports without losing ranges or generating invalid values', () => {
    const entry = compileQuickRoute(draft({ match: 'port', values: '443, 1000-2000' }), engine, OUTBOUNDS, INBOUNDS);
    if (engine === 'xray') expect(entry.rule.port).toBe('1000-2000,443');
    else {
      expect(entry.rule.port).toEqual([443]);
      expect(entry.rule.port_range).toEqual(['1000:2000']);
    }
  });

  it('never deletes or toggles unmanaged rules', () => {
    const special = { custom: { keep: true }, action: 'route', inbound: ['in-a'], outbound: 'direct' };
    const managed = compileQuickRoute(draft(), engine, OUTBOUNDS, INBOUNDS);
    const mixed = [special, managed.rule];
    expect(removeQuickRoute(mixed, 'fake', engine)).toEqual(mixed);
    const toggled = setQuickRouteEnabled(mixed, managed.id, false, engine);
    expect(toggled[0]).toEqual(special);
    expect(toggled[1].enabled).toBe(false);
    expect(removeQuickRoute(toggled, managed.id, engine)).toEqual([special]);
  });

  it('rejects invalid or missing route targets and matchers', () => {
    expect(() => compileQuickRoute(draft({ target: 'missing' }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    expect(() => compileQuickRoute(draft({ inboundTags: ['gone'] }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    expect(() => compileQuickRoute(draft({ match: 'inbound', inboundTags: [] }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    for (const values of ['foo..bar', 'example.com/hello', 'https://example.com']) {
      expect(() => compileQuickRoute(draft({ match: 'domain', values }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    }
    for (const values of ['999.1.1.1', '1.1.1.1/33', '192.0.2.3/abc']) {
      expect(() => compileQuickRoute(draft({ match: 'ip', values }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    }
    for (const values of ['0', '65536', '8000-7000', '22:23']) {
      expect(() => compileQuickRoute(draft({ match: 'port', values }), engine, OUTBOUNDS, INBOUNDS)).toThrow();
    }
  });

  it('detects missing outbound and unreachable quick rule after catch-all', () => {
    const managed = compileQuickRoute(draft(), engine, OUTBOUNDS, INBOUNDS);
    const catchAll: RouteRecord = engine === 'xray'
      ? { type: 'field', outboundTag: 'direct' } : { action: 'route', outbound: 'direct' };
    const issues = diagnoseRouting([catchAll, managed.rule], engine, ['direct']);
    expect(issues.map((issue) => issue.code)).toContain('missing-target');
    expect(issues.map((issue) => issue.code)).toContain('catch-all');
    expect(issues.map((issue) => issue.code)).toContain('unconditional');
  });
});

describe('sing-box reject action', () => {
  it('allows explicit rejection without fake outbound', () => {
    const rule = compileQuickRoute(draft({ target: '@reject' }), 'sing-box', OUTBOUNDS, INBOUNDS);
    expect(rule.rule).toMatchObject({ action: 'reject', inbound: ['in-a', 'in-b'] });
    expect(rule.rule.outbound).toBeUndefined();
  });
  it('does not invent a synthetic rejection destination in Xray', () => {
    expect(() => compileQuickRoute(draft({ target: '@reject' }), 'xray', OUTBOUNDS, INBOUNDS)).toThrow();
  });
});
