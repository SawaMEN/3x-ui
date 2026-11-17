import { useCallback, useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import LocalCertificateAssistant from '@/pages/inbounds/form/security/LocalCertificateAssistant';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Empty,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import { CertificateInventorySchema } from '@/schemas/domain-certificates';
import CertificateAuditPanel from './CertificateAuditPanel';

export default function CertificateInventoryPanel({
  compact = false,
  onShare,
}: { compact?: boolean; onShare?: () => void } = {}) {
  const { t, i18n } = useTranslation();
  const label = (key: string) => t('pages.settings.domains.certificateInventory.' + key);
  const [renewing, setRenewing] = useState<string>();
  const [selected, setSelected] = useState<{
    domain: string;
    nodeId: number;
    coverSite: boolean;
    nodeName?: string;
  }>();
  const ru = i18n.language.startsWith('ru');

  const [notice, contextHolder] = message.useMessage();
  const certificates = useQuery({
    queryKey: ['settings', 'domains', 'certificate-inventory'],
    queryFn: async () => {
      const response = await HttpUtil.get('/panel/api/domains/certificates', undefined, {
        silent: true,
      });
      if (!response?.success) throw new Error(response?.msg || 'Unable to load certificates');
      return CertificateInventorySchema.parse(response.obj);
    },
    retry: false,
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
  });

  const refetchInventory = certificates.refetch;
  const onReady = useCallback(() => {
    setSelected(undefined);
    void refetchInventory();
  }, [refetchInventory]);

  async function renew(id: string) {
    setRenewing(id);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/renew',
        { id },
        { silent: true, timeout: 780000 },
      );
      if (!response?.success) throw new Error(response?.msg || label('renewFailed'));
      notice.success(label('renewCompleted'));
      void certificates.refetch();
    } catch (error) {
      notice.error(error instanceof Error ? error.message : label('renewFailed'));
    } finally {
      setRenewing(undefined);
    }
  }

  return (
    <>
      {contextHolder}
      <Card
        size="small"
        className={compact ? 'domain-inventory' : undefined}
        title={compact ? undefined : label('title')}
        extra={
          !compact && (
            <Button
              icon={<ReloadOutlined />}
              size="small"
              loading={certificates.isFetching}
              onClick={() => void certificates.refetch()}
            >
              {label('refresh')}
            </Button>
          )
        }
      >
        {compact && (
          <div className="domain-inventory-toolbar">
            <Typography.Text type="secondary">{label('hint')}</Typography.Text>
            <Space wrap>
              <Button onClick={onShare}>
                {t('pages.settings.domains.workspace.takeNodeCertificate')}
              </Button>
              <Button
                icon={<ReloadOutlined />}
                aria-label={label('refresh')}
                loading={certificates.isFetching}
                onClick={() => void certificates.refetch()}
              />
            </Space>
          </div>
        )}
        {!compact && <Typography.Paragraph type="secondary">{label('hint')}</Typography.Paragraph>}
        {certificates.error && (
          <Alert
            type="error"
            showIcon
            title={certificates.error.message}
            style={{ marginBottom: 12 }}
          />
        )}
        {certificates.data?.firewallWarning && (
          <Alert
            type="warning"
            showIcon
            title={label('firewallWarning')}
            description={certificates.data.firewallWarning}
            style={{ marginBottom: 12 }}
          />
        )}
        <Table
          rowKey="id"
          size="small"
          loading={certificates.isPending}
          scroll={{ x: 600 }}
          pagination={{ pageSize: 8, showSizeChanger: false, hideOnSinglePage: true }}
          expandable={{
            expandedRowRender: (item) => (
              <Space orientation="vertical" size={6}>
                <Typography.Text strong>{label('certificatePath')}</Typography.Text>
                <Typography.Text copyable={!!item.certFile} style={{ overflowWrap: 'anywhere' }}>
                  {item.certFile || label('remotePath')}
                </Typography.Text>
                <Typography.Text type="secondary">{item.usedBy.join(', ')}</Typography.Text>
                {item.ports.length > 0 && (
                  <Space>
                    <Typography.Text code>TCP {item.ports.join(', ')}</Typography.Text>
                    <Typography.Text type="secondary">{label('temporaryPorts')}</Typography.Text>
                  </Space>
                )}
                {item.warning && <Typography.Text type="warning">{item.warning}</Typography.Text>}
              </Space>
            ),
          }}
          locale={{ emptyText: <Empty description={label('empty')} /> }}
          dataSource={certificates.data?.items ?? []}
          columns={[
            {
              title: label('domain'),
              key: 'name',
              render: (_value, item) => (
                <Space orientation="vertical" size={2}>
                  <Typography.Text strong>{item.name}</Typography.Text>
                  {item.names.some((name) => name !== item.name) && (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      SAN: {item.names.join(', ')}
                    </Typography.Text>
                  )}
                  {item.nodeName && (
                    <Tag>
                      {label('node')}: {item.nodeName}
                    </Tag>
                  )}
                </Space>
              ),
            },
            {
              title: label('manager'),
              key: 'manager',
              render: (_value, item) => (
                <Space orientation="vertical" size={2}>
                  <Tag>{label('source.' + item.manager)}</Tag>
                </Space>
              ),
            },
            {
              title: label('validity'),
              key: 'validity',
              render: (_value, item) => (
                <Space orientation="vertical" size={2}>
                  <Tag
                    color={
                      item.state === 'valid'
                        ? 'success'
                        : item.state === 'expired' || item.state === 'error'
                          ? 'error'
                          : 'warning'
                    }
                  >
                    {t('pages.settings.domains.state.' + item.state, { defaultValue: item.state })}
                  </Tag>
                  {item.expiresAt > 0 && (
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium' }).format(
                        item.expiresAt * 1000,
                      )}
                      {' · '}
                      {item.daysLeft} {label('days')}
                    </Typography.Text>
                  )}
                  {item.issuer && <Typography.Text type="secondary">{item.issuer}</Typography.Text>}
                </Space>
              ),
            },
            {
              title: label('renewal'),
              key: 'renewal',
              render: (_value, item) => (
                <Space orientation="vertical" size={2}>
                  <Tag>{label('challenge.' + item.challenge)}</Tag>
                  <Typography.Text type="secondary">
                    {item.manager === 'certbot' || item.manager === 'acme.sh'
                      ? label('panelSchedule')
                      : item.autoRenew
                        ? label('automatic')
                        : label('externalControl')}
                  </Typography.Text>
                </Space>
              ),
            },
            {
              title: label('actions'),
              key: 'actions',
              render: (_value, item) =>
                item.canObtain && !['valid', 'renewal_due', 'disabled'].includes(item.state) ? (
                  <Button
                    size="small"
                    type="primary"
                    onClick={() =>
                      setSelected({
                        domain: item.name,
                        nodeId: item.nodeId || 0,
                        coverSite: item.coverSite === true,
                        nodeName: item.nodeName,
                      })
                    }
                  >
                    {ru ? 'Получить сертификат' : 'Obtain certificate'}
                  </Button>
                ) : !item.nodeId &&
                  (item.manager === 'acme.sh' ||
                    item.manager === 'certbot' ||
                    (item.manager === 'certmagic' &&
                      item.autoRenew &&
                      item.expiresAt > 0 &&
                      item.daysLeft <= 30)) ? (
                  <Popconfirm
                    title={label('renewConfirm')}
                    description={label('renewDescription')}
                    onConfirm={() => renew(item.id)}
                    okText={label('renewAction')}
                  >
                    <Button size="small" loading={renewing === item.id} disabled={!!renewing}>
                      {label('renewAction')}
                    </Button>
                  </Popconfirm>
                ) : null,
            },
          ]}
        />
      </Card>
      {selected && (
        <Modal
          open
          title={ru ? 'Получение сертификата' : 'Obtain certificate'}
          footer={null}
          onCancel={() => setSelected(undefined)}
          destroyOnHidden
        >
          <Typography.Paragraph>
            <strong>{selected.domain}</strong> ·{' '}
            {selected.nodeName || (ru ? 'Основной сервер' : 'Main server')}
          </Typography.Paragraph>
          <InventoryCertificatePreparation
            key={`${selected.nodeId}:${selected.domain}`}
            {...selected}
            onReady={onReady}
          />
        </Modal>
      )}
      <Collapse
        className="domain-inventory-history"
        items={[
          {
            key: 'history',
            label: t('pages.settings.domains.certificateInventory.audit.title'),
            children: <CertificateAuditPanel />,
          },
        ]}
      />
    </>
  );
}

function InventoryCertificatePreparation({
  domain,
  nodeId,
  coverSite,
  onReady,
}: {
  domain: string;
  nodeId: number;
  coverSite: boolean;
  onReady: () => void;
}) {
  const form = useForm({
    defaultValues: { protocol: 'trojan', nodeId, streamSettings: { security: 'tls' } },
  });
  return (
    <FormProvider {...form}>
      <LocalCertificateAssistant sni={domain} coverSite={coverSite} autoStart onReady={onReady} />
    </FormProvider>
  );
}
