import { describe, expect, it } from 'vitest';

import NodeList from '@/pages/nodes/NodeList';
import type { NodeRecord } from '@/schemas/node';

import { renderWithProviders } from './test-utils';

const noop = () => {};

function sampleNodes(): NodeRecord[] {
  return [
    { id: 1, name: 'parent', guid: 'p1', transitive: false, enable: true, status: 'online' },
    { id: 0, name: 'child-a', guid: 'ca', parentGuid: 'p1', transitive: true },
    { id: 0, name: 'child-b', guid: 'cb', parentGuid: 'p1', transitive: true },
  ];
}

describe('NodeList desktop table row keys', () => {
  it('gives transitive sub-node rows distinct keys instead of colliding on id 0', () => {
    const { container } = renderWithProviders(
      <NodeList
        nodes={sampleNodes()}
        isMobile={false}
        selectedIds={[]}
        onSelectionChange={noop}
        onAdd={noop}
        onMtls={noop}
        onEdit={noop}
        onDelete={noop}
        onProbe={noop}
        onToggleEnable={noop}
        onUpdateNode={noop}
        onUpdateSelected={noop}
        onReorder={async () => {}}
      />,
    );

    const rowKeys = Array.from(container.querySelectorAll('tr[data-row-key]'))
      .map((row) => row.getAttribute('data-row-key'))
      .filter((key): key is string => key !== null);

    expect(rowKeys).toEqual(expect.arrayContaining(['1', 't-ca', 't-cb']));
    expect(new Set(rowKeys).size).toBe(rowKeys.length);
  });
});
