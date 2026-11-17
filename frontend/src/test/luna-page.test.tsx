import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from './test-utils';

describe('Luna appearance', () => {
  it('ignores a saved light palette and has no palette toggle', async () => {
    localStorage.setItem('xui-theme', 'light');
    window.__SUB_PAGE_DATA__ = {
      enabled: true,
      sId: 'test',
      subUrl: 'https://vpn.example/sub/test',
      appearance: {
        enabled: true,
        theme: 'luna',
        title: 'VetrOFF',
        description: 'Private connection',
        logoURL: '',
        accent: '#ff0000',
      },
    };
    const { default: SubPage } = await import('@/pages/sub/SubPage');
    const page = renderWithProviders(<SubPage />);
    const root = page.container.querySelector('.subscription-page')!;
    expect(root.classList.contains('sub-luna')).toBe(true);
    expect(root.classList.contains('is-dark')).toBe(true);
    expect(root.querySelector('.luna-backdrop')?.getAttribute('aria-hidden')).toBe('true');
    expect(root.querySelectorAll('.luna-stars span').length).toBe(48);
    expect((root as HTMLElement).style.getPropertyValue('--sub-accent')).toBe('');
    expect(root.querySelector('.luna-moon')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Theme' })).toBeNull();
    page.unmount();
    const restored = renderWithProviders(<SubPage />);
    expect(
      restored.container.querySelector('.subscription-page')?.classList.contains('is-dark'),
    ).toBe(true);
    restored.unmount();
    localStorage.removeItem('xui-theme');
    delete window.__SUB_PAGE_DATA__;
  }, 20000);
});
