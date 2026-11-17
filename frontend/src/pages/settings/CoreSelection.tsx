import { Button, Popconfirm, Space, Tag } from 'antd';
import {
  AppstoreOutlined,
  CloudDownloadOutlined,
  ThunderboltOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
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
  onAction: (kind: CoreKind, action: 'install' | 'select' | 'delete') => void;
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
                {!bundled && !core?.installed && (
                  <Button
                    size="small"
                    disabled={!core || busy !== null}
                    loading={busy === kind}
                    icon={<CloudDownloadOutlined />}
                    onClick={() => onAction(kind, 'install')}
                  >
                    {t('pages.coreSelection.install')}
                  </Button>
                )}
                {!bundled && core?.installed && (
                  <Popconfirm
                    title={t('pages.coreSelection.removeConfirm')}
                    description={t('pages.coreSelection.removeDescription')}
                    disabled={core.selected || core.running || busy !== null}
                    onConfirm={() => onAction(kind, 'delete')}
                  >
                    <Button
                      danger
                      size="small"
                      icon={<DeleteOutlined />}
                      title={
                        core.selected || core.running
                          ? t('pages.coreSelection.switchBeforeRemove')
                          : undefined
                      }
                      disabled={core.selected || core.running || busy !== null}
                      loading={busy === kind}
                    >
                      {t('pages.coreSelection.remove')}
                    </Button>
                  </Popconfirm>
                )}
                {core?.installed && (!core.selected || !core.running) && (
                  <Button
                    size="small"
                    type="primary"
                    disabled={busy !== null}
                    loading={busy === kind}
                    onClick={() => onAction(kind, 'select')}
                  >
                    {t(core.selected ? 'pages.coreSelection.start' : 'pages.coreSelection.use')}
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
