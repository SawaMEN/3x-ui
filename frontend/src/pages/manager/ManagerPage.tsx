import { lazy, Suspense, useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Input,
  Layout,
  Modal,
  QRCode,
  Row,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tabs,
  Tag,
  Typography,
  Upload,
  message,
} from 'antd';
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { useNodeMutations } from '@/api/queries/useNodeMutations';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { HttpUtil } from '@/utils';
import { SnapshotSchema, SubscriptionSchema, newSubscription } from './schema';
import type { Relay, Subscription } from './schema';
import SubscriptionEditor from './SubscriptionEditor';
import SubscriptionWorkspace from './SubscriptionWorkspace';
import BulkSharedModal from './BulkSharedModal';
import CoverSiteModal from './CoverSiteModal';
import CoverCertificateFleet from './CoverCertificateFleet';
import RelayEditor from './RelayEditor';
import { runSNIScan } from '@/pages/inbounds/form/security/sni-scanner';
import { scannedServerName } from '@/pages/inbounds/form/security/RealityTargetScannerModal';
import './ManagerPage.css';
const NodeFormModal = lazy(() => import('@/pages/nodes/NodeFormModal'));
const InboundFormModal = lazy(() => import('@/pages/inbounds/form/InboundFormModal'));
const Scanner = lazy(() => import('@/pages/inbounds/form/security/RealityTargetScannerModal'));
export default function ManagerPage() {
  const { t, i18n } = useTranslation();
  const m = (k: string) => t('pages.manager.' + k);
  const { antdThemeConfig } = useTheme();
  const navigate = useNavigate();
  const cache = useQueryClient();
  const [msg, msgHolder] = message.useMessage();
  const [modal, modalHolder] = Modal.useModal();
  const [page, setPage] = useState(1);
  const [subscriptionPage, setSubscriptionPage] = useState(1);
  const [state, setState] = useState('');
  const [kind, setKind] = useState('');
  const [sub, setSub] = useState<Subscription | null>(null);
  const [relay, setRelay] = useState<Relay | null>(null);
  const [nodeOpen, setNodeOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [bulkSharedOpen, setBulkSharedOpen] = useState(false);
  const [coverSiteOpen, setCoverSiteOpen] = useState(false);
  const [scanOpen, setScanOpen] = useState(false);
  const [domains, setDomains] = useState('');
  const [qr, setQr] = useState('');
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');
  const [pending, setPending] = useState(false);
  const [format, setFormat] = useState('base64');
  useEffect(() => {
    const id = window.setTimeout(() => setSearch(searchInput.trim()), 300);
    return () => window.clearTimeout(id);
  }, [searchInput]);
  const { nodes, refetch: refetchNodes } = useNodesQuery();
  const nodeActions = useNodeMutations();
  const options = useInboundOptions();
  const q = useQuery({
    queryKey: ['manager', page, state, kind, subscriptionPage, search],
    queryFn: async () => {
      const r = await HttpUtil.get(
        '/panel/api/manager/state',
        { page, state, kind, subscriptionPage, search },
        { silent: true },
      );
      if (!r.success) throw new Error(r.msg);
      return SnapshotSchema.parse(r.obj);
    },
    refetchInterval: (query) => ((query.state.data?.pendingOperations ?? 0) > 0 ? 2000 : 30000),
    refetchIntervalInBackground: false,
  });
  const refresh = () => {
    void cache.invalidateQueries({ queryKey: ['manager'] });
    void options.refetch();
  };
  const action = async (path: string, data?: unknown) => {
    setPending(true);
    try {
      const r = await HttpUtil.post('/panel/api/manager' + path, data, {
        headers: { 'Content-Type': 'application/json' },
        silent: true,
      });
      if (!r.success) {
        msg.error(r.msg);
        return false;
      }
      refresh();
      return true;
    } catch (e) {
      msg.error(String(e));
      return false;
    } finally {
      setPending(false);
    }
  };
  const save = async (value: Subscription, generate: boolean) => {
    const r = await HttpUtil.post('/panel/api/manager/subscriptions', value, {
      headers: { 'Content-Type': 'application/json' },
      silent: true,
    });
    if (!r.success) {
      msg.error(r.msg);
      return false;
    }
    const saved = SubscriptionSchema.parse(r.obj);
    refresh();
    if (generate) {
      // Saving and queueing are separate operations: do not report a saved
      // subscription as unsaved if the follow-up queue request fails.
      if (await action(`/subscriptions/${saved.id}/rotate`)) msg.success(m('queued'));
      else msg.warning(m('savedNoGeneration'));
    }
    return true;
  };
  const openSubscription = async (id: number) => {
    try {
      const r = await HttpUtil.get(`/panel/api/manager/subscriptions/${id}`, undefined, {
        silent: true,
      });
      if (!r.success) {
        msg.error(r.msg);
        return;
      }
      setSub(SubscriptionSchema.parse(r.obj));
    } catch (e) {
      msg.error(String(e));
    }
  };
  const confirm = (path: string) =>
    modal.confirm({
      title: m('deleteConfirm'),
      content: m('deleteHint'),
      okText: t('delete'),
      okType: 'danger',
      onOk: async () => {
        if (!(await action(path))) throw new Error('Failed');
      },
    });
  const exportDomains = () => {
    const url = URL.createObjectURL(
      new Blob([(q.data?.domains ?? []).map((d) => d.name).join('\n')], { type: 'text/plain' }),
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = 'manager-domains.txt';
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <ConfigProvider theme={antdThemeConfig}>
      {msgHolder}
      {modalHolder}
      <Layout className="manager-page">
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Space orientation="vertical" style={{ width: '100%' }} size="middle">
              <div className="manager-page-header">
                <div className="manager-page-heading">
                  <Typography.Title level={3} style={{ margin: 0 }}>
                    {m('title')}
                  </Typography.Title>
                  <Typography.Text type="secondary">{m('subtitle')}</Typography.Text>
                </div>
                <div className="manager-page-tools">
                  <Select
                    aria-label={m('format')}
                    value={format}
                    onChange={setFormat}
                    options={['base64', 'raw', 'singbox', 'hiddify', 'xray'].map((value) => ({
                      value,
                      label: value,
                    }))}
                    style={{ minWidth: 140 }}
                  />
                  <Button icon={<ReloadOutlined />} loading={q.isFetching} onClick={refresh}>
                    {t('refresh')}
                  </Button>
                </div>
              </div>
              {q.error && (
                <Alert
                  type="error"
                  showIcon
                  title={String(q.error)}
                  action={<Button onClick={refresh}>{t('refresh')}</Button>}
                />
              )}
              <Spin spinning={q.isPending}>
                <Row gutter={[12, 12]}>
                  {[
                    [m('subscriptions'), q.data?.allSubscriptionsTotal ?? 0],
                    [m('autoRotate'), q.data?.autoRotateTotal ?? 0],
                    [m('nodes'), nodes.length],
                    [m('pending'), q.data?.pendingOperations ?? 0],
                  ].map(([label, value]) => (
                    <Col xs={12} md={6} key={String(label)}>
                      <Card size="small" className="summary-card">
                        <Statistic title={label} value={value} />
                      </Card>
                    </Col>
                  ))}
                </Row>
              </Spin>
              <Tabs
                items={[
                  {
                    key: 'subscriptions',
                    label: m('subscriptions'),
                    children: (
                      <SubscriptionWorkspace
                        data={q.data}
                        loading={q.isPending}
                        pending={pending}
                        search={searchInput}
                        page={subscriptionPage}
                        format={format}
                        onSearch={(value) => {
                          setSubscriptionPage(1);
                          setSearchInput(value);
                        }}
                        onPage={setSubscriptionPage}
                        onAdd={() => setSub(newSubscription())}
                        onRotateAll={() => void action('/rotate')}
                        onRotate={(id) => void action(`/subscriptions/${id}/rotate`)}
                        onEdit={(id) => void openSubscription(id)}
                        onDelete={(id) => confirm(`/subscriptions/${id}/delete`)}
                        onQR={setQr}
                        onManageClient={() => navigate('/clients')}
                        onBulkAdd={() => setBulkSharedOpen(true)}
                      />
                    ),
                  },
                  {
                    key: 'nodes',
                    label: m('nodes'),
                    children: (
                      <>
                        <div className="manager-toolbar">
                          <Typography.Text>{m('nodesHint')}</Typography.Text>
                          <Space wrap>
                            <Button onClick={() => navigate('/nodes')}>{m('manageNodes')}</Button>
                            <Button type="primary" onClick={() => setNodeOpen(true)}>
                              {m('addNode')}
                            </Button>
                          </Space>
                        </div>
                        <Table
                          rowKey="id"
                          dataSource={nodes}
                          scroll={{ x: 650 }}
                          columns={[
                            { title: m('name'), dataIndex: 'name' },
                            { title: m('address'), dataIndex: 'address' },
                            { title: m('status'), dataIndex: 'status' },
                            { title: m('core'), dataIndex: 'coreType' },
                            {
                              title: m('actions'),
                              render: (_, n) => (
                                <Button
                                  onClick={() =>
                                    void nodeActions.probe(n.id).then(() => refetchNodes())
                                  }
                                >
                                  {m('test')}
                                </Button>
                              ),
                            },
                          ]}
                        />
                      </>
                    ),
                  },
                  {
                    key: 'relays',
                    label: m('relays'),
                    children: (
                      <>
                        <div className="manager-toolbar">
                          <Typography.Text>{m('relayHint')}</Typography.Text>
                          <Button
                            type="primary"
                            onClick={() =>
                              setRelay({
                                id: 0,
                                name: '',
                                address: '',
                                sshPort: 22,
                                username: 'root',
                                hostKey: '',
                                keyAuth: false,
                                nodeId: 0,
                                installed: false,
                                credential: '',
                              })
                            }
                          >
                            {m('addRelay')}
                          </Button>
                        </div>
                        <Table
                          rowKey="id"
                          dataSource={q.data?.relays}
                          scroll={{ x: 650 }}
                          columns={[
                            { title: m('name'), dataIndex: 'name' },
                            { title: m('address'), dataIndex: 'address' },
                            {
                              title: m('node'),
                              render: (_, r) =>
                                nodes.find((n) => n.id === r.nodeId)?.name || r.nodeId,
                            },
                            {
                              title: m('status'),
                              render: (_, r) => (
                                <Tag color={r.installed ? 'green' : 'default'}>
                                  {m(r.installed ? 'installed' : 'notInstalled')}
                                </Tag>
                              ),
                            },
                            {
                              title: m('actions'),
                              render: (_, r) => (
                                <Space wrap>
                                  <Button
                                    size="small"
                                    disabled={pending || r.installed}
                                    onClick={() => setRelay(r)}
                                  >
                                    {t('edit')}
                                  </Button>
                                  <Button
                                    size="small"
                                    disabled={pending}
                                    onClick={() =>
                                      void action(
                                        `/relays/${r.id}/${r.installed ? 'uninstall' : 'install'}`,
                                      )
                                    }
                                  >
                                    {m(r.installed ? 'uninstall' : 'install')}
                                  </Button>
                                  <Button
                                    size="small"
                                    danger
                                    disabled={pending}
                                    onClick={() => confirm(`/relays/${r.id}/delete`)}
                                  >
                                    {t('delete')}
                                  </Button>
                                </Space>
                              ),
                            },
                          ]}
                        />
                      </>
                    ),
                  },
                  {
                    key: 'domains',
                    label: m('domains'),
                    children: (
                      <Card size="small">
                        <Space orientation="vertical" style={{ width: '100%' }}>
                          <CoverCertificateFleet nodes={nodes} />
                          <Alert type="info" showIcon title={m('domainsHint')} />
                          <Typography.Text>
                            {q.data?.domains.length ?? 0} {m('domains')}
                          </Typography.Text>
                          <Space wrap>
                            <Button icon={<SearchOutlined />} onClick={() => setScanOpen(true)}>
                              {m('scan')}
                            </Button>
                            <Button onClick={() => setCoverSiteOpen(true)}>
                              {i18n.language.startsWith('ru')
                                ? 'Сайт-заглушка и TLS'
                                : 'Cover site & TLS'}
                            </Button>
                            <Button onClick={exportDomains}>{m('export')}</Button>
                            <Upload
                              accept=".txt,.csv"
                              showUploadList={false}
                              beforeUpload={(file) => {
                                if (file.size > 1048576) {
                                  msg.error(m('fileTooLarge'));
                                  return false;
                                }
                                void file.text().then(setDomains);
                                return false;
                              }}
                            >
                              <Button>{m('import')}</Button>
                            </Upload>
                            <Button
                              onClick={() =>
                                setDomains((q.data?.domains ?? []).map((d) => d.name).join('\n'))
                              }
                            >
                              {m('loadList')}
                            </Button>
                          </Space>
                          <Input.TextArea
                            aria-label={m('domains')}
                            rows={7}
                            value={domains}
                            onChange={(e) => setDomains(e.target.value)}
                          />
                          <Space wrap>
                            <Button
                              type="primary"
                              loading={pending}
                              onClick={() =>
                                void action('/domains', { domains, replace: false }).then((ok) => {
                                  if (ok) setDomains('');
                                })
                              }
                            >
                              {m('appendDomains')}
                            </Button>
                            <Button
                              danger
                              onClick={() =>
                                modal.confirm({
                                  title: m('replaceDomains'),
                                  onOk: async () => {
                                    if (!(await action('/domains', { domains, replace: true })))
                                      throw new Error('Failed');
                                  },
                                })
                              }
                            >
                              {m('replaceDomains')}
                            </Button>
                          </Space>
                        </Space>
                      </Card>
                    ),
                  },
                  {
                    key: 'journal',
                    label: m('journal'),
                    children: (
                      <>
                        <Space wrap style={{ marginBottom: 16 }}>
                          <Select
                            aria-label={m('status')}
                            style={{ minWidth: 180 }}
                            value={state}
                            onChange={(v) => {
                              setPage(1);
                              setState(v);
                            }}
                            options={[
                              '',
                              'queued',
                              'running',
                              'success',
                              'failed',
                              'interrupted',
                              'cleanup_pending',
                              'recovered',
                            ].map((v) => ({ value: v, label: v ? m(v) : m('allStates') }))}
                          />
                          <Select
                            aria-label={m('process')}
                            style={{ minWidth: 180 }}
                            value={kind}
                            onChange={(v) => {
                              setPage(1);
                              setKind(v);
                            }}
                            options={[
                              '',
                              'rotate',
                              'delete',
                              'relay-install',
                              'relay-uninstall',
                              'relay-delete',
                              'cleanup',
                            ].map((v) => ({ value: v, label: v || m('allProcesses') }))}
                          />
                        </Space>
                        <Table
                          rowKey="id"
                          scroll={{ x: 900 }}
                          dataSource={q.data?.operations}
                          pagination={{
                            current: page,
                            pageSize: 20,
                            total: q.data?.operationTotal,
                            showSizeChanger: false,
                            onChange: setPage,
                          }}
                          columns={[
                            { title: '#', dataIndex: 'id' },
                            { title: m('process'), dataIndex: 'kind' },
                            {
                              title: m('status'),
                              dataIndex: 'state',
                              render: (v: string) => (
                                <Tag
                                  color={
                                    v === 'success'
                                      ? 'green'
                                      : v === 'failed' || v === 'interrupted'
                                        ? 'red'
                                        : 'blue'
                                  }
                                >
                                  {m(v)}
                                </Tag>
                              ),
                            },
                            {
                              title: m('progress'),
                              render: (_, op) =>
                                `${op.completed}/${op.total} · ${m('attempts')}: ${op.attempts}`,
                            },
                            { title: m('result'), dataIndex: 'message' },
                            {
                              title: m('created'),
                              dataIndex: 'createdAt',
                              render: (v: number) => new Date(v).toLocaleString(),
                            },
                            {
                              title: m('actions'),
                              render: (_, op) =>
                                ['failed', 'cleanup_pending'].includes(op.state) && (
                                  <Button
                                    disabled={pending}
                                    onClick={() => void action(`/operations/${op.id}/retry`)}
                                  >
                                    {m('retry')}
                                  </Button>
                                ),
                            },
                          ]}
                        />
                      </>
                    ),
                  },
                ]}
              />
            </Space>
          </Layout.Content>
        </Layout>
      </Layout>
      <CoverSiteModal
        open={coverSiteOpen}
        nodes={nodes}
        onClose={() => setCoverSiteOpen(false)}
        onSuccess={(domain, state) => {
          refresh();
          void cache.invalidateQueries({ queryKey: ['settings', 'managed-sni'] });
          msg.success(
            i18n.language.startsWith('ru')
              ? `Сайт ${domain}: ${state}`
              : `Site ${domain}: ${state}`,
          );
        }}
      />
      <BulkSharedModal
        open={bulkSharedOpen}
        onClose={() => setBulkSharedOpen(false)}
        onSuccess={(created) => {
          refresh();
          msg.success(
            i18n.language.startsWith('ru')
              ? `Создано и поставлено в очередь: ${created}`
              : `Created and queued: ${created}`,
          );
        }}
      />
      {sub && (
        <SubscriptionEditor
          key={`${sub.id}-${sub.token}`}
          value={sub}
          relays={q.data?.relays ?? []}
          domains={(q.data?.domains ?? []).map((d) => d.name)}
          onClose={() => setSub(null)}
          onSave={save}
          onTemplate={() => setTemplateOpen(true)}
        />
      )}
      {relay && (
        <RelayEditor
          key={relay.id}
          value={relay}
          onClose={() => setRelay(null)}
          onSave={(v) => action('/relays', v)}
        />
      )}
      <Modal open={!!qr} title={m('subscriptionUrl')} footer={null} onCancel={() => setQr('')}>
        <QRCode value={qr || ' '} size={256} style={{ margin: 'auto' }} />
      </Modal>
      <Suspense fallback={<Spin />}>
        {nodeOpen && (
          <NodeFormModal
            open
            mode="add"
            node={null}
            onOpenChange={setNodeOpen}
            testConnection={nodeActions.testConnection}
            fetchFingerprint={nodeActions.fetchFingerprint}
            fetchInbounds={nodeActions.fetchInbounds}
            save={async (v) => {
              const r = await nodeActions.create(v);
              if (r.success) {
                setNodeOpen(false);
                refresh();
              }
              return r;
            }}
          />
        )}
        {templateOpen && (
          <InboundFormModal
            open
            mode="add"
            dbInbound={null}
            dbInbounds={[]}
            availableNodes={nodes}
            availableNodesFetched
            onClose={() => setTemplateOpen(false)}
            onSaved={() => {
              setTemplateOpen(false);
              void options.refetch();
            }}
          />
        )}
        {scanOpen && (
          <Scanner
            open
            enableHistory
            purpose="reality"
            onClose={() => setScanOpen(false)}
            scanRealityCandidates={(targets, mode, signal, progress, recheck) =>
              runSNIScan(undefined, 'reality', targets, mode, signal, progress, recheck)
            }
            onPick={(r) => {
              const name = scannedServerName(r);
              if (name) void action('/domains', { domains: name, replace: false });
            }}
          />
        )}
      </Suspense>
    </ConfigProvider>
  );
}
