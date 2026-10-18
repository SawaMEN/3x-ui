import { Card, ConfigProvider, Layout } from 'antd';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import OlcRTCSettingsTab from '@/pages/settings/OlcRTCSettingsTab';

export default function OlcRTCPage() {
  const { antdThemeConfig, isDark, isUltra } = useTheme();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={`singbox-page ${isDark ? 'is-dark ' : ''}${isUltra ? 'is-ultra' : ''}`.trim()}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Card>
              <OlcRTCSettingsTab />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
