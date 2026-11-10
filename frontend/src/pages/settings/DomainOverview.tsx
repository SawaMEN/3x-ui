import { Button, Card, Progress, Space, Tag, Tooltip, Typography } from 'antd';
import {
  CloudServerOutlined,
  SafetyCertificateOutlined,
  GlobalOutlined,
  AlertOutlined,
  PlusOutlined,
  ScanOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainFrontCheck, DomainFrontStatus } from '@/generated/zod';
import type { DomainConfigForm } from '@/schemas/domain-front';
import { getDomainSummary } from './domain-health';
import './domain-workspace.css';

export type DomainPreset = 'subscription' | 'direct' | 'cdn' | 'sni' | 'shared';

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
}: Props) {
  const { t } = useTranslation();
  const l = (key: string, values?: Record<string, string | number>) =>
    t('pages.settings.domains.workspace.' + key, values);
  const summary = getDomainSummary(draft, status, checks);
  const running = !!status?.running;
  const canAudit =
    !dirty && !loading && !auditing && !status?.error && !!status && summary.active > 0;

  return (
    <Card className="domain-workspace" loading={loading} size="small">
      <div className="domain-workspace-heading">
        <div>
          <Typography.Title level={4} style={{ marginBottom: 4 }}>
            {l('title')}
          </Typography.Title>
          <Typography.Text type="secondary">{l('description')}</Typography.Text>
        </div>
        <Tag color={running ? 'success' : draft.enabled ? 'warning' : 'default'}>
          {running ? l('online') : draft.enabled ? l('offline') : l('disabled')}
        </Tag>
      </div>
      <div className="domain-workspace-stats">
        <div className="domain-workspace-stat">
          <GlobalOutlined aria-hidden />
          <Typography.Text type="secondary">{l('active')}</Typography.Text>
          <strong>
            {summary.active}
            <small> / {summary.total}</small>
          </strong>
        </div>
        <div className="domain-workspace-stat">
          <SafetyCertificateOutlined aria-hidden />
          <Typography.Text type="secondary">{l('healthy')}</Typography.Text>
          <strong>{summary.healthy}</strong>
        </div>
        <div className="domain-workspace-stat">
          <AlertOutlined aria-hidden />
          <Typography.Text type="secondary">{l('attention')}</Typography.Text>
          <strong>{summary.attention + summary.certificateAlerts}</strong>
        </div>
        <div className="domain-workspace-stat">
          <CloudServerOutlined aria-hidden />
          <Typography.Text type="secondary">{l('managedSni')}</Typography.Text>
          <strong>{managedSniCount}</strong>
        </div>
      </div>
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
        {draft.enabled &&
          draft.domains.some((domain) => domain.enabled && domain.tls === 'auto') &&
          (!draft.email || !draft.acceptTerms) && (
            <Typography.Text type="warning">{l('acmeHint')}</Typography.Text>
          )}
        {draft.enabled && !running && !dirty && !status?.error && (
          <Typography.Text type="warning">{l('gatewayHint')}</Typography.Text>
        )}
        {dirty && <Typography.Text type="secondary">{l('unsavedHint')}</Typography.Text>}
      </div>
      <div className="domain-workspace-actions">
        <div>
          <Typography.Text strong>{l('quickAdd')}</Typography.Text>
          <Space wrap size={8} style={{ display: 'flex', marginTop: 8 }}>
            {(['subscription', 'direct', 'cdn', 'sni', 'shared'] as const).map((preset) => (
              <Tooltip key={preset} title={l('hint.' + preset)}>
                <Button size="small" icon={<PlusOutlined />} onClick={() => onAdd(preset)}>
                  {l('preset.' + preset)}
                </Button>
              </Tooltip>
            ))}
          </Space>
        </div>
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
