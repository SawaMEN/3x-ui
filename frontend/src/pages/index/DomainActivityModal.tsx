import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Space, Switch, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined } from '@ant-design/icons';

import { useRefreshRequest } from '@/hooks/useRefreshRequest';
import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';

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
  windowSeconds: number;
  entries: DomainActivityEntry[];
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
  windowSeconds: 120,
  entries: [],
};

export default function DomainActivityModal({
  open,
  onClose,
  initialClient = '',
}: DomainActivityModalProps) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [clientFilter, setClientFilter] = useState(initialClient);
  const [domainFilter, setDomainFilter] = useState('');
  const [updating, setUpdating] = useState(false);

  useEffect(() => {
    if (open) setClientFilter(initialClient);
  }, [open, initialClient]);

  const load = useCallback(async (signal: AbortSignal) => {
    const msg = await HttpUtil.get<DomainActivityState>(
      '/panel/api/server/domainActivity', undefined, { signal, silent: true },
    );
    return msg?.success && msg.obj && Array.isArray(msg.obj.entries) ? msg.obj : undefined;
  }, []);

  // The backend returns only a short rolling window; polling stops when closed
  // or when the document is hidden. No client-side activity history is retained.
  const { data, loading, refresh } = useRefreshRequest(open, load, EMPTY, 3000);

  const toggle = async (enabled: boolean) => {
    setUpdating(true);
    try {
      const msg = await HttpUtil.post(
        '/panel/api/server/domainActivity/config', { enabled },
      );
      if (msg?.success) refresh();
    } finally {
      setUpdating(false);
    }
  };

  const entries = useMemo(() => {
    const client = clientFilter.trim().toLowerCase();
    const domain = domainFilter.trim().toLowerCase();
    return data.entries.filter(
      (entry) =>
        entry.email.toLowerCase().includes(client) &&
        entry.domain.toLowerCase().includes(domain),
    );
  }, [data.entries, clientFilter, domainFilter]);

  const columns: ColumnsType<DomainActivityEntry> = [
    { title: t('pages.index.domainActivityClient'), dataIndex: 'email', key: 'email', ellipsis: true },
    { title: t('pages.index.domainActivityDomain'), dataIndex: 'domain', key: 'domain', ellipsis: true },
    {
      title: t('pages.index.domainActivityLastSeen'),
      dataIndex: 'lastSeen',
      key: 'lastSeen',
      width: 125,
      render: (value: string) => {
        const date = new Date(value);
        return Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString();
      },
    },
    {
      title: t('pages.index.domainActivityConnections'),
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
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Alert
          type="info"
          showIcon
          description={t('pages.index.domainActivityDescription', {
            seconds: data.windowSeconds,
          })}
        />
        {!data.supported && (
          <Alert type="warning" showIcon message={t('pages.index.domainActivityUnavailable')} />
        )}
        {data.supported && !data.accessLogEnabled && (
          <Alert type="warning" showIcon message={t('pages.index.domainActivityEnableLog')} />
        )}
        <Space align="center" wrap>
          <Typography.Text>{t('pages.index.domainActivityToggle')}</Typography.Text>
          <Switch
            checked={data.enabled}
            disabled={updating || (!data.enabled && (!data.supported || !data.accessLogEnabled))}
            loading={updating}
            onChange={(value) => void toggle(value)}
            aria-label={t('pages.index.domainActivityToggle')}
          />
          <Button icon={<ReloadOutlined />} onClick={refresh} loading={loading}>
            {t('refresh')}
          </Button>
        </Space>
        {data.enabled && data.supported && data.accessLogEnabled ? (
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
              locale={{ emptyText: t('pages.index.domainActivityNoData') }}
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
