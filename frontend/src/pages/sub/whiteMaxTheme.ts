import { buildAntdThemeConfig } from '@/hooks/useTheme';

// Values measured from https://whitemaxmod.com/style.css (25 October 2026).
// Fonts are bundled locally; this theme does not load assets from the reference site.
export function whiteMaxTheme() {
  const base = buildAntdThemeConfig('dark');
  return {
    ...base,
    token: {
      ...base.token,
      colorPrimary: '#ffffff',
      colorPrimaryHover: '#eeeeee',
      colorPrimaryActive: '#dddddd',
      colorTextLightSolid: '#1a1a1a',
      colorLink: '#cccccc',
      colorLinkHover: '#ffffff',
      colorText: '#ffffff',
      colorTextSecondary: '#aaaaaa',
      colorBgLayout: '#2a2a2e',
      colorBgContainer: '#333338',
      colorBgElevated: '#28282e',
      colorBorder: '#55555b',
      borderRadius: 16,
      fontFamily: '"Inter Variable", Inter, system-ui, -apple-system, sans-serif',
    },
  };
}
