import { useState } from 'react';
import { Alert, Button, Card, Select, Space, Typography, message } from 'antd';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import type { AllSetting } from '@/models/setting';
import type { DomainFrontStatus } from '@/generated/zod';
import { HttpUtil } from '@/utils';

export type PanelDomainPatch = Pick<AllSetting, 'webDomain' | 'webCertFile' | 'webKeyFile'>;
const CertificateSchema = z.object({
  domain: z.string(),
  certFile: z.string().min(1),
  keyFile: z.string().min(1),
  expiresAt: z.number(),
  nodeId: z.number().optional(),
});

export default function PanelDomainCertificate({
  status,
  allSetting,
  disabled,
  onSwitch,
  compact = false,
}: {
  compact?: boolean;
  status?: DomainFrontStatus;
  allSetting: AllSetting;
  disabled: boolean;
  onSwitch: (patch: PanelDomainPatch) => void;
}) {
  const { t } = useTranslation();
  const label = (key: string) => t('pages.settings.domains.panelDomain.' + key);
  const [selected, setSelected] = useState<string>();
  // Readiness is a view snapshot; the server rechecks the actual pair on click.
  const [openedAt] = useState(() => Date.now() / 1000);
  const [resolving, setResolving] = useState(false);
  const [notice, context] = message.useMessage();
  const choices =
    status?.config.domains.filter((domain) => domain.enabled && domain.mode === 'direct') ?? [];
  const cert = status?.certificates.find((item) => item.name === selected);
  const same =
    selected === allSetting.webDomain &&
    cert?.certFile === allSetting.webCertFile &&
    cert?.keyFile === allSetting.webKeyFile;
  const ready = (name: string) => {
    const domain = choices.find((item) => item.name === name);
    const pair = status?.certificates.find((item) => item.name === name);
    return (
      !!status?.running &&
      !!status.config.enabled &&
      !!pair?.certFile &&
      !!pair.keyFile &&
      pair.expiresAt > openedAt &&
      !(status.config.staging && (domain?.tls === 'auto' || domain?.tls === 'certmagic'))
    );
  };
  async function choose() {
    if (!selected || disabled || resolving) return;
    setResolving(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/panel-domain',
        { domain: selected },
        { silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || label('failed'));
      const pair = CertificateSchema.parse(response.obj);
      if (
        pair.domain !== selected ||
        (pair.nodeId ?? 0) !== 0 ||
        pair.expiresAt <= Date.now() / 1000
      )
        throw new Error(label('failed'));
      onSwitch({ webDomain: pair.domain, webCertFile: pair.certFile, webKeyFile: pair.keyFile });
    } catch (error) {
      notice.error(error instanceof Error ? error.message : label('failed'));
    } finally {
      setResolving(false);
    }
  }
  return (
    <Card
      size="small"
      title={compact ? undefined : label('title')}
      className={compact ? 'domain-panel-identity' : undefined}
    >
      {context}
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Typography.Text>
          {label('current')}:{' '}
          <Typography.Text strong>{allSetting.webDomain || label('byIP')}</Typography.Text>
        </Typography.Text>
        <Typography.Text type="secondary">{label('hint')}</Typography.Text>
        <Space.Compact style={{ width: '100%' }}>
          <Select
            style={{ flex: 1, minWidth: 0 }}
            aria-label={label('select')}
            placeholder={label('select')}
            value={selected}
            onChange={setSelected}
            disabled={disabled || resolving}
            options={choices.map((domain) => ({
              value: domain.name,
              label: domain.name + (ready(domain.name) ? '' : ' — ' + label('pending')),
              disabled: !ready(domain.name),
            }))}
          />
          <Button
            type="primary"
            onClick={() => void choose()}
            loading={resolving}
            disabled={disabled || !selected || !ready(selected) || same}
          >
            {label('switch')}
          </Button>
        </Space.Compact>
        {disabled && <Alert type="info" showIcon title={label('saveFirst')} />}
        {!choices.length && <Typography.Text type="secondary">{label('empty')}</Typography.Text>}
      </Space>
    </Card>
  );
}
