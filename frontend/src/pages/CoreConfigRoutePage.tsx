import { useAllSettings } from '@/api/queries/useAllSettings';
import SingBoxRoutePage from './singbox/SingBoxRoutePage';
import XrayPage from './xray/XrayPage';
export default function CoreConfigRoutePage() {
  const { allSetting } = useAllSettings();
  return allSetting.coreType === 'xray' ? <XrayPage /> : <SingBoxRoutePage />;
}
