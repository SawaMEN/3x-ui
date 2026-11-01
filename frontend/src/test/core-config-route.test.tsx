import { render, screen } from '@testing-library/react';
import { it, expect, vi } from 'vitest';
import CoreConfigRoutePage from '@/pages/CoreConfigRoutePage';
import { useAllSettings } from '@/api/queries/useAllSettings';
vi.mock('@/api/queries/useAllSettings', () => ({ useAllSettings: vi.fn() }));
vi.mock('@/pages/singbox/SingBoxRoutePage', () => ({ default: () => <div>sing-box editor</div> }));
vi.mock('@/pages/xray/XrayPage', () => ({ default: () => <div>Xray editor</div> }));
it('opens the selected engine editor', () => {
  vi.mocked(useAllSettings).mockReturnValue({ allSetting: { coreType: 'xray' } } as never);
  const view = render(<CoreConfigRoutePage />);
  expect(screen.getByText('Xray editor')).toBeTruthy();
  vi.mocked(useAllSettings).mockReturnValue({
    allSetting: { coreType: 'sing-box-upstream' },
  } as never);
  view.rerender(<CoreConfigRoutePage />);
  expect(screen.getByText('sing-box editor')).toBeTruthy();
});
