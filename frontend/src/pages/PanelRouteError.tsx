import { Button, Result } from 'antd';
import { useRouteError } from 'react-router';
import { useTranslation } from 'react-i18next';

// A render/chunk failure must not strand the administrator on React Router's
// raw "Unexpected Application Error" screen while the VPN service is healthy.
export default function PanelRouteError() {
  const error = useRouteError();
  const { t } = useTranslation();
  const details = error instanceof Error ? error.message : '';

  const goHome = () => {
    const basePath = (window.X_UI_BASE_PATH || '/').replace(/\/?$/, '/');
    window.location.assign(basePath + 'panel/');
  };

  return (
    <main style={{ maxWidth: 780, margin: 'min(12vh, 100px) auto', padding: '0 24px' }}>
      <Result
        status="error"
        title={t('pages.panelRecovery.title')}
        subTitle={t('pages.panelRecovery.message')}
        extra={[
          <Button key="reload" type="primary" onClick={() => window.location.reload()}>
            {t('pages.panelRecovery.reload')}
          </Button>,
          <Button key="home" onClick={goHome}>
            {t('pages.panelRecovery.home')}
          </Button>,
        ]}
      />
      {details && (
        <details style={{ overflowWrap: 'anywhere', marginTop: 12 }}>
          <summary>{t('pages.panelRecovery.details')}</summary>
          <pre style={{ whiteSpace: 'pre-wrap', userSelect: 'text' }}>{details}</pre>
        </details>
      )}
    </main>
  );
}
