import { fireEvent, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, expect, test, vi } from 'vitest';

import AppSidebar from '@/layouts/AppSidebar';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { renderWithProviders } from './test-utils';

vi.mock('@/api/queries/useAllSettings', () => ({
  useAllSettings: vi.fn(() => ({ allSetting: {} })),
}));

afterEach(() => {
  localStorage.clear();
});

function renderSidebar() {
  return renderWithProviders(
    <MemoryRouter>
      <AppSidebar />
    </MemoryRouter>,
  );
}

test('uses a single wordmark with full and compact labels', () => {
  const view = renderSidebar();
  const sidebarRoot = view.container.querySelector('.ant-sidebar');

  expect(view.container.querySelector('.brand-text-full')?.textContent).toBe('3X-UI');
  expect(view.container.querySelector('.brand-text-compact')?.textContent).toBe('3X');
  expect(view.container.querySelector('.sider-brand .brand-mark')).toBeNull();

  fireEvent.mouseEnter(sidebarRoot!);
  expect(view.container.querySelector('.sider-brand-content')).toHaveClass('brand-is-expanded');
  fireEvent.mouseLeave(sidebarRoot!);
  expect(view.container.querySelectorAll('.brand-text-full')).toHaveLength(1);
  expect(view.container.querySelectorAll('.brand-text-compact')).toHaveLength(1);
});

test('keeps the sidebar expanded after pinning it from the header and restores the choice', () => {
  const first = renderSidebar();
  const sidebar = first.container.querySelector('.ant-layout-sider');
  const sidebarRoot = first.container.querySelector('.ant-sidebar');

  expect(sidebar?.classList.contains('ant-layout-sider-collapsed')).toBe(true);

  fireEvent.mouseEnter(sidebarRoot!);

  const pinButton = screen.getByRole('button', { name: 'Pin sidebar' });
  expect(pinButton.closest('.brand-actions')).not.toBeNull();

  fireEvent.click(pinButton);
  fireEvent.mouseLeave(sidebarRoot!);

  expect(sidebar?.classList.contains('ant-layout-sider-collapsed')).toBe(false);
  expect(sidebarRoot?.getAttribute('style')).toContain('--sider-rail: 220px');
  expect(localStorage.getItem('sidebar-pinned')).toBe('true');

  first.unmount();

  const second = renderSidebar();
  const restoredSidebar = second.container.querySelector('.ant-layout-sider');
  const restoredSidebarRoot = second.container.querySelector('.ant-sidebar');

  expect(restoredSidebar?.classList.contains('ant-layout-sider-collapsed')).toBe(false);
  expect(restoredSidebarRoot?.getAttribute('style')).toContain('--sider-rail: 220px');
  expect(screen.getByRole('button', { name: 'Pin sidebar' })).not.toBeNull();
});

test('returns to the compact rail after unpinning', () => {
  const view = renderSidebar();
  const sidebar = view.container.querySelector('.ant-layout-sider');
  const sidebarRoot = view.container.querySelector('.ant-sidebar');

  fireEvent.mouseEnter(sidebarRoot!);
  fireEvent.click(screen.getByRole('button', { name: 'Pin sidebar' }));
  fireEvent.click(screen.getByRole('button', { name: 'Pin sidebar' }));
  fireEvent.mouseLeave(sidebarRoot!);

  expect(sidebar?.classList.contains('ant-layout-sider-collapsed')).toBe(true);
  expect(sidebarRoot?.getAttribute('style')).toContain('--sider-rail: 72px');
  expect(localStorage.getItem('sidebar-pinned')).toBe('false');
});

test('keeps core and swap controls in general settings, not the sidebar submenu', () => {
  vi.mocked(useAllSettings).mockReturnValue({ allSetting: { coreType: 'xray' } } as never);
  const view = renderWithProviders(
    <MemoryRouter initialEntries={['/settings#general']}>
      <AppSidebar />
    </MemoryRouter>,
  );

  const sidebarRoot = view.container.querySelector('.ant-sidebar');
  fireEvent.mouseEnter(sidebarRoot!);
  expect(screen.getByText('General')).toBeTruthy();
  expect(screen.queryByText('Proxy Core')).toBeNull();
  expect(screen.queryByText('Swap / ZRAM')).toBeNull();
  expect(screen.queryByText('API Docs')).toBeNull();
  expect(screen.getByText('Xray Configs')).toBeTruthy();
});

test('shows only the active core configuration menu', () => {
  vi.mocked(useAllSettings).mockReturnValue({ allSetting: { coreType: 'xray' } } as never);
  const xrayView = renderWithProviders(
    <MemoryRouter initialEntries={['/xray#basic']}>
      <AppSidebar />
    </MemoryRouter>,
  );

  const xrayRoot = xrayView.container.querySelector('.ant-sidebar');
  fireEvent.mouseEnter(xrayRoot!);
  expect(screen.getByText('Xray Configs')).toBeTruthy();
  expect(screen.getByText('Basics')).toBeTruthy();
  expect(screen.queryByText('sing-box Configs')).toBeNull();
  expect(screen.queryByText('Proxy Core')).toBeNull();
  expect(screen.queryByText('Swap / ZRAM')).toBeNull();
  expect(screen.queryByText('API Docs')).toBeNull();
  xrayView.unmount();

  vi.mocked(useAllSettings).mockReturnValue({ allSetting: { coreType: 'sing-box' } } as never);
  const singBoxView = renderWithProviders(
    <MemoryRouter initialEntries={['/singbox#basic']}>
      <AppSidebar />
    </MemoryRouter>,
  );

  const singBoxRoot = singBoxView.container.querySelector('.ant-sidebar');
  fireEvent.mouseEnter(singBoxRoot!);
  expect(screen.getByText('sing-box Configs')).toBeTruthy();
  expect(screen.queryByText('Xray Configs')).toBeNull();
  expect(screen.queryByText('Proxy Core')).toBeNull();
  expect(screen.queryByText('Swap / ZRAM')).toBeNull();
  expect(screen.queryByText('API Docs')).toBeNull();
});
