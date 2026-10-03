import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Input,
  Layout,
  Popconfirm,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { CloseCircleOutlined, ReloadOutlined, ThunderboltOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { HttpUtil } from '@/utils';
import TrafficHistoryPanel from './TrafficHistoryPanel';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };
type JsonObject = Record<string, unknown>;

type Session = {
  id: string;
  inbound?: string;
  user?: string;
  outbound?: string;
  source?: string;
  destination?: string;
  domain?: string;
  upload?: number;
  download?: number;
  chain?: string[];
};

type ConfigSnapshot = {
  config?: { outbounds?: JsonObject[] };
  running?: boolean;
  version?: string;
};

type ProbeResult = {
  tag?: string;
  success?: boolean;
  delay?: number;
  error?: string;
  mode?: string;
};

function formatBytes(value = 0) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`;
}

function outboundTag(outbound: JsonObject) {
  return typeof outbound.tag === 'string' ? outbound.tag : '';
}

function outboundType(outbound: JsonObject) {
  if (typeof outbound.type === 'string') return outbound.type;
  return typeof outbound.protocol === 'string' ? outbound.protocol : '';
}

export default function SingBoxRuntimePage() {
  const { antdThemeConfig } = useTheme();
  const [messageApi, contextHolder] = message.useMessage();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [outbounds, setOutbounds] = useState<JsonObject[]>([]);
  const [running, setRunning] = useState(false);
  const [version, setVersion] = useState('');
  const [resource, setResource] = useState<'all' | 'user' | 'inbound' | 'outbound'>('all');
  const [filter, setFilter] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const [testing, setTesting] = useState(false);
  const [probeResults, setProbeResults] = useState<Record<string, ProbeResult>>({});

  const loadConfig = useCallback(async () => {
    const response = (await HttpUtil.get('/panel/api/setting/singbox/config', undefined, {
      silent: true,
    })) as ApiMsg<ConfigSnapshot>;
    if (!response.success || !response.obj) return;
    setRunning(!!response.obj.running);
    setVersion(response.obj.version || '');
    setOutbounds(
      Array.isArray(response.obj.config?.outbounds) ? response.obj.config.outbounds : [],
    );
  }, []);

  const loadSessions = useCallback(async () => {
    setLoadingSessions(true);
    try {
      const response = (await HttpUtil.get('/panel/api/server/singbox/sessions', undefined, {
        silent: true,
      })) as ApiMsg<Session[]>;
      if (!response.success) {
        throw new Error(response.msg || 'Failed to load sing-box sessions');
      }
      setSessions(Array.isArray(response.obj) ? response.obj : []);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoadingSessions(false);
    }
  }, [messageApi]);

  const refresh = useCallback(async () => {
    await Promise.all([loadConfig(), loadSessions()]);
  }, [loadConfig, loadSessions]);

  useEffect(() => {
    const timer = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(timer);
  }, [refresh]);

  useEffect(() => {
    if (!autoRefresh) return undefined;
    const timer = window.setInterval(() => void loadSessions(), 5000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, loadSessions]);

  const disconnect = useCallback(
    async (session: Session) => {
      let response: ApiMsg;
      if (session.user) {
        response = (await HttpUtil.post('/panel/api/server/singbox/sessions/disconnect-user', {
          user: session.user,
          inbound: session.inbound || '',
        })) as ApiMsg;
      } else if (session.inbound) {
        response = (await HttpUtil.post('/panel/api/server/singbox/sessions/disconnect-inbound', {
          inbound: session.inbound,
        })) as ApiMsg;
      } else {
        messageApi.warning('This session has no user or inbound identity to disconnect safely.');
        return;
      }
      if (!response.success) {
        messageApi.error(response.msg || 'Failed to disconnect session');
        return;
      }
      await loadSessions();
    },
    [loadSessions, messageApi],
  );

  const testAll = useCallback(async () => {
    if (outbounds.length === 0) return;
    setTesting(true);
    try {
      const response = (await HttpUtil.post('/panel/api/xray/testOutbounds', {
        outbounds: JSON.stringify(outbounds),
        mode: 'tcp',
      })) as ApiMsg<ProbeResult[]>;
      if (!response.success || !Array.isArray(response.obj)) {
        throw new Error(response.msg || 'Failed to test sing-box outbounds');
      }
      const next: Record<string, ProbeResult> = {};
      response.obj.forEach((result, index) => {
        const name = result.tag || outboundTag(outbounds[index]) || `#${index + 1}`;
        next[name] = result;
      });
      setProbeResults(next);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setTesting(false);
    }
  }, [messageApi, outbounds]);

  const filterOptions = useMemo(() => {
    const values = new Set<string>();
    sessions.forEach((session) => {
      if (resource === 'user' && session.user) values.add(session.user);
      if (resource === 'inbound' && session.inbound) values.add(session.inbound);
      if (resource === 'outbound') {
        if (session.outbound) values.add(session.outbound);
        session.chain?.forEach((value) => values.add(value));
      }
    });
    return Array.from(values).sort();
  }, [resource, sessions]);

  const visibleSessions = useMemo(() => {
    const wanted = filter.trim().toLowerCase();
    if (!wanted) return sessions;
    return sessions.filter((session) => {
      if (resource === 'user') return session.user?.toLowerCase() === wanted;
      if (resource === 'inbound') return session.inbound?.toLowerCase() === wanted;
      if (resource === 'outbound') {
        return (
          session.outbound?.toLowerCase() === wanted ||
          session.chain?.some((item) => item.toLowerCase() === wanted)
        );
      }
      return [
        session.user,
        session.inbound,
        session.outbound,
        session.source,
        session.destination,
        session.domain,
      ]
        .filter(Boolean)
        .some((item) => String(item).toLowerCase().includes(wanted));
    });
  }, [filter, resource, sessions]);

  const sessionColumns = [
    { title: 'User', dataIndex: 'user', width: 140, render: (value: string) => value || '—' },
    { title: 'Source', dataIndex: 'source', width: 180, render: (value: string) => value || '—' },
    {
      title: 'Destination',
      key: 'destination',
      render: (_: unknown, row: Session) => row.domain || row.destination || '—',
    },
    { title: 'Inbound', dataIndex: 'inbound', width: 170, render: (value: string) => value || '—' },
    {
      title: 'Outbound',
      key: 'outbound',
      width: 220,
      render: (_: unknown, row: Session) => (
        <Space direction="vertical" size={0}>
          <span>{row.outbound || '—'}</span>
          {!!row.chain?.length && (
            <Typography.Text type="secondary">{row.chain.join(' → ')}</Typography.Text>
          )}
        </Space>
      ),
    },
    {
      title: 'Traffic',
      key: 'traffic',
      width: 170,
      render: (_: unknown, row: Session) =>
        `${formatBytes(row.upload)} ↑ / ${formatBytes(row.download)} ↓`,
    },
    {
      title: '',
      key: 'actions',
      width: 60,
      render: (_: unknown, row: Session) => (
        <Popconfirm title="Disconnect matching session(s)?" onConfirm={() => void disconnect(row)}>
          <Button danger type="text" icon={<CloseCircleOutlined />} />
        </Popconfirm>
      ),
    },
  ];

  const healthRows = outbounds.map((outbound, index) => {
    const name = outboundTag(outbound) || `#${index + 1}`;
    return { key: name, name, type: outboundType(outbound), result: probeResults[name] };
  });

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {contextHolder}
      <Layout className="page-layout">
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Space direction="vertical" size={12} style={{ width: '100%' }}>
              <Card>
                <Row gutter={[12, 12]} align="middle">
                  <Col flex="auto">
                    <Typography.Title level={2} style={{ margin: 0 }}>
                      Sing-box Runtime
                    </Typography.Title>
                    <Typography.Text type="secondary">
                      Live sessions, traffic history and outbound health.
                    </Typography.Text>
                  </Col>
                  <Col>
                    <Space wrap>
                      <Tag color={running ? 'success' : 'default'}>
                        {running ? 'running' : 'stopped'}
                      </Tag>
                      {version && <Tag>{version}</Tag>}
                      <Button icon={<ReloadOutlined />} onClick={() => void refresh()}>
                        Refresh
                      </Button>
                    </Space>
                  </Col>
                </Row>
              </Card>

              {!running && (
                <Alert
                  type="warning"
                  showIcon
                  message="Sing-box is not running; live sessions are unavailable."
                />
              )}

              <Tabs
                items={[
                  {
                    key: 'sessions',
                    label: `Sessions (${visibleSessions.length})`,
                    children: (
                      <Card>
                        <Space direction="vertical" size={12} style={{ width: '100%' }}>
                          <Space wrap>
                            <Select
                              value={resource}
                              style={{ width: 150 }}
                              options={['all', 'user', 'inbound', 'outbound'].map((value) => ({
                                value,
                                label: value,
                              }))}
                              onChange={(value) => {
                                setResource(value);
                                setFilter('');
                              }}
                            />
                            {resource === 'all' ? (
                              <Input
                                value={filter}
                                onChange={(event) => setFilter(event.target.value)}
                                allowClear
                                placeholder="Filter sessions"
                                style={{ width: 280 }}
                              />
                            ) : (
                              <Select
                                allowClear
                                showSearch
                                value={filter || undefined}
                                options={filterOptions.map((value) => ({ value, label: value }))}
                                onChange={(value) => setFilter(value || '')}
                                style={{ width: 280 }}
                              />
                            )}
                            <Button loading={loadingSessions} onClick={() => void loadSessions()}>
                              Refresh
                            </Button>
                            <Switch checked={autoRefresh} onChange={setAutoRefresh} />
                            <Typography.Text type="secondary">Auto 5s</Typography.Text>
                          </Space>
                          <Table
                            rowKey="id"
                            size="small"
                            dataSource={visibleSessions}
                            columns={sessionColumns}
                            loading={loadingSessions}
                            pagination={{ pageSize: 25 }}
                            scroll={{ x: 1100 }}
                          />
                        </Space>
                      </Card>
                    ),
                  },
                  {
                    key: 'health',
                    label: 'Outbound health',
                    children: (
                      <Card>
                        <Space direction="vertical" size={12} style={{ width: '100%' }}>
                          <Button
                            type="primary"
                            icon={<ThunderboltOutlined />}
                            loading={testing}
                            disabled={outbounds.length === 0}
                            onClick={() => void testAll()}
                          >
                            Test all
                          </Button>
                          <Table
                            rowKey="key"
                            size="small"
                            dataSource={healthRows}
                            pagination={false}
                            columns={[
                              { title: 'Tag', dataIndex: 'name' },
                              { title: 'Type', dataIndex: 'type', width: 150 },
                              {
                                title: 'Status',
                                width: 140,
                                render: (_: unknown, row: (typeof healthRows)[number]) =>
                                  !row.result ? (
                                    <Tag>not tested</Tag>
                                  ) : row.result.success ? (
                                    <Tag color="success">{row.result.delay || 0} ms</Tag>
                                  ) : (
                                    <Tag color="error">failed</Tag>
                                  ),
                              },
                              {
                                title: 'Details',
                                render: (_: unknown, row: (typeof healthRows)[number]) =>
                                  row.result?.error || row.result?.mode || '—',
                              },
                            ]}
                          />
                        </Space>
                      </Card>
                    ),
                  },
                  {
                    key: 'history',
                    label: 'Traffic history',
                    children: <TrafficHistoryPanel />,
                  },
                ]}
              />
            </Space>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
