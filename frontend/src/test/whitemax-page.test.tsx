import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from './test-utils';
import { whiteMaxTheme } from '@/pages/sub/whiteMaxTheme';

describe('WhiteMAX reference appearance', () => {
  it('uses the reference palettes and keeps contrast for the primary button', () => {
    expect(whiteMaxTheme(true).token).toMatchObject({
      colorPrimary: '#ffffff',
      colorTextLightSolid: '#1a1a1a',
      colorBgLayout: '#2a2a2e',
    });
    expect(whiteMaxTheme(false).token).toMatchObject({
      colorPrimary: '#1a1a1a',
      colorTextLightSolid: '#ffffff',
      colorBgLayout: '#f2f2f5',
    });
  });

  it('starts in reference dark mode, toggles in place and remembers the palette', async () => {
    localStorage.removeItem('xui-whitemax-palette');
    window.__SUB_PAGE_DATA__ = {
      enabled: true,
      sId: 'test',
      subUrl: 'https://vpn.example/sub/test',
      appearance: {
        enabled: true,
        theme: 'whitemax',
        title: 'VetrOFF',
        description: 'Private connection',
        logoURL: '',
        accent: '#ff0000',
      },
    };
    const { default: SubPage } = await import('@/pages/sub/SubPage');
    const page = renderWithProviders(<SubPage />);
    const root = page.container.querySelector('.subscription-page')!;
    expect(root.classList.contains('sub-whitemax')).toBe(true);
    expect(root.classList.contains('is-dark')).toBe(true);
    expect(root.querySelector('.wm-backdrop')?.getAttribute('aria-hidden')).toBe('true');
    expect(root.querySelectorAll('.wm-particles span').length).toBe(56);
    expect((root as HTMLElement).style.getPropertyValue('--sub-accent')).toBe('');
    fireEvent.click(screen.getByRole('button', { name: 'Light / dark WhiteMAX' }));
    expect(root.classList.contains('sub-whitemax')).toBe(true);
    expect(root.classList.contains('is-dark')).toBe(false);
    expect(localStorage.getItem('xui-whitemax-palette')).toBe('light');
    page.unmount();
    const restored = renderWithProviders(<SubPage />);
    expect(
      restored.container.querySelector('.subscription-page')?.classList.contains('is-dark'),
    ).toBe(false);
    restored.unmount();
    localStorage.removeItem('xui-whitemax-palette');
    delete window.__SUB_PAGE_DATA__;
  }, 20000);
});
