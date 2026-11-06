import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router';

import { keys } from '@/api/queryKeys';
import RoutingTab from '@/pages/xray/routing/RoutingTab';
import type { XraySettingsValue } from '@/hooks/useXraySetting';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

function renderRouting(availableInbounds: Array<{ id: number; tag: string; remark: string }>, tags: string[] = []) {
  const queryClient = makeTestQueryClient();
  queryClient.setQueryData(keys.inbounds.options(), availableInbounds);
  const settings = {
    inbounds: [],
    outbounds: [{ tag: 'direct', protocol: 'freedom' }],
    routing: { rules: [] },
  } as XraySettingsValue;
  renderWithProviders(
    <MemoryRouter>
      <RoutingTab
        templateSettings={settings}
        setTemplateSettings={vi.fn()}
        inboundTags={tags}
        clientReverseTags={[]}
        isMobile={false}
      />
    </MemoryRouter>,
    { queryClient },
  );
}

function openInboundPicker() {
  const picker = screen.getByRole('combobox', { name: 'Входящие подключения для маршрута' });
  const trigger = picker.closest('.ant-select')?.querySelector('.ant-select-selector');
  expect(trigger).not.toBeNull();
  fireEvent.mouseDown(trigger!);
}

describe('Routing inbound picker', () => {
  it('shows panel-managed inbounds missing from the Xray template and config payload', async () => {
    renderRouting([{ id: 7, tag: 'in-live', remark: 'My inbound' }]);
    openInboundPicker();
    expect(await screen.findByText('in-live (My inbound)')).not.toBeNull();
  });

  it('deduplicates tags from API options and the older config response', async () => {
    renderRouting([{ id: 7, tag: 'in-live', remark: 'My inbound' }], ['in-live']);
    openInboundPicker();
    expect(await screen.findAllByText('in-live (My inbound)')).toHaveLength(1);
  });

  it('uses the config tags as a fallback for older API responses', async () => {
    renderRouting([], ['in-from-config']);
    openInboundPicker();
    expect(await screen.findByText('in-from-config')).not.toBeNull();
  });
});
