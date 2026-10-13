import { Button, Tag } from 'antd';
import {
  AppstoreOutlined,
  CheckCircleOutlined,
  CloudDownloadOutlined,
  ClockCircleOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import './CoreSelection.css';

export default function CoreSelection({
  installed,
  running,
}: {
  installed: boolean | null;
  running: boolean;
}) {
  const { t } = useTranslation();
  const status =
    installed === null
      ? t('loading')
      : !installed
        ? t('pages.coreSelection.missing')
        : t(running ? 'pages.singBox.running' : 'pages.singBox.stopped');
  return (
    <section className="core-selection" aria-label={t('pages.settings.coreType')}>
      <div className="core-selection-list">
        <article className="core-selection-card core-selection-card-default">
          <span className="core-selection-icon">
            <AppstoreOutlined />
          </span>
          <div className="core-selection-content">
            <div className="core-selection-name">
              <strong>{t('pages.coreSelection.defaultTitle')}</strong>
              <Tag color="blue">{t('pages.coreSelection.defaultBadge')}</Tag>
            </div>
            <p>{t('pages.coreSelection.defaultDescription')}</p>
          </div>
          <div className="core-selection-action" role="status">
            <Tag
              color={installed === null ? undefined : installed && running ? 'success' : 'warning'}
            >
              <CheckCircleOutlined /> {status}
            </Tag>
          </div>
        </article>
        {['sing-box', 'Xray'].map((name) => (
          <article className="core-selection-card" key={name}>
            <span className="core-selection-icon">
              {name === 'sing-box' ? <CloudDownloadOutlined /> : <ThunderboltOutlined />}
            </span>
            <div className="core-selection-content">
              <div className="core-selection-name">
                <strong dir="ltr">{name}</strong>
              </div>
              <p>{t('pages.coreSelection.separate')}</p>
            </div>
            <div className="core-selection-action">
              <Button disabled size="small" icon={<CloudDownloadOutlined />}>
                {t('pages.coreSelection.installSoon')}
              </Button>
            </div>
          </article>
        ))}
      </div>
      <p className="core-selection-note">
        <ClockCircleOutlined /> {t('pages.coreSelection.note')}
      </p>
    </section>
  );
}
