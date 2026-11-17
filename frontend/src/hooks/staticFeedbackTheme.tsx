import { useSyncExternalStore, type ReactNode } from 'react';
import { ConfigProvider, type ThemeConfig } from 'antd';

import { AntdRootConfig } from '@/i18n/AntdRootConfig';

let currentTheme: ThemeConfig = {};
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function StaticFeedbackTheme({ children }: { children: ReactNode }) {
  const theme = useSyncExternalStore(subscribe, () => currentTheme);
  return (
    <AntdRootConfig>
      <ConfigProvider theme={theme}>{children}</ConfigProvider>
    </AntdRootConfig>
  );
}

// Static Modal/message/notification APIs mount outside the application's React
// tree. Subscribe their roots so even an already open dialog follows a theme
// change, including low-power mode, without losing its form state.
export function configureStaticFeedbackTheme(theme: ThemeConfig) {
  ConfigProvider.config({
    holderRender: (children) => <StaticFeedbackTheme>{children}</StaticFeedbackTheme>,
  });
  if (currentTheme === theme) return;
  currentTheme = theme;
  listeners.forEach((listener) => listener());
}
