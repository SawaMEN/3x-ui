import { theme } from 'antd';
import type { ThemeConfig } from 'antd';

export function undergroundTheme(): ThemeConfig {
  return {
    algorithm: theme.darkAlgorithm,
    cssVar: { key: 'subscription-underground' },
    hashed: true,
    token: {
      colorBgBase: '#0c161b',
      colorBorderSecondary: '#34474e',
      colorPrimary: '#e9b17e',
      colorPrimaryHover: '#f4c79e',
      colorPrimaryActive: '#ca8f59',
      colorTextLightSolid: '#171c20',
      colorLink: '#e9b17e',
      colorLinkHover: '#f4c79e',
      colorText: '#edf0ee',
      colorTextSecondary: '#b8c4c5',
      colorTextTertiary: '#a4b3b6',
      colorBgLayout: '#10191e',
      colorBgContainer: '#1b282e',
      colorBgElevated: '#25353b',
      colorBorder: '#47595e',
      borderRadius: 10,
      borderRadiusLG: 18,
      controlHeight: 40,
      fontFamily: "system-ui, -apple-system, 'Segoe UI', sans-serif",
    },
    components: {
      Layout: { bodyBg: 'transparent' },
      Menu: { itemBg: 'transparent', subMenuItemBg: 'transparent' },
      Button: { primaryShadow: '0 4px 18px #e9b17e16' },
      Tabs: { inkBarColor: '#e9b17e' },
    },
  };
}
