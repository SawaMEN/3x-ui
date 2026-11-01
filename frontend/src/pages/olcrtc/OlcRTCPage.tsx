import { Card, ConfigProvider, Layout } from 'antd';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import OlcRTCManagement from './OlcRTCManagement';
import '@/pages/singbox/SingBoxPage.css';

export default function OlcRTCPage() {
  const { antdThemeConfig, isDark, isUltra } = useTheme();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout
        className={`singbox-page ${isDark ? 'is-dark ' : ''}${isUltra ? 'is-ultra' : ''}`.trim()}
      >
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Card>
              <OlcRTCManagement />
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
