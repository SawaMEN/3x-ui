import { buildAntdThemeConfig } from '@/hooks/useTheme';

export function undergroundTheme() {
  const base = buildAntdThemeConfig('dark');
  return {
    ...base,
    token: {
      ...base.token,
      colorPrimary: '#e5ad78',
      colorPrimaryHover: '#f4c79e',
      colorPrimaryActive: '#ca8f59',
      colorTextLightSolid: '#171c20',
      colorLink: '#e5ad78',
      colorLinkHover: '#f4c79e',
      colorText: '#edf0ee',
      colorTextSecondary: '#b8c4c5',
      colorTextTertiary: '#a4b3b6',
      colorBgLayout: '#10191e',
      colorBgContainer: '#1b282e',
      colorBgElevated: '#25353b',
      colorBorder: '#47595e',
      borderRadius: 6,
      fontFamily: "'DM Sans Variable', system-ui, sans-serif",
    },
  };
}
