import { Button, Card, Dropdown, Progress, Space, Tag, Tooltip, Typography } from 'antd';
import {
  DownOutlined,
  ScanOutlined,
  ReloadOutlined,
  CheckCircleOutlined,
  GlobalOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainFrontCheck, DomainFrontStatus } from '@/generated/zod';
import type { DomainConfigForm } from '@/schemas/domain-front';
import { getDomainSummary } from './domain-health';
import './domain-workspace.css';

export type DomainPreset = 'subscription' | 'direct' | 'cdn' | 'sni' | 'shared' | 'certificate';

type Props = {
  draft: DomainConfigForm;
  status?: DomainFrontStatus;
  checks: Record<string, DomainFrontCheck>;
  managedSniCount: number;
  dirty: boolean;
  loading: boolean;
  auditing: boolean;
  auditDone: number;
  auditTotal: number;
  onAudit: () => void;
  onAdd: (preset: DomainPreset) => void;
  busy: boolean;
  refreshing: boolean;
  onRefresh: () => void;
  onPreflight: () => void;
};

export default function DomainOverview({
  draft,
  status,
  checks,
  managedSniCount,
  dirty,
  loading,
  auditing,
  auditDone,
  auditTotal,
  onAudit,
  onAdd,
  busy,
  refreshing,
  onRefresh,
  onPreflight,
}: Props) {
  const { t } = useTranslation();
  const l = (key: string, values?: Record<string, string | number>) =>
    t('pages.settings.domains.workspace.' + key, values);
  const summary = getDomainSummary(draft, status, checks);
  const running = !!status?.running;
  const canAudit =
    !dirty && !busy && !loading && !auditing && !status?.error && !!status && summary.active > 0;

  return (
    <Card className="domain-workspace" loading={loading} size="small">
      <div className="domain-workspace-heading">
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            <GlobalOutlined className="domain-workspace-icon" /> {l('title')}
          </Typography.Title>
          <Typography.Text type="secondary">{l('subtitle')}</Typography.Text>
        </div>
        <Tag color={running ? 'success' : status?.config.enabled ? 'warning' : 'default'}>
          {running ? l('online') : status?.config.enabled ? l('offline') : l('disabled')}
        </Tag>
      </div>
      <div className="domain-workspace-summary">
        <Typography.Text>
          {l('active')}: <strong>{summary.active}</strong>
        </Typography.Text>
        <Typography.Text>
          {l('attention')}: <strong>{summary.issues}</strong>
        </Typography.Text>
        <Typography.Text type="secondary">
          {l('managedSni')}: {managedSniCount}
        </Typography.Text>
      </div>
      {(summary.failedCertificates > 0 || summary.expiringCertificates > 0 || !summary.total) && (
        <div className="domain-workspace-guidance">
          {!summary.total && <Typography.Text type="secondary">{l('emptyHint')}</Typography.Text>}
          {summary.failedCertificates > 0 && (
            <Typography.Text type="danger">
              {l('certError', { count: summary.failedCertificates })}
            </Typography.Text>
          )}
          {summary.expiringCertificates > 0 && (
            <Typography.Text type="warning">
              {l('certExpiring', { count: summary.expiringCertificates })}
            </Typography.Text>
          )}
        </div>
      )}
      <div className="domain-workspace-actions">
        <Space wrap>
          <Space.Compact>
            <Button
              type="primary"
              disabled={!status || loading || busy}
              onClick={() => onAdd('subscription')}
            >
              {t('pages.settings.domains.add')}
            </Button>
            <Dropdown
              trigger={['click']}
              menu={{
                items: (['direct', 'cdn', 'sni', 'shared', 'certificate'] as const).map(
                  (preset) => ({
                    key: preset,
                    label: l('preset.' + preset),
                    title: l('hint.' + preset),
                  }),
                ),
                onClick: ({ key }) => onAdd(key as DomainPreset),
              }}
            >
              <Button
                type="primary"
                icon={<DownOutlined />}
                aria-label={l('otherTypes')}
                disabled={!status || loading || busy}
              />
            </Dropdown>
          </Space.Compact>
          <Tooltip title={t('pages.settings.domains.refresh')}>
            <Button
              icon={<ReloadOutlined />}
              aria-label={t('pages.settings.domains.refresh')}
              loading={refreshing}
              disabled={busy}
              onClick={onRefresh}
            />
          </Tooltip>
          <Tooltip title={t('pages.settings.domains.preflight.action')}>
            <Button
              icon={<CheckCircleOutlined />}
              aria-label={t('pages.settings.domains.preflight.action')}
              disabled={!status || loading || busy}
              onClick={onPreflight}
            >
              {t('pages.settings.domains.preflight.action')}
            </Button>
          </Tooltip>
        </Space>
        <div className="domain-workspace-audit">
          <Tooltip title={dirty ? l('auditSaveFirst') : l('auditHint')}>
            <Button
              icon={<ScanOutlined />}
              loading={auditing}
              disabled={!canAudit && !auditing}
              onClick={onAudit}
            >
              {l('audit')}
            </Button>
          </Tooltip>
          {auditTotal > 0 && (
            <Progress
              size="small"
              percent={Math.round((auditDone / auditTotal) * 100)}
              status={auditing ? 'active' : 'normal'}
              format={() => auditDone + ' / ' + auditTotal}
            />
          )}
        </div>
      </div>
    </Card>
  );
}
