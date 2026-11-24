import { describe, expect, it } from 'vitest';
import { auditAdaptiveGroups } from '@/pages/settings/adaptive-connect-audit';
import type { SubBalancer } from '@/schemas/subBalancer';

const group: SubBalancer = {
  id: 1,
  remark: 'test',
  enabled: true,
  sortOrder: 1,
  strategy: 'leastPing',
  inboundIds: [1, 2],
};

describe('adaptive connection readiness', () => {
  it('detects duplicate/disabled/unsupported members', () => {
    const [row] = auditAdaptiveGroups(
      [{ ...group, inboundIds: [1, 1, 2, 3] }],
      [
        { id: 1, enable: true, protocol: 'vless' },
        { id: 2, enable: false, protocol: 'vless' },
        { id: 3, enable: true, protocol: 'mtproto' },
      ],
    );
    expect(row.members).toBe(1);
    expect(row.issues).toContain('insufficient-members');
    expect(row.issues).toContain('unavailable-members');
  });
  it('does not equate multiple inbounds or nodes with independent public IPs', () => {
    const members = [
      { id: 1, enable: true, protocol: 'vless', network: 'tcp', nodeId: 1 },
      { id: 2, enable: true, protocol: 'vless', network: 'tcp', nodeId: 1 },
    ];
    expect(auditAdaptiveGroups([group], members)[0].issues).toContain('shared-node');
    expect(
      auditAdaptiveGroups([group], [members[0], { ...members[1], nodeId: 2 }])[0].issues,
    ).toContain('node-diversity-unverified');
  });
});
