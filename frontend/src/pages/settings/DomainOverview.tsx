import { Button, Dropdown, Space, Tag, Typography } from 'antd';
import { PlusOutlined, MoreOutlined, GlobalOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainFrontStatus } from '@/generated/zod';
import './domain-workspace.css';

export type DomainPreset = 'subscription' | 'direct' | 'cdn' | 'sni' | 'shared' | 'certificate';
export default function DomainOverview({
  status,
  loading,
  busy,
  onAdd,
  onRefresh,
  onPreflight,
  onAudit,
  auditing,
  dirty,
}: {
  status?: DomainFrontStatus;
  loading: boolean;
  busy: boolean;
  dirty: boolean;
  auditing: boolean;
  onAdd: () => void;
  onRefresh: () => void;
  onPreflight: () => void;
  onAudit: () => void;
}) {
  const { t } = useTranslation();
  const l = (key: string) => t('pages.settings.domains.' + key);
  return (
    <header className="domain-hub-header">
      <div className="domain-hub-brand">
        <span className="domain-hub-logo">
          <GlobalOutlined />
        </span>
        <div>
          <Typography.Title level={3}>{l('title')}</Typography.Title>
          <Space size={8} wrap>
            <Tag color={status?.running ? 'success' : 'default'}>
              {l(
                status?.running
                  ? 'workspace.online'
                  : status?.config.enabled
                    ? 'workspace.offline'
                    : 'workspace.disabled',
              )}
            </Tag>
            <Typography.Text type="secondary">{l('redesign.subtitle')}</Typography.Text>
          </Space>
        </div>
      </div>
      <Space wrap>
        <Button
          type="primary"
          aria-label={l('add')}
          icon={<PlusOutlined />}
          disabled={loading || busy || !status}
          onClick={onAdd}
        >
          {l('add')}
        </Button>
        <Dropdown
          trigger={['click']}
          menu={{
            items: [
              { key: 'preflight', label: l('preflight.action'), disabled: !status || busy },
              {
                key: 'audit',
                label: l('workspace.audit'),
                disabled:
                  !status ||
                  dirty ||
                  busy ||
                  auditing ||
                  !status.config.domains.some((d) => d.enabled),
              },
              { key: 'refresh', label: l('refresh'), disabled: busy },
            ],
            onClick: ({ key }) => {
              if (key === 'preflight') onPreflight();
              if (key === 'audit') onAudit();
              if (key === 'refresh') onRefresh();
            },
          }}
        >
          <Button icon={<MoreOutlined />} aria-label={l('redesign.tools')} loading={auditing} />
        </Dropdown>
      </Space>
    </header>
  );
}
