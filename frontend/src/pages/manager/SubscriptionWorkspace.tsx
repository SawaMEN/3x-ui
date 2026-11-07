import { Button, Card, Col, Dropdown, Empty, Input, Pagination, Progress, Row, Space, Tag, Typography } from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  MoreOutlined,
  PlusOutlined,
  QrcodeOutlined,
  RetweetOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { SizeFormatter } from '@/utils';
import type { ManagerSnapshot, Subscription } from './schema';
import {
  buildManagerSubscriptionUrl,
  getManagerClientState,
  managerDate,
} from './subscription-utils';

interface SubscriptionWorkspaceProps {
  data?: ManagerSnapshot;
  loading: boolean;
  pending: boolean;
  search: string;
  page: number;
  format: string;
  onSearch: (value: string) => void;
  onPage: (value: number) => void;
  onAdd: () => void;
  onRotateAll: () => void;
  onRotate: (id: number) => void;
  onEdit: (id: number) => void;
  onDelete: (id: number) => void;
  onQR: (url: string) => void;
  onManageClient: () => void;
}

export default function SubscriptionWorkspace({
  data,
  loading,
  pending,
  search,
  page,
  format,
  onSearch,
  onPage,
  onAdd,
  onRotateAll,
  onRotate,
  onEdit,
  onDelete,
  onQR,
  onManageClient,
}: SubscriptionWorkspaceProps) {
  const { t } = useTranslation();
  const m = (key: string) => t('pages.manager.' + key);
  const basePath = window.X_UI_BASE_PATH || '/';
  const origin = window.location.origin;
  const running = data?.activeOperations.filter(
    (op) => op.state === 'queued' || op.state === 'running',
  ) ?? [];

  const status = (sub: Subscription) => {
    if (!sub.enabled) return { color: 'default', text: m('disabled') };
    const client = data?.clients[String(sub.clientId)];
    if (sub.clientId > 0 && !client) {
      return { color: 'warning', text: m('clientUnavailable') };
    }
    const state = getManagerClientState(client);
    if (state === 'disabled') return { color: 'warning', text: m('clientDisabled') };
    if (state === 'expired') return { color: 'red', text: m('expired') };
    if (state === 'quota') return { color: 'red', text: m('trafficExhausted') };
    return { color: 'success', text: m('active') };
  };

  return (
    <div className="manager-workspace">
      <div className="manager-subscriptions-toolbar">
        <Input
          allowClear
          prefix={<SearchOutlined />}
          aria-label={m('search')}
          placeholder={m('search')}
          value={search}
          onChange={(event) => onSearch(event.target.value)}
          className="manager-search"
        />
        <Space wrap className="manager-subscription-actions">
          <Button disabled={pending} icon={<RetweetOutlined />} onClick={onRotateAll}>
            {m('rotateAll')}
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>
            {m('addSubscription')}
          </Button>
        </Space>
      </div>

      {!loading && data?.subscriptions.length === 0 && (
        <div className="manager-empty">
          <Empty description={m('empty')}>
            <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>
              {m('addSubscription')}
            </Button>
          </Empty>
        </div>
      )}

      <Row gutter={[16, 16]} align="stretch">
        {data?.subscriptions.map((sub) => {
          const client = data.clients[String(sub.clientId)];
          const url = buildManagerSubscriptionUrl(sub.token, format, basePath, origin);
          const job = running.find((op) => op.subscriptionId === sub.id);
          const state = status(sub);
          const used = client ? client.up + client.down : 0;
          const quotaPercent = client?.total
            ? Math.max(0, Math.min(100, Math.round((used / client.total) * 100)))
            : 0;
          return (
            <Col xs={24} lg={12} xxl={8} key={sub.id}>
              <Card
                size="small"
                className="manager-subscription-card"
                title={
                  <div className="manager-subscription-title">
                    <Typography.Text strong ellipsis={{ tooltip: sub.name }}>
                      {sub.name}
                    </Typography.Text>
                    <Typography.Text type="secondary" className="manager-subscription-id">
                      #{sub.id}
                    </Typography.Text>
                  </div>
                }
                extra={<Tag color={state.color}>{state.text}</Tag>}
              >
                <div className="manager-subscription-content">
                  <div className="manager-subscription-meta">
                    <Tag bordered={false}>{m('connections')}: {sub.entries.length}/20</Tag>
                    <Tag color={sub.autoRotate ? 'processing' : 'default'} bordered={false}>
                      {sub.autoRotate
                        ? m('autoRotate') + ' · ' + sub.intervalMinutes + ' ' + m('minutes')
                        : m('manual')}
                    </Tag>
                  </div>
                  {sub.note && (
                    <Typography.Paragraph
                      className="manager-subscription-note"
                      type="secondary"
                      ellipsis={{ rows: 2, expandable: true }}
                    >
                      {sub.note}
                    </Typography.Paragraph>
                  )}
                  <div className="manager-subscription-detail">
                    <Typography.Text type="secondary">{m('lastRotation')}</Typography.Text>
                    <Typography.Text>{managerDate(sub.lastRotation)}</Typography.Text>
                  </div>
                  {client && (
                    <div className="manager-client-summary">
                      <div className="manager-subscription-detail">
                        <Typography.Text strong ellipsis={{ tooltip: client.email }}>
                          {client.email}
                        </Typography.Text>
                        <Button type="link" size="small" onClick={onManageClient}>
                          {m('manageClient')}
                        </Button>
                      </div>
                      <div className="manager-subscription-detail">
                        <Typography.Text type="secondary">{m('traffic')}</Typography.Text>
                        <Typography.Text>
                          {SizeFormatter.sizeFormat(used)} / {client.total > 0
                            ? SizeFormatter.sizeFormat(client.total)
                            : '∞'}
                        </Typography.Text>
                      </div>
                      {client.total > 0 && (
                        <Progress
                          size="small"
                          percent={quotaPercent}
                          showInfo={false}
                          status={quotaPercent === 100 ? 'exception' : 'normal'}
                        />
                      )}
                      <div className="manager-subscription-detail">
                        <Typography.Text type="secondary">{m('expiry')}</Typography.Text>
                        <Typography.Text>
                          {client.expiryTime > 0 ? managerDate(client.expiryTime) : '∞'}
                        </Typography.Text>
                      </div>
                    </div>
                  )}
                  <div className="manager-subscription-link">
                    <Typography.Text type="secondary">{m('subscriptionUrl')}</Typography.Text>
                    <Typography.Paragraph copyable={{ text: url }} ellipsis={{ rows: 2, expandable: true }}>
                      {url}
                    </Typography.Paragraph>
                  </div>
                  {job && (
                    <div className="manager-job">
                      <Typography.Text type="secondary">
                        {m(job.state)} · {job.completed}/{job.total}
                      </Typography.Text>
                      <Progress
                        size="small"
                        percent={Math.max(0, Math.min(100, Math.round(
                          (100 * job.completed) / Math.max(1, job.total),
                        )))}
                        status="active"
                      />
                    </div>
                  )}
                </div>
                <div className="manager-subscription-footer">
                  <Space wrap size="small">
                    <Button
                      type="primary"
                      size="small"
                      icon={<EditOutlined />}
                      disabled={pending || !!job}
                      onClick={() => onEdit(sub.id)}
                    >
                      {t('edit')}
                    </Button>
                    <Button
                      size="small"
                      icon={<RetweetOutlined />}
                      disabled={pending || !!job || !sub.enabled}
                      onClick={() => onRotate(sub.id)}
                    >
                      {m('generate')}
                    </Button>
                    <Button
                      size="small"
                      icon={<QrcodeOutlined />}
                      aria-label="QR"
                      onClick={() => onQR(url)}
                    >
                      QR
                    </Button>
                  </Space>
                  <Dropdown
                    menu={{
                      items: [{
                        key: 'delete',
                        icon: <DeleteOutlined />,
                        label: t('delete'),
                        danger: true,
                        disabled: pending || !!job,
                        onClick: () => onDelete(sub.id),
                      }],
                    }}
                    trigger={['click']}
                  >
                    <Button
                      type="text"
                      icon={<MoreOutlined />}
                      aria-label={m('actions')}
                      title={m('actions')}
                    />
                  </Dropdown>
                </div>
              </Card>
            </Col>
          );
        })}
      </Row>
      {(data?.subscriptionTotal ?? 0) > (data?.pageSize ?? 25) && (
        <Pagination
          className="manager-subscriptions-pagination"
          current={page}
          pageSize={data?.pageSize ?? 25}
          total={data?.subscriptionTotal ?? 0}
          showSizeChanger={false}
          responsive
          onChange={onPage}
        />
      )}
    </div>
  );
}
