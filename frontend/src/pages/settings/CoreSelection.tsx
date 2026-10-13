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
      <div className="core-selection-grid">
        <article className="core-selection-card core-selection-card-default">
          <div className="core-selection-card-top">
            <span className="core-selection-name">
              <AppstoreOutlined /> <strong>{t('pages.coreSelection.defaultTitle')}</strong>
            </span>
            <Tag color="blue">{t('pages.coreSelection.defaultBadge')}</Tag>
          </div>
          <p>{t('pages.coreSelection.defaultDescription')}</p>
          <div className="core-selection-status" role="status">
            <Tag
              color={installed === null ? undefined : installed && running ? 'success' : 'warning'}
            >
              <CheckCircleOutlined /> {status}
            </Tag>
          </div>
        </article>
        {['sing-box', 'Xray'].map((name) => (
          <article className="core-selection-card" key={name}>
            <div className="core-selection-card-top">
              <span className="core-selection-name">
                {name === 'sing-box' ? <CloudDownloadOutlined /> : <ThunderboltOutlined />}
                <strong dir="ltr">{name}</strong>
              </span>
              <Tag>{t('pages.coreSelection.soon')}</Tag>
            </div>
            <p>{t('pages.coreSelection.separate')}</p>
            <Button block disabled size="small" icon={<CloudDownloadOutlined />}>
              {t('pages.coreSelection.installSoon')}
            </Button>
          </article>
        ))}
      </div>
      <p className="core-selection-note">
        <ClockCircleOutlined /> {t('pages.coreSelection.note')}
      </p>
    </section>
  );
}
