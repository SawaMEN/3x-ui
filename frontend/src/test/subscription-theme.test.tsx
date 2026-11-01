import { render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ConfigProvider, Layout, theme } from 'antd';
import { undergroundTheme } from '@/pages/sub/undergroundTheme';
import SubHeader from '@/pages/sub/SubHeader';
import { ThemeProvider, useTheme } from '@/hooks/useTheme';
import { renderWithProviders } from './test-utils';

function ThemeProbe() {
  const { isDark } = useTheme();
  return <output>{isDark ? 'dark' : 'light'}</output>;
}

function ScopedThemeProbe() {
  const { antdThemeConfig } = useTheme();
  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout data-testid="panel-surface">
        <TokenProbe name="panel-primary" />
      </Layout>
      <ConfigProvider theme={undergroundTheme()}>
        <Layout data-testid="subscription-surface">
          <TokenProbe name="subscription-primary" />
        </Layout>
      </ConfigProvider>
    </ConfigProvider>
  );
}

function TokenProbe({ name }: { name: string }) {
  const { token } = theme.useToken();
  return <output data-testid={name}>{token.colorPrimary}</output>;
}

describe('subscription theme selection', () => {
  it('isolates Underground variables and tokens from the panel theme', () => {
    localStorage.setItem('xui-theme', 'light');
    const page = render(
      <ThemeProvider fixedMode="light">
        <ScopedThemeProbe />
      </ThemeProvider>,
    );
    expect(screen.getByTestId('panel-primary').textContent).toBe('#0071e3');
    const resolvedPrimary = screen.getByTestId('subscription-primary').textContent;
    expect(resolvedPrimary).toBe(theme.getDesignToken(undergroundTheme()).colorPrimary);
    expect(resolvedPrimary).not.toBe(screen.getByTestId('panel-primary').textContent);
    expect(screen.getByTestId('panel-surface').classList.contains('xui')).toBe(true);
    expect(screen.getByTestId('subscription-surface').className).toContain(
      'subscription-underground',
    );
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(localStorage.getItem('xui-theme')).toBe('light');
    page.unmount();
  });
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
