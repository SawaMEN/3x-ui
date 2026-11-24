import { describe, expect, it } from 'vitest';
import { compareClientDiagnostics, parseClientDiagnostic } from '@/pages/settings/vetroff-diagnostic-report';

const ok = {stage:'tls',outcome:'ok',durationMs:25};
const timeout = {stage:'tls',outcome:'timeout',durationMs:4000};
const base = {version:1,scope:'client-physical',host:'test.example.com',controlHost:'control.example.com',
  ip:'1.1.1.1',fingerprint:'go-standard',checkedAt:'2026-11-25T10:00:00.123456789Z',
  baseline:[ok,ok],burst:[timeout,ok,ok,ok],control:ok,evidence:'no-signal'};
const parse = (patch = {}) => parseClientDiagnostic(JSON.stringify({...base,...patch}));
const now = Date.parse('2026-11-25T10:05:00Z');

describe('VetrOFF-Core diagnostic reports',()=>{
  it('recomputes evidence and accepts Go nanosecond timestamps',()=>{
    expect(parse().evidence).toBe('asymmetry');
    expect(parse({burst:[ok,ok,ok,ok],evidence:'asymmetry'}).evidence).toBe('no-signal');
    expect(parse({control:timeout}).evidence).toBe('inconclusive');
    expect(parse({ip:'',baseline:null,burst:null,control:undefined}).evidence).toBe('inconclusive');
  });
  it('accepts explicit diagnostic TLS presets but compares matching profiles only',()=>{
    for (const fingerprint of ['go-standard','utls-chrome-auto','utls-firefox-auto','utls-safari-auto']) {
      const physical=parse({fingerprint});
      const tunnel=parse({scope:'client-tunnel',fingerprint,burst:[ok,ok,ok,ok]});
      expect(physical.fingerprint).toBe(fingerprint);
      expect(compareClientDiagnostics(physical,tunnel,now)).toBe('physical-only-signal');
    }
    expect(compareClientDiagnostics(parse({fingerprint:'utls-chrome-auto'}),parse({scope:'client-tunnel',fingerprint:'utls-firefox-auto'}),now)).toBe('not-comparable');
    expect(()=>parse({fingerprint:'randomized'})).toThrow();
  });
  it('rejects malformed report fields',()=>{
    for(const patch of [{version:2},{scope:'server'},{fingerprint:'chrome'},{ip:'not-ip'},
      {baseline:[ok,ok,ok]},{burst:[{...ok,durationMs:-1}]},{host:'http://bad'},
      {checkedAt:'yesterday'},{baseline:[{...ok,stage:'tcp'}]}]) expect(()=>parse(patch)).toThrow();
  });
  it('compares matching, recent, opposite paths only',()=>{
    const physical=parse();
    const tunnel=parse({scope:'client-tunnel',burst:[ok,ok,ok,ok],checkedAt:'2026-11-25T10:02:00Z'});
    expect(compareClientDiagnostics(physical,tunnel,now)).toBe('physical-only-signal');
    expect(compareClientDiagnostics(tunnel,physical,now)).toBe('physical-only-signal');
    expect(compareClientDiagnostics(physical,physical,now)).toBe('not-comparable');
    expect(compareClientDiagnostics(physical,{...tunnel,ip:'8.8.8.8'},now)).toBe('not-comparable');
    expect(compareClientDiagnostics(physical,{...tunnel,checkedAt:'2026-11-25T09:45:00Z'},now)).toBe('not-comparable');
    expect(compareClientDiagnostics(physical,tunnel,now+48*3600000)).toBe('not-comparable');
    expect(compareClientDiagnostics(physical,{...tunnel,evidence:'inconclusive'},now)).toBe('inconclusive');
  });
});
