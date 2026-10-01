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
import {
  CloseCircleOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { HttpUtil } from '@/utils';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };
type JsonObject = Record<string, unknown>;

type SingBoxConnection = {
  id: string;
  inbound?: string;
  inboundType?: string;
  network?: string;
  source?: string;
  destination?: string;
  domain?: string;
  protocol?: string;
  user?: string;
  outbound?: string;
  outboundType?: string;
  chain?: string[];
  createdAt?: number;
  uplink?: number;
  downlink?: number;
  uplinkTotal?: number;
  downlinkTotal?: number;
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
  return typeof outbound.type === 'string' ? outbound.type : '';
}

export default function SingBoxRuntimePage() {
  const { antdThemeConfig } = useTheme();
  const [messageApi, contextHolder] = message.useMessage();
  const [connections, setConnections] = useState<SingBoxConnection[]>([]);
  const [outbounds, setOutbounds] = useState<JsonObject[]>([]);
  const [running, setRunning] = useState(false);
  const [version, setVersion] = useState('');
  const [resource, setResource] = useState('all');
  const [tag, setTag] = useState('');
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [loadingSessions, setLoadingSessions] = useState(false);
  const [testing, setTesting] = useState(false);
  const [probeResults, setProbeResults] = useState<Record<string, ProbeResult>>({});

  const loadConfig = useCallback(async () => {
    const msg = (await HttpUtil.get('/panel/api/setting/singbox/config', undefined, {
      silent: true,
    })) as ApiMsg<ConfigSnapshot>;
    if (!msg.success || !msg.obj) return;
    setRunning(!!msg.obj.running);
    setVersion(msg.obj.version || '');
    setOutbounds(Array.isArray(msg.obj.config?.outbounds) ? msg.obj.config!.outbounds! : []);
  }, []);

  const loadConnections = useCallback(async () => {
    setLoadingSessions(true);
    try {
      const params: Record<string, string> = {};
      if (resource !== 'all') params.resource = resource;
      if (tag.trim()) params.tag = tag.trim();
      const msg = (await HttpUtil.get('/panel/api/setting/singbox/connections', params, {
        silent: true,
      })) as ApiMsg<SingBoxConnection[]>;
      if (!msg.success) throw new Error(msg.msg || 'Failed to load sing-box connections');
      setConnections(Array.isArray(msg.obj) ? msg.obj : []);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoadingSessions(false);
    }
  }, [messageApi, resource, tag]);

  const refresh = useCallback(async () => {
    await Promise.all([loadConfig(), loadConnections()]);
  }, [loadConfig, loadConnections]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!autoRefresh) return;
    const timer = window.setInterval(() => void loadConnections(), 5000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, loadConnections]);

  const closeConnection = useCallback(
    async (id: string) => {
      const msg = await HttpUtil.post(`/panel/api/setting/singbox/connections/${encodeURIComponent(id)}/close`);
      if (!msg.success) return;
      setConnections((current) => current.filter((item) => item.id !== id));
    },
    [],
  );

  const testAll = useCallback(async () => {
    if (outbounds.length === 0) return;
    setTesting(true);
    try {
      const msg = (await HttpUtil.post('/panel/api/xray/testOutbounds', {
        outbounds: JSON.stringify(outbounds),
        mode: 'tcp',
      })) as ApiMsg<ProbeResult[]>;
      if (!msg.success || !Array.isArray(msg.obj)) {
        throw new Error(msg.msg || 'Failed to test sing-box outbounds');
      }
      const next: Record<string, ProbeResult> = {};
      msg.obj.forEach((result, index) => {
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
    connections.forEach((item) => {
      if (resource === 'user' && item.user) values.add(item.user);
      if (resource === 'inbound' && item.inbound) values.add(item.inbound);
      if (resource === 'outbound') {
        if (item.outbound) values.add(item.outbound);
        item.chain?.forEach((value) => values.add(value));
      }
    });
    return Array.from(values).sort();
  }, [connections, resource]);

  const sessionColumns = [
    {
      title: 'User',
      dataIndex: 'user',
      width: 140,
      render: (value: string) => value || '—',
    },
    {
      title: 'Source',
      dataIndex: 'source',
      width: 180,
      render: (value: string) => value || '—',
    },
    {
      title: 'Destination',
      key: 'destination',
      render: (_: unknown, row: SingBoxConnection) => row.domain || row.destination || '—',
    },
    {
      title: 'Inbound',
      key: 'inbound',
      width: 170,
      render: (_: unknown, row: SingBoxConnection) => (
        <Space direction="vertical" size={0}>
          <span>{row.inbound || '—'}</span>
          {row.inboundType && <Typography.Text type="secondary">{row.inboundType}</Typography.Text>}
        </Space>
      ),
    },
    {
      title: 'Outbound',
      key: 'outbound',
      width: 210,
      render: (_: unknown, row: SingBoxConnection) => (
        <Space direction="vertical" size={2}>
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
      width: 150,
      render: (_: unknown, row: SingBoxConnection) =>
        `${formatBytes(row.uplinkTotal || row.uplink)} ↑ / ${formatBytes(row.downlinkTotal || row.downlink)} ↓`,
    },
    {
      title: '',
      key: 'actions',
      width: 60,
      render: (_: unknown, row: SingBoxConnection) => (
        <Popconfirm
          title="Close connection?"
          onConfirm={() => void closeConnection(row.id)}
          okButtonProps={{ danger: true }}
        >
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
                      Live sessions, connection control and outbound reachability.
                    </Typography.Text>
                  </Col>
                  <Col>
                    <Space wrap>
                      <Tag color={running ? 'success' : 'default'}>{running ? 'running' : 'stopped'}</Tag>
                      {version && <Tag>{version}</Tag>}
                      <Button icon={<ReloadOutlined />} onClick={() => void refresh()}>
                        Refresh
                      </Button>
                    </Space>
                  </Col>
                </Row>
              </Card>

              {!running && (
                <Alert type="warning" showIcon message="Sing-box is not running. Live sessions are unavailable." />
              )}

              <Tabs
                items={[
                  {
                    key: 'sessions',
                    label: `Sessions (${connections.length})`,
                    children: (
                      <Card>
                        <Space direction="vertical" size={12} style={{ width: '100%' }}>
                          <Space wrap>
                            <Select
                              value={resource}
                              style={{ width: 150 }}
                              options={[
                                { value: 'all', label: 'All' },
                                { value: 'user', label: 'User' },
                                { value: 'inbound', label: 'Inbound' },
                                { value: 'outbound', label: 'Outbound' },
                              ]}
                              onChange={(value) => {
                                setResource(value);
                                setTag('');
                              }}
                            />
                            {resource === 'all' ? (
                              <Input
                                value={tag}
                                onChange={(event) => setTag(event.target.value)}
                                placeholder="Filter by user / inbound / outbound"
                                allowClear
                                style={{ width: 300 }}
                              />
                            ) : (
                              <Select
                                allowClear
                                showSearch
                                value={tag || undefined}
                                options={filterOptions.map((value) => ({ value, label: value }))}
                                onChange={(value) => setTag(value || '')}
                                placeholder="Filter tag"
                                style={{ width: 300 }}
                              />
                            )}
                            <Button loading={loadingSessions} onClick={() => void loadConnections()}>
                              Apply
                            </Button>
                            <Space>
                              <Switch checked={autoRefresh} onChange={setAutoRefresh} />
                              <Typography.Text type="secondary">Auto 5s</Typography.Text>
                            </Space>
                          </Space>
                          <Table
                            rowKey="id"
                            size="small"
                            loading={loadingSessions}
                            dataSource={connections}
                            columns={sessionColumns}
                            pagination={{ pageSize: 25, showSizeChanger: true }}
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
                          <Alert
                            type="info"
                            showIcon
                            message="Fast reachability test"
                            description="TCP-based native outbounds are probed directly. UDP transports and selector/urltest groups are intentionally reported as requiring an active-core probe instead of being mis-tested through Xray."
                          />
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
                                key: 'status',
                                width: 140,
                                render: (_: unknown, row: (typeof healthRows)[number]) =>
                                  !row.result ? (
                                    <Tag>not tested</Tag>
                                  ) : row.result.success ? (
                                    <Tag color="success">{row.result.delay ?? 0} ms</Tag>
                                  ) : (
                                    <Tag color="error">failed</Tag>
                                  ),
                              },
                              {
                                title: 'Details',
                                key: 'details',
                                render: (_: unknown, row: (typeof healthRows)[number]) =>
                                  row.result?.error || row.result?.mode || '—',
                              },
                            ]}
                          />
                        </Space>
                      </Card>
                    ),
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
