import { render, screen } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import CoreConfigRoutePage from '@/pages/CoreConfigRoutePage';
const state = vi.hoisted(() => ({
  allSetting: { coreType: 'sing-box' },
  fetched: true,
  fetchError: undefined as string | undefined,
}));
vi.mock('@/api/queries/useAllSettings', () => ({ useAllSettings: () => state }));
vi.mock('@/pages/xray/XrayPage', () => ({ default: () => <div>Xray editor</div> }));
vi.mock('@/pages/singbox/SingBoxRoutePage', () => ({ default: () => <div>sing-box editor</div> }));
describe('shared core menu routes', () => {
  it.each(['sing-box', 'hiddify-core', 'xray'])('selects the %s editor', async (core) => {
    state.allSetting.coreType = core;
    state.fetchError = undefined;
    render(<CoreConfigRoutePage />);
    expect(
      await screen.findByText(core === 'xray' ? 'Xray editor' : 'sing-box editor'),
    ).toBeTruthy();
  });
  it('shows a settings failure instead of an endless spinner', () => {
    state.fetchError = 'Settings unavailable';
    render(<CoreConfigRoutePage />);
    expect(screen.getByText('Settings unavailable')).toBeTruthy();
  });
});
