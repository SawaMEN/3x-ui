import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router/dom';
import { ConfigProvider, message } from 'antd';
import 'antd/dist/reset.css';
import '@/styles/utils.css';
import '@/styles/page-shell.css';
import '@/styles/page-cards.css';
import '@/styles/material-ui.css';
import '@/styles/sidebar-brand-alignment.css';

import { setupHttp } from '@/api/http-init';
import { readyI18n } from '@/i18n/react';
import { ThemeProvider, useTheme } from '@/hooks/useTheme';
import { QueryProvider } from '@/api/QueryProvider';
import { installChunkLoadRecovery } from '@/lib/chunk-load-recovery';
import { router } from '@/routes';

<<<<<<< HEAD
installChunkLoadRecovery();
=======
// A stale tab keeps this entry's old chunk URLs after a deploy. Reload once per
// panel base path and entry URL; the same bundle must not loop on a real outage.
const chunkRecoveryKey = `xui:chunk-recovery:${window.X_UI_BASE_PATH || '/'}:${import.meta.url}`;
let chunkRecoveryCommitted = false;

window.addEventListener('vite:preloadError', (event) => {
  if (chunkRecoveryCommitted) return;
  chunkRecoveryCommitted = true;
  let shouldReload = false;
  try {
    if (sessionStorage.getItem(chunkRecoveryKey) == null) {
      sessionStorage.setItem(chunkRecoveryKey, '1');
      shouldReload = true;
    }
  } catch {
    chunkRecoveryCommitted = false;
    return;
  }
  if (!shouldReload) {
    chunkRecoveryCommitted = false;
    return;
  }
  event.preventDefault();
  location.reload();
});

>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
setupHttp();

const messageContainer = document.getElementById('message');
if (messageContainer) {
  message.config({ getContainer: () => messageContainer });
}

function AppProviders() {
  const { antdThemeConfig } = useTheme();
  return (
    <ConfigProvider theme={antdThemeConfig}>
      <QueryProvider>
        <RouterProvider router={router} />
      </QueryProvider>
    </ConfigProvider>
  );
}

readyI18n().then(() => {
  const root = document.getElementById('app');
  if (root) {
    createRoot(root).render(
      <ThemeProvider>
        <AppProviders />
      </ThemeProvider>,
    );
  }
});
