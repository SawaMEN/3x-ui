import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SubHeader from '@/pages/sub/SubHeader';
import { ThemeProvider, useTheme } from '@/hooks/useTheme';
import { renderWithProviders } from './test-utils';

function ThemeProbe() {
  const { isDark } = useTheme();
  return <output>{isDark ? 'dark' : 'light'}</output>;
}

describe('subscription theme selection', () => {
  it('keeps the language selector without offering a public theme switch', () => {
    renderWithProviders(
      <SubHeader title="Test" sId="id" email="" lang="en-US" onLangChange={vi.fn()} isDark />,
    );
    expect(screen.queryByRole('button', { name: 'Theme' })).toBeNull();
    expect(screen.getByRole('button', { name: 'Language' })).toBeTruthy();
  });

  it('uses the configured theme without overwriting the saved panel preference', () => {
    localStorage.setItem('xui-theme', 'cyberpunk');
    const page = render(
      <ThemeProvider fixedMode="light">
        <ThemeProbe />
      </ThemeProvider>,
    );
    expect(screen.getByText('light')).toBeTruthy();
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(localStorage.getItem('xui-theme')).toBe('cyberpunk');
    page.unmount();
  });
});
