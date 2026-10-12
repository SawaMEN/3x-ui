import { Button, Tag } from 'antd';
import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  ClockCircleOutlined,
  ThunderboltOutlined,
  AppstoreOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import './CoreSelection.css';

export default function CoreSelection() {
  const { t } = useTranslation();

  return (
    <section className="core-selection" aria-labelledby="core-selection-title">
      <header className="core-selection-heading">
        <div>
          <span className="core-selection-eyebrow">3x-ui</span>
          <h1 id="core-selection-title">{t('pages.coreSelection.title')}</h1>
          <p>{t('pages.coreSelection.subtitle')}</p>
        </div>
        <Tag icon={<ClockCircleOutlined />}>{t('pages.coreSelection.soon')}</Tag>
      </header>
      <div className="core-selection-grid">
        <article className="core-selection-card core-selection-card-default">
          <div className="core-selection-card-top">
            <span className="core-selection-icon">
              <AppstoreOutlined />
            </span>
            <Tag color="blue">{t('pages.coreSelection.defaultBadge')}</Tag>
          </div>
          <h2>{t('pages.coreSelection.defaultTitle')}</h2>
          <p>{t('pages.coreSelection.defaultDescription')}</p>
          <div className="core-selection-detail">
            <CheckCircleOutlined /> {t('pages.coreSelection.bundled')}
          </div>
          <Button block disabled icon={<CheckCircleOutlined />}>
            {t('pages.coreSelection.defaultBadge')}
          </Button>
        </article>
        {['sing-box', 'Xray'].map((name) => (
          <article className="core-selection-card" key={name}>
            <div className="core-selection-card-top">
              <span className="core-selection-icon">
                {name === 'sing-box' ? <CloudDownloadOutlined /> : <ThunderboltOutlined />}
              </span>
              <Tag>{t('pages.coreSelection.soon')}</Tag>
            </div>
            <h2 dir="ltr">{name}</h2>
            <p>{t('pages.coreSelection.externalDescription', { name })}</p>
            <div className="core-selection-detail">
              <CloudDownloadOutlined /> {t('pages.coreSelection.separate')}
            </div>
            <Button block disabled icon={<ClockCircleOutlined />}>
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
