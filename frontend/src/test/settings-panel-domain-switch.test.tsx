import { beforeEach, afterEach, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { AllSetting } from '@/models/setting';
import { HttpUtil, Msg } from '@/utils';
import { waitForPanelRestart } from '@/pages/settings/panel-url';
import SettingsPage from '@/pages/settings/SettingsPage';
import { renderWithProviders } from './test-utils';
import type { PanelDomainPatch } from '@/pages/settings/PanelDomainCertificate';

vi.mock('@/api/queries/useAllSettings', () => ({ useAllSettings: vi.fn() }));
vi.mock('@/layouts/AppSidebar', () => ({ default: () => null }));
vi.mock('@/pages/settings/AdditionalSettingsTab', () => ({
  default: ({
    onPanelDomainSwitch,
  }: {
    onPanelDomainSwitch: (patch: PanelDomainPatch) => void;
  }) => (
    <button
      onClick={() =>
        onPanelDomainSwitch({
          webDomain: 'direct.test',
          webCertFile: '/managed/direct.crt',
          webKeyFile: '/managed/direct.key',
        })
      }
    >
      Choose direct domain
    </button>
  ),
}));
vi.mock('@/pages/settings/panel-url', async (original) => ({
  ...(await original<typeof import('@/pages/settings/panel-url')>()),
  waitForPanelRestart: vi.fn().mockResolvedValue(false),
}));
const savePayload = vi.fn();
let events: string[];
beforeEach(() => {
  events = [];
  savePayload.mockReset();
  savePayload.mockImplementation(async () => {
    events.push('save');
    return new Msg(true, '', {});
  });
  vi.mocked(useAllSettings).mockReturnValue({
    allSetting: new AllSetting({
      webDomain: 'old.test',
      webPort: 2053,
      webBasePath: '/secret/',
      webCertFile: '/old.crt',
      webKeyFile: '/old.key',
    }),
    updateSetting: vi.fn(),
    fetched: true,
    fetchError: undefined,
    spinning: false,
    setSpinning: vi.fn(),
    saveDisabled: true,
    saveAll: vi.fn(),
    savePayload,
  });
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.post).mockImplementation(async () => {
    events.push('restart');
    return new Msg(true, '', {});
  });
  vi.mocked(waitForPanelRestart).mockClear();
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(new Response('', { headers: { 'X-Panel-Instance': 'old-instance' } })),
  );
});
afterEach(() => vi.unstubAllGlobals());
it('shows the new direct listener URL and saves the chosen identity before restarting', async () => {
  renderWithProviders(
    <MemoryRouter initialEntries={['/settings#advanced/domains']}>
      <SettingsPage />
    </MemoryRouter>,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Choose direct domain' }));
  const dialog = await screen.findByRole('dialog');
  const target = 'https://direct.test:2053/secret/panel/settings';
  expect(within(dialog).getByRole('link', { name: target }).getAttribute('href')).toBe(target);
  expect(savePayload).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Restart Panel' }));
  await waitFor(() =>
    expect(waitForPanelRestart).toHaveBeenCalledWith(target, window.location.href, 'old-instance'),
  );
  expect(savePayload).toHaveBeenCalledWith({
    webDomain: 'direct.test',
    webCertFile: '/managed/direct.crt',
    webKeyFile: '/managed/direct.key',
  });
  expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/setting/restartPanel');
  expect(events).toEqual(['save', 'restart']);
});
it('does not change settings when the switch confirmation is cancelled', async () => {
  renderWithProviders(
    <MemoryRouter initialEntries={['/settings#advanced/domains']}>
      <SettingsPage />
    </MemoryRouter>,
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Choose direct domain' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  expect(savePayload).not.toHaveBeenCalled();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});
