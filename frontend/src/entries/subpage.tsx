import { createRoot } from 'react-dom/client';
import { message } from 'antd';
import 'antd/dist/reset.css';

import { readyI18n } from '@/i18n/react';
import { AntdRootConfig } from '@/i18n/AntdRootConfig';
import { ThemeProvider } from '@/hooks/useTheme';
import { subscriptionThemeMode } from '@/pages/sub/lunaTheme';
import SubPage from '@/pages/sub/SubPage';

const messageContainer = document.getElementById('message');
if (messageContainer) {
  message.config({ getContainer: () => messageContainer });
}

readyI18n('subscription').then(() => {
  const root = document.getElementById('app');
  if (root) {
    createRoot(root).render(
      <AntdRootConfig>
        <ThemeProvider
          fixedMode={subscriptionThemeMode(window.__SUB_PAGE_DATA__?.appearance?.theme)}
        >
          <SubPage />
        </ThemeProvider>
      </AntdRootConfig>,
    );
  }
});
