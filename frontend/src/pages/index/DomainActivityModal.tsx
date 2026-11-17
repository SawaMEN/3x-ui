import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Select, Space, Switch, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';

import { useRefreshRequest } from '@/hooks/useRefreshRequest';
import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';

interface DomainActivityEntry {
  email: string;
  domain: string;
  lastSeen: string;
  count: number;
}

interface DomainActivityState {
  enabled: boolean;
  supported: boolean;
  accessLogEnabled: boolean;
  core: string;
  mode: 'active' | 'recent';
  error?: string;
  windowSeconds: number;
  entries: DomainActivityEntry[];
  nodeId?: number; // client-side request identity; never inferred from node metadata
}

interface DomainActivityModalProps {
  open: boolean;
  onClose: () => void;
  initialClient?: string;
}

const EMPTY: DomainActivityState = {
  enabled: false,
  supported: true,
  accessLogEnabled: false,
  core: '',
  mode: 'recent',
  windowSeconds: 120,
  entries: [],
  nodeId: 0,
};

export default function DomainActivityModal({
  open,
  onClose,
  initialClient = '',
}: DomainActivityModalProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const { nodes } = useNodesQuery();
  const [nodeId, setNodeId] = useState(0);
  const nodeOptions = useMemo(
    () => [
      { value: 0, label: t('pages.index.domainActivityLocalNode') },
      ...nodes
        .filter((node) => node.enable && !node.transitive)
        .map((node) => ({ value: node.id, label: node.name || node.address || String(node.id) })),
    ],
    [nodes, t],
  );
  const [clientFilter, setClientFilter] = useState(initialClient);
  const [domainFilter, setDomainFilter] = useState('');
  const [updating, setUpdating] = useState(false);

  const load = useCallback(
    async (signal: AbortSignal): Promise<DomainActivityState | undefined> => {
      try {
        const url = '/panel/api/server/domainActivity' + (nodeId === 0 ? '' : '?nodeId=' + nodeId);
        const msg = await HttpUtil.get<DomainActivityState>(url, undefined, {
          signal,
          silent: true,
        });
        if (signal.aborted) return undefined;
        if (msg.success && msg.obj && Array.isArray(msg.obj.entries)) {
          return { ...msg.obj, nodeId };
        }
        return { ...EMPTY, nodeId, error: msg.msg || 'Request failed' };
      } catch (error) {
        if (signal.aborted) return undefined;
        return {
          ...EMPTY,
          nodeId,
          error: error instanceof Error ? error.message : 'Request failed',
        };
      }
    },
    [nodeId],
  );

  // The backend returns only a short rolling window; polling stops when closed
  // or when the document is hidden. No client-side activity history is retained.
  const { data, loading, refresh } = useRefreshRequest(open, load, EMPTY, 3000);
  // A source change must never display an old node's activity as a live result.
  const viewData = data.nodeId === nodeId ? data : EMPTY;

  const toggle = async (enabled: boolean) => {
    if (nodeId !== 0) return;
    setUpdating(true);
    try {
      const msg = await HttpUtil.post(
        '/panel/api/server/domainActivity/config',
        { enabled },
        { headers: { 'Content-Type': 'application/json' } },
      );
      if (msg?.success) refresh();
    } finally {
      setUpdating(false);
    }
  };

  const entries = useMemo(() => {
    const client = clientFilter.trim().toLowerCase();
    const domain = domainFilter.trim().toLowerCase();
    // The backend already applies the Xray time window. Rechecking against
    // the browser clock hides valid rows when the user's clock is inaccurate.
    return viewData.entries.filter(
      (entry) =>
        (entry.email ?? '').toLowerCase().includes(client) &&
        entry.domain.toLowerCase().includes(domain),
    );
  }, [viewData.entries, clientFilter, domainFilter]);

  const columns: ColumnsType<DomainActivityEntry> = [
    {
      title: t('pages.index.domainActivityClient'),
      dataIndex: 'email',
      key: 'email',
      ellipsis: true,
      render: (value: string) => value || t('pages.index.domainActivityUnattributed'),
    },
    {
      title: t('pages.index.domainActivityDomain'),
      dataIndex: 'domain',
      key: 'domain',
      ellipsis: true,
    },
    {
      title: t(
        viewData.mode === 'active'
          ? 'pages.index.domainActivityCheckedAt'
          : 'pages.index.domainActivityLastSeen',
      ),
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      width: 125,
      render: (value: string) => {
        const date = new Date(value);
        return Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString();
      },
    },
    {
      title: t(
        viewData.mode === 'active'
          ? 'pages.index.domainActivityActiveConnections'
          : 'pages.index.domainActivityConnections',
      ),
      dataIndex: 'count',
      key: 'count',
      width: 110,
    },
  ];

  return (
    <Modal
      title={t('pages.index.domainActivityTitle')}
      open={open}
      onCancel={onClose}
      footer={null}
      width={isMobile ? 'calc(100vw - 16px)' : 900}
      destroyOnHidden
    >
      <Space orientation="vertical" style={{ width: '100%' }} size="middle">
        {nodeOptions.length > 1 && (
          <Select<number>
            aria-label={t('pages.index.domainActivityNode')}
            value={nodeId}
            options={nodeOptions}
            style={{ width: isMobile ? '100%' : 300 }}
            onChange={setNodeId}
          />
        )}
        {nodeId !== 0 && (
          <Alert type="info" showIcon description={t('pages.index.domainActivityRemoteSetting')} />
        )}
        <Alert
          type="info"
          showIcon
          description={
            viewData.mode === 'active'
              ? t('pages.index.domainActivityActiveDescription')
              : t('pages.index.domainActivityDescription', { seconds: viewData.windowSeconds })
          }
        />
        {!viewData.supported && (
          <Alert type="warning" showIcon title={t('pages.index.domainActivityUnavailable')} />
        )}
        {!!viewData.error && (
          <Alert
            type="error"
            showIcon
            title={t('pages.index.domainActivityReadError')}
            description={viewData.error}
          />
        )}
        {viewData.mode === 'recent' && viewData.supported && !viewData.accessLogEnabled && (
          <Alert type="warning" showIcon title={t('pages.index.domainActivityEnableLog')} />
        )}
        <Space align="center" wrap>
          <Typography.Text>{t('pages.index.domainActivityToggle')}</Typography.Text>
          <Switch
            checked={viewData.enabled}
            disabled={
              updating ||
              nodeId !== 0 ||
              (!viewData.enabled &&
                (!viewData.supported || (viewData.mode === 'recent' && !viewData.accessLogEnabled)))
            }
            loading={updating}
            onChange={(value) => void toggle(value)}
            aria-label={t('pages.index.domainActivityToggle')}
          />
          <Button icon={<ReloadOutlined />} onClick={refresh} loading={loading}>
            {t('refresh')}
          </Button>
        </Space>
        {viewData.enabled &&
        viewData.supported &&
        (viewData.mode === 'active' || viewData.accessLogEnabled) ? (
          <>
            <Space wrap>
              <Input
                aria-label={t('pages.index.domainActivityClientFilter')}
                placeholder={t('pages.index.domainActivityClientFilter')}
                value={clientFilter}
                allowClear
                onChange={(event) => setClientFilter(event.target.value)}
                style={{ width: isMobile ? 240 : 250 }}
              />
              <Input
                aria-label={t('pages.index.domainActivityDomainFilter')}
                placeholder={t('pages.index.domainActivityDomainFilter')}
                value={domainFilter}
                allowClear
                onChange={(event) => setDomainFilter(event.target.value)}
                style={{ width: isMobile ? 240 : 250 }}
              />
            </Space>
            <Table<DomainActivityEntry>
              size="small"
              columns={columns}
              dataSource={entries}
              rowKey={(entry) => entry.email + ':' + entry.domain}
              pagination={false}
              scroll={{ x: 680, y: 420 }}
              locale={{
                emptyText: t(
                  viewData.mode === 'active'
                    ? 'pages.index.domainActivityNoActiveData'
                    : 'pages.index.domainActivityNoData',
                ),
              }}
              loading={loading && entries.length === 0}
            />
          </>
        ) : (
          <Typography.Text type="secondary">
            {t('pages.index.domainActivityDisabled')}
          </Typography.Text>
        )}
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {t('pages.index.domainActivityDisclaimer')}
        </Typography.Text>
      </Space>
    </Modal>
  );
}
