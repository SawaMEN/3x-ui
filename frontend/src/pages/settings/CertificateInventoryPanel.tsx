import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Card, Empty, Space, Table, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import { CertificateInventorySchema } from '@/schemas/domain-certificates';

export default function CertificateInventoryPanel() {
  const { t, i18n } = useTranslation();
  const label = (key: string) => t('pages.settings.domains.certificateInventory.' + key);
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

  return (
    <Card
      size="small"
      title={label('title')}
      extra={
        <Button
          icon={<ReloadOutlined />}
          size="small"
          loading={certificates.isFetching}
          onClick={() => void certificates.refetch()}
        >
          {label('refresh')}
        </Button>
      }
    >
      <Typography.Paragraph type="secondary">{label('hint')}</Typography.Paragraph>
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
        scroll={{ x: 780 }}
        pagination={{ pageSize: 8, showSizeChanger: false }}
        locale={{ emptyText: <Empty description={label('empty')} /> }}
        dataSource={certificates.data?.items ?? []}
        columns={[
          {
            title: label('domain'),
            key: 'name',
            render: (_value, item) => (
              <Space orientation="vertical" size={2}>
                <Typography.Text strong>{item.name}</Typography.Text>
                {item.names.length > 0 && (
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
                <Typography.Text type="secondary">{item.usedBy.join(', ')}</Typography.Text>
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
                  {item.state}
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
                {item.ports.length > 0 ? (
                  <Typography.Text code>TCP {item.ports.join(', ')}</Typography.Text>
                ) : (
                  <Typography.Text type="secondary">{label('noPorts')}</Typography.Text>
                )}
                <Typography.Text type="secondary">
                  {item.autoRenew ? label('automatic') : label('externalControl')}
                </Typography.Text>
                {item.warning && <Typography.Text type="warning">{item.warning}</Typography.Text>}
              </Space>
            ),
          },
          {
            title: label('certificatePath'),
            key: 'path',
            render: (_value, item) =>
              item.certFile ? (
                <Typography.Text copyable style={{ overflowWrap: 'anywhere' }}>
                  {item.certFile}
                </Typography.Text>
              ) : (
                <Typography.Text type="secondary">{label('remotePath')}</Typography.Text>
              ),
          },
        ]}
      />
    </Card>
  );
}
