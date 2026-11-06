import { theme } from 'antd';
import type { ThemeConfig } from 'antd';

export function undergroundTheme(): ThemeConfig {
  return {
    algorithm: theme.darkAlgorithm,
    cssVar: { key: 'subscription-underground' },
    hashed: true,
    token: {
      colorBgBase: '#0a0e17',
      colorBorderSecondary: '#2a3449',
      colorPrimary: '#8ca8ff',
      colorPrimaryHover: '#b1c2ff',
      colorPrimaryActive: '#728fee',
      colorTextLightSolid: '#111827',
      colorLink: '#a8baff',
      colorLinkHover: '#cad5ff',
      colorText: '#f0f3fb',
      colorTextSecondary: '#a7b1c7',
      colorTextTertiary: '#8793ac',
      colorBgLayout: '#0c111d',
      colorBgContainer: '#151e2c',
      colorBgElevated: '#202b3d',
      colorBorder: '#354157',
      borderRadius: 12,
      borderRadiusLG: 20,
      controlHeight: 42,
      fontFamily: "Inter, system-ui, -apple-system, 'Segoe UI', sans-serif",
    },
    components: {
      Layout: { bodyBg: 'transparent' },
      Menu: { itemBg: 'transparent', subMenuItemBg: 'transparent' },
      Button: { primaryShadow: '0 8px 28px #4b6fe833' },
      Tabs: { inkBarColor: '#8ca8ff' },
    },
  };
}
