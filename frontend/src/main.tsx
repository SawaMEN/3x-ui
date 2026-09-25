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
import { router } from '@/routes';

const CHUNK_RELOAD_KEY = '3x-ui:chunk-reload';
const CHUNK_RELOAD_PARAM = '_3xui_chunk_reload';
const CHUNK_RELOAD_COOLDOWN = 10_000;

let chunkReloadScheduled = false;

function parseReloadTimestamp(value: string | null): number {
  const timestamp = Number(value ?? 0);
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function getStoredReloadTimestamp(): number {
  try {
    return parseReloadTimestamp(sessionStorage.getItem(CHUNK_RELOAD_KEY));
  } catch {
    return 0;
  }
}

function clearChunkReloadParam() {
  const url = new URL(window.location.href);
  const reloadTimestamp = parseReloadTimestamp(url.searchParams.get(CHUNK_RELOAD_PARAM));
  if (!reloadTimestamp) return;

  // Keep the cache-busting parameter in the URL when sessionStorage is not
  // available. It then doubles as the cross-navigation reload-loop guard.
  try {
    sessionStorage.setItem(CHUNK_RELOAD_KEY, String(reloadTimestamp));
    url.searchParams.delete(CHUNK_RELOAD_PARAM);
    window.history.replaceState(window.history.state, '', url.toString());
  } catch {
    // The URL timestamp remains available as a fallback loop guard.
  }
}

function reloadAfterChunkLoadError(): boolean {
  // Vite can emit vite:preloadError and then reject the same dynamic import.
  // Treat the second event as handled while the forced navigation is pending.
  if (chunkReloadScheduled) return true;

  const now = Date.now();
  const url = new URL(window.location.href);
  const urlReloadTimestamp = parseReloadTimestamp(url.searchParams.get(CHUNK_RELOAD_PARAM));
  const lastReload = Math.max(urlReloadTimestamp, getStoredReloadTimestamp());

  // If the freshly loaded page still cannot load its chunk, do not create an
  // automatic reload loop. Let the original error surface instead.
  if (now - lastReload < CHUNK_RELOAD_COOLDOWN) return false;

  chunkReloadScheduled = true;

  try {
    sessionStorage.setItem(CHUNK_RELOAD_KEY, String(now));
  } catch {
    // sessionStorage may be blocked; the URL parameter below is sufficient.
  }

  // A plain location.reload() can reuse stale HTML from an intermediary cache.
  // A unique query value forces a new document request while preserving the
  // current SPA route, existing query parameters, and hash.
  url.searchParams.set(CHUNK_RELOAD_PARAM, String(now));
  window.location.replace(url.toString());
  return true;
}

clearChunkReloadParam();

window.addEventListener('vite:preloadError', (event) => {
  if (reloadAfterChunkLoadError()) {
    event.preventDefault();
  }
});

window.addEventListener('unhandledrejection', (event) => {
  const message = event.reason instanceof Error ? event.reason.message : String(event.reason ?? '');

  if (
    /Failed to fetch dynamically imported module|Importing a module script failed|error loading dynamically imported module/i.test(
      message,
    ) &&
    reloadAfterChunkLoadError()
  ) {
    event.preventDefault();
  }
});

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
