import { Button, Space, Tag } from 'antd';
import { AppstoreOutlined, CloudDownloadOutlined, ThunderboltOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import './CoreSelection.css';

export type CoreKind = 'sing-box' | 'sing-box-upstream' | 'xray';
export interface CoreStatus {
  type: CoreKind;
  installed: boolean;
  running: boolean;
  version: string;
  selected: boolean;
}
export default function CoreSelection({
  cores,
  busy,
  onAction,
}: {
  cores: CoreStatus[];
  busy: CoreKind | null;
  onAction: (kind: CoreKind, action: 'install' | 'select') => void;
}) {
  const { t } = useTranslation();
  return (
    <section className="core-selection" aria-label={t('pages.settings.coreType')}>
      <div className="core-selection-list">
        {(['sing-box', 'sing-box-upstream', 'xray'] as const).map((kind) => {
          const core = cores.find((item) => item.type === kind);
          const bundled = kind === 'sing-box';
          const title = bundled
            ? t('pages.coreSelection.defaultTitle')
            : kind === 'xray'
              ? 'Xray'
              : 'sing-box';
          return (
            <article
              className={`core-selection-card${core?.selected ? ' core-selection-card-default' : ''}`}
              key={kind}
            >
              <span className="core-selection-icon">
                {bundled ? (
                  <AppstoreOutlined />
                ) : kind === 'xray' ? (
                  <ThunderboltOutlined />
                ) : (
                  <CloudDownloadOutlined />
                )}
              </span>
              <div className="core-selection-content">
                <div className="core-selection-name">
                  <strong>{title}</strong>
                  {core?.selected && <Tag color="blue">{t('pages.coreSelection.selected')}</Tag>}
                  {core?.version && <Tag>{core.version}</Tag>}
                  <Tag color={core?.running ? 'success' : 'default'}>
                    {!core
                      ? t('loading')
                      : !core.installed
                        ? t('pages.coreSelection.missing')
                        : t(core.running ? 'pages.singBox.running' : 'pages.singBox.stopped')}
                  </Tag>
                </div>
                <p>
                  {t(
                    bundled
                      ? 'pages.coreSelection.defaultDescription'
                      : kind === 'sing-box-upstream'
                        ? 'pages.coreSelection.upstreamDescription'
                        : 'pages.coreSelection.separate',
                  )}
                </p>
              </div>
              <Space className="core-selection-action" wrap>
                {!bundled && !core?.running && (
                  <Button
                    size="small"
                    disabled={!core || busy !== null}
                    loading={busy === kind}
                    icon={<CloudDownloadOutlined />}
                    onClick={() => onAction(kind, 'install')}
                  >
                    {t(
                      core?.installed
                        ? 'pages.coreSelection.update'
                        : 'pages.coreSelection.install',
                    )}
                  </Button>
                )}
                {core?.installed && !core.selected && (
                  <Button
                    size="small"
                    type="primary"
                    disabled={busy !== null}
                    loading={busy === kind}
                    onClick={() => onAction(kind, 'select')}
                  >
                    {t('pages.coreSelection.use')}
                  </Button>
                )}
              </Space>
            </article>
          );
        })}
      </div>
      <p className="core-selection-note">{t('pages.coreSelection.note')}</p>
    </section>
  );
}
