import { useQuery } from '@tanstack/react-query';
import { Alert, Card, Empty, Table, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { HttpUtil } from '@/utils';

const auditEntry = z.object({
  at: z.number().int(),
  manager: z.string(),
  certificateId: z.string().optional(),
  domain: z.string().optional(),
  action: z.string(),
  outcome: z.string(),
  nodeId: z.number().int().positive().optional(),
});

export default function CertificateAuditPanel() {
  const { t, i18n } = useTranslation();
  const label = (key: string) => t('pages.settings.domains.certificateInventory.audit.' + key);
  const history = useQuery({
    queryKey: ['settings', 'domains', 'certificate-audit'],
    queryFn: async () => {
      const response = await HttpUtil.get('/panel/api/domains/certificates/events', undefined, {
        silent: true,
      });
      if (!response?.success)
        throw new Error(response?.msg || 'Unable to load certificate history');
      return z.array(auditEntry).parse(response.obj);
    },
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
    retry: false,
  });
  return (
    <Card size="small" title={label('title')} style={{ marginTop: 12 }}>
      <Typography.Paragraph type="secondary">{label('description')}</Typography.Paragraph>
      {history.error && (
        <Alert type="error" showIcon title={history.error.message} style={{ marginBottom: 12 }} />
      )}
      <Table
        rowKey="key"
        size="small"
        loading={history.isPending}
        dataSource={(history.data ?? []).map((entry, key) => ({ ...entry, key }))}
        pagination={{ pageSize: 8, showSizeChanger: false }}
        locale={{ emptyText: <Empty description={label('empty')} /> }}
        scroll={{ x: 650 }}
        columns={[
          {
            title: label('date'),
            dataIndex: 'at',
            render: (at: number) =>
              new Intl.DateTimeFormat(i18n.language, {
                dateStyle: 'short',
                timeStyle: 'medium',
              }).format(at * 1000),
          },
          {
            title: label('domain'),
            render: (_value, item) => (
              <span>
                {item.domain || label('unspecified')}
                {item.nodeId != null && <Tag style={{ marginInlineStart: 8 }}>Node #{item.nodeId}</Tag>}
              </span>
            ),
          },
          { title: label('manager'), dataIndex: 'manager' },
          {
            title: label('action'),
            dataIndex: 'action',
            render: (value: string) => label('actions.' + value),
          },
          {
            title: label('outcome'),
            dataIndex: 'outcome',
            render: (value: string) => (
              <Tag
                color={
                  value === 'failed' || value === 'expired'
                    ? 'error'
                    : value === 'expiring'
                      ? 'warning'
                      : value === 'succeeded' || value === 'recovered'
                        ? 'success'
                        : undefined
                }
              >
                {label('outcomes.' + value)}
              </Tag>
            ),
          },
        ]}
      />
    </Card>
  );
}
