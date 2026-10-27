import { buildAntdThemeConfig } from '@/hooks/useTheme';
import type { ThemeMode } from '@/hooks/useTheme';

export function subscriptionThemeMode(theme?: string): ThemeMode {
  switch (theme) {
    case 'light':
    case 'dark':
    case 'ultra-dark':
    case 'colorful':
    case 'blue-gray':
    case 'cyberpunk':
      return theme;
    case 'luna':
    case 'whitemax':
      return 'dark';
    default:
      return 'cyberpunk';
  }
}

export function lunaTheme() {
  const base = buildAntdThemeConfig('dark');
  return {
    ...base,
    token: {
      ...base.token,
      colorPrimary: '#ead29b',
      colorPrimaryHover: '#f6e5bd',
      colorPrimaryActive: '#d5b978',
      colorTextLightSolid: '#171b26',
      colorLink: '#ead29b',
      colorLinkHover: '#f6e5bd',
      colorText: '#f1f3fa',
      colorTextSecondary: '#b4bdcf',
      colorTextTertiary: '#9ba8c2',
      colorBgLayout: '#090e1c',
      colorBgContainer: '#141d30',
      colorBgElevated: '#1c2940',
      colorBorder: '#3b4861',
      borderRadius: 14,
      fontFamily: '"Inter Variable", Inter, system-ui, -apple-system, sans-serif',
    },
  };
}
