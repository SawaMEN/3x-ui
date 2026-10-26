import { lazy, Suspense } from 'react';
import { createBrowserRouter, Navigate, type RouteObject } from 'react-router';
import { Spin } from 'antd';

import PanelLayout from '@/layouts/PanelLayout';
import { importWithChunkRecovery } from '@/lib/chunk-load-recovery';

const IndexPage = lazy(() => importWithChunkRecovery(() => import('@/pages/index/IndexPage')));
const InboundsPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/inbounds/InboundsPage')),
);
const ClientsPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/clients/ClientsPage')),
);
const OlcRTCPage = lazy(() => importWithChunkRecovery(() => import('@/pages/olcrtc/OlcRTCPage')));
const GroupsPage = lazy(() => importWithChunkRecovery(() => import('@/pages/groups/GroupsPage')));
const NodesPage = lazy(() => importWithChunkRecovery(() => import('@/pages/nodes/NodesPage')));
const HostsPage = lazy(() => importWithChunkRecovery(() => import('@/pages/hosts/HostsPage')));
const FirewallPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/firewall/FirewallPage')),
);
const SettingsPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/settings/SettingsPage')),
);

const SingBoxPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/singbox/SingBoxRoutePage')),
);
const CoreConfigRoutePage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/CoreConfigRoutePage')),
);
const ApiDocsPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/api-docs/ApiDocsPage')),
);

function withSuspense(node: React.ReactNode) {
  return (
    <Suspense
      fallback={
        <div
          style={{
            minHeight: 240,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
          }}
        >
          <Spin size="large" />
        </div>
      }
    >
      {node}
    </Suspense>
  );
}

const routes: RouteObject[] = [
  {
    path: '/',
    element: <PanelLayout />,
    children: [
      { index: true, element: withSuspense(<IndexPage />) },
      { path: 'inbounds', element: withSuspense(<InboundsPage />) },
      { path: 'clients', element: withSuspense(<ClientsPage />) },
      { path: 'olcrtc', element: withSuspense(<OlcRTCPage />) },
      { path: 'groups', element: withSuspense(<GroupsPage />) },
      { path: 'nodes', element: withSuspense(<NodesPage />) },
      { path: 'hosts', element: withSuspense(<HostsPage />) },
      { path: 'firewall', element: withSuspense(<FirewallPage />) },
      { path: 'settings', element: withSuspense(<SettingsPage />) },
      { path: 'xray', element: <Navigate to="/singbox" replace /> },
      { path: 'singbox', element: withSuspense(<SingBoxPage />) },
      { path: 'outbound', element: withSuspense(<CoreConfigRoutePage />) },
      { path: 'routing', element: withSuspense(<CoreConfigRoutePage />) },
      { path: 'api-docs', element: withSuspense(<ApiDocsPage />) },
      // Backwards-compatible old bookmarks; Telemt now lives under Inbounds.
      { path: 'telemt', element: <Navigate to="/inbounds" replace /> },
    ],
  },
];

function computeBasename() {
  const raw = (typeof window !== 'undefined' && window.X_UI_BASE_PATH) || '/';
  const trimmed = raw.replace(/\/+$/, '');
  return `${trimmed}/panel`;
}

export const router = createBrowserRouter(routes, {
  basename: computeBasename(),
});
