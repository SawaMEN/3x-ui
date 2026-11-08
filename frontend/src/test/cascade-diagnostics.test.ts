import { describe, expect, it } from 'vitest';
import {
  DIAGNOSTIC_CHECKLIST, reconcileProbeProblem, stages, verifiedForMerge,
  type CascadeCheck, type CascadeTarget,
} from '../pages/nodes/cascade-diagnostics';

const target: CascadeTarget = {
  tag: 'node-route-18', inboundId: 18, nodeId: 2, nodeName: 'Вторая нода',
  remark: 'Целевой VLESS', protocol: 'vless', port: 443, available: true,
};
const result: CascadeCheck = {
  tag: 'node-route-18', at: '2026-11-08T00:00:00Z', sourceInbound: 'local-vless',
  probe: { tag: 'node-route-18', reachable: true, checkedTarget: true,
    tunnelChecked: true, transport: 'tcp', detail: 'HTTP через туннель прошёл' },
  route: { matched: true, outboundTag: 'node-route-18' },
};

describe('cascade dashboard evidence and merge gates', () => {
  it('does not confuse TCP reachability with authenticated traffic', () => {
    const onlyTCP: CascadeCheck = {
      ...result, route: undefined,
      probe: { tag: target.tag, reachable: true, checkedTarget: true,
        transport: 'tcp', detail: 'TCP-порт отвечает' },
    };
    const evidence = stages(target, onlyTCP);
    expect(evidence[1].status).toBe('ok');
    expect(evidence[2].status).toBe('warning');
    expect(evidence[3].status).toBe('unknown');
    expect(evidence[4].status).toBe('unknown');
  });
  it('requires a matching route to selected managed outbound', () => {
    expect(stages(target, { ...result, route: { matched: true, outboundTag: 'direct' } })[3].status).toBe('failed');
    expect(stages(target, { ...result, sourceInbound: '' })[3].status).toBe('failed');
    expect(stages(target, result)[3].status).toBe('ok');
  });
  it('never marks client end-to-end stage successful from service outbound probe', () => {
    expect(stages(target, result)[4].status).toBe('unknown');
  });
  it('reports disabled node and failed authentication separately', () => {
    const blocked = { ...target, available: false, reason: 'Нода не в сети' };
    const failed: CascadeCheck = { ...result,
      probe: { ...result.probe!, reachable: false, detail: 'Аутентификация отклонена' },
    };
    expect(stages(blocked, failed)[0].status).toBe('failed');
    expect(stages(blocked, failed)[2].status).toBe('failed');
  });
  it('creates and updates a structured automatic issue only from measured probe result', () => {
    const failed: CascadeCheck = { ...result, probe: {
      ...result.probe!, reachable: false, detail: 'Ошибка подключения',
    } };
    const opened = reconcileProbeProblem([], target, failed);
    expect(opened).toHaveLength(1);
    expect(opened[0].status).toBe('open');
    expect(opened[0].evidence).toMatch(/Ошибка/);
    const resolved = reconcileProbeProblem(opened, target, result);
    expect(resolved[0].status).toBe('verified');
  });
  it('requires live client evidence, CI review, no unresolved issues and all checks for merge', () => {
    const checks = { [target.tag]: result };
    expect(DIAGNOSTIC_CHECKLIST).toHaveLength(8);
    expect(verifiedForMerge([target], checks, [], '', true)).toBe(false);
    expect(verifiedForMerge([target], checks, [], 'trace-2026-11-08', false)).toBe(false);
    expect(verifiedForMerge([target], checks, [
      { id: 'manual-1', tag: target.tag, nodeId: target.nodeId, protocol: 'vless',
        type: 'routing', title: 'route mismatch', status: 'fixed', updatedAt: result.at },
    ], 'trace-2026-11-08', true)).toBe(false);
    expect(verifiedForMerge([target], checks, [], 'trace-2026-11-08', true)).toBe(true);
    expect(verifiedForMerge([{ ...target, available: false }], checks, [], 'trace-2026-11-08', true)).toBe(false);
  });
});
