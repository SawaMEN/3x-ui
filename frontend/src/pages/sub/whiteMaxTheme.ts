import { buildAntdThemeConfig } from '@/hooks/useTheme';

// Values measured from https://whitemaxmod.com/style.css (25 October 2026).
// Fonts are bundled locally; this theme does not load assets from the reference site.
export function whiteMaxTheme(dark: boolean) {
  const base = buildAntdThemeConfig(dark ? 'dark' : 'light');
  return {
    ...base,
    token: {
      ...base.token,
      colorPrimary: dark ? '#ffffff' : '#1a1a1a',
      colorPrimaryHover: dark ? '#eeeeee' : '#333333',
      colorPrimaryActive: dark ? '#dddddd' : '#111111',
      colorTextLightSolid: dark ? '#1a1a1a' : '#ffffff',
      colorLink: dark ? '#cccccc' : '#555555',
      colorLinkHover: dark ? '#ffffff' : '#1a1a1a',
      colorText: dark ? '#ffffff' : '#1a1a1a',
      colorTextSecondary: dark ? '#aaaaaa' : '#555555',
      colorBgLayout: dark ? '#2a2a2e' : '#f2f2f5',
      colorBgContainer: dark ? '#333338' : '#e2e2e8',
      colorBgElevated: dark ? '#28282e' : '#f0f0f3',
      colorBorder: dark ? '#55555b' : '#bdbdc5',
      borderRadius: 16,
      fontFamily: '"Inter Variable", Inter, system-ui, -apple-system, sans-serif',
    },
  };
}

export function readWhiteMaxDark(): boolean {
  try {
    return localStorage.getItem('xui-whitemax-palette') !== 'light';
  } catch {
    return true;
  }
}
