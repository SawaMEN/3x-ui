import { lazy, Suspense } from 'react';
import { Alert, Spin } from 'antd';

import { useAllSettings } from '@/api/queries/useAllSettings';
import { importWithChunkRecovery } from '@/lib/chunk-load-recovery';

const XrayPage = lazy(() => importWithChunkRecovery(() => import('@/pages/xray/XrayPage')));
const SingBoxPage = lazy(() =>
  importWithChunkRecovery(() => import('@/pages/singbox/SingBoxRoutePage')),
);

// Shared menu entries edit the selected core, including direct bookmarked URLs.
export default function CoreConfigRoutePage() {
  const { allSetting, fetched, fetchError } = useAllSettings();
  if (fetchError) return <Alert type="error" message={fetchError} />;
  if (!fetched) return <Spin />;
  const core: string = allSetting.coreType;
  if (!['xray', 'sing-box', 'hiddify-core'].includes(core))
    return <Alert type="error" message="Не удалось определить выбранное ядро." />;
  return (
    <Suspense fallback={<Spin />}>{core === 'xray' ? <XrayPage /> : <SingBoxPage />}</Suspense>
  );
}
