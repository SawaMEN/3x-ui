import { useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useServerDraft } from '@/hooks/useServerDraft';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Collapse,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import { antdRule } from '@/utils/antdRule';
import { DomainFrontStatusSchema, DomainFrontCheckSchema } from '@/generated/zod';
import type { DomainFrontCheck } from '@/generated/zod';
import {
  DomainConfigFormSchema,
  DomainFormSchema,
  defaultDomain,
  defaultDomainConfig,
} from '@/schemas/domain-front';
import type { DomainConfigForm, DomainForm } from '@/schemas/domain-front';
import { useInboundOptions } from '@/api/queries/useInboundOptions';

export default function DomainFrontTab() {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const statusQuery = useQuery({
    queryKey: keys.settings.domains(),
    queryFn: async () => {
      const msg = await HttpUtil.get('/panel/api/domains/status', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      return DomainFrontStatusSchema.parse(msg.obj);
    },
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
  });
  const status = statusQuery.data;
  const config = status?.config;
  const server = useMemo(
    () => (config ? DomainConfigFormSchema.parse(config) : undefined),
    [config],
  );
  const {
    draft: localDraft,
    setDraft,
    isDirty: dirty,
    markSaved,
  } = useServerDraft(
    server,
    (value) => structuredClone(value),
    (left, right) => JSON.stringify(left) === JSON.stringify(right),
  );
  const draft = localDraft ?? defaultDomainConfig;
  const loading = statusQuery.isPending;
  const initialized = server !== undefined;
  const { data: options = [] } = useInboundOptions();
  const [checks, setChecks] = useState<Record<string, DomainFrontCheck>>({});
  const [checking, setChecking] = useState<string>();
  const [expanded, setExpanded] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<number>();
  const [form] = Form.useForm<DomainForm>();
  const [messageApi, holder] = message.useMessage();
  const mode = Form.useWatch('mode', form);
  const tls = Form.useWatch('tls', form);
  const label = (key: string) => t(`pages.settings.domains.${key}`);

  function load() {
    return statusQuery.refetch();
  }

  function update(patch: Partial<DomainConfigForm>) {
    setDraft((prev) => ({ ...(prev ?? defaultDomainConfig), ...patch }));
  }
  async function apply() {
    const parsed = DomainConfigFormSchema.safeParse(draft);
    if (!parsed.success) {
      messageApi.error(t(parsed.error.issues[0].message));
      return;
    }
    setBusy(true);
    try {
      const msg = await HttpUtil.post('/panel/api/domains/apply', parsed.data, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      const next = DomainFrontStatusSchema.parse(msg.obj);
      const saved = DomainConfigFormSchema.parse(next.config);
      setDraft(saved);
      markSaved(saved);
      queryClient.setQueryData(keys.settings.domains(), next);
      void queryClient.invalidateQueries({ queryKey: keys.settings.defaults() });
      messageApi.success(t('pages.settings.domains.applied'));
    } catch (e) {
      messageApi.error(e instanceof Error ? e.message : t('somethingWentWrong'));
      void load();
    } finally {
      setBusy(false);
    }
  }
  async function check(name: string) {
    setChecking(name);
    try {
      const msg = await HttpUtil.post('/panel/api/domains/check', { name }, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      const result = DomainFrontCheckSchema.parse(msg.obj);
      setChecks((prev) => ({ ...prev, [name]: result }));
      setExpanded((prev) => [...new Set([...prev, name])]);
    } catch (e) {
      messageApi.error(e instanceof Error ? e.message : t('somethingWentWrong'));
    } finally {
      setChecking(undefined);
    }
  }
  function openEditor(index: number) {
    form.resetFields();
    form.setFieldsValue(index < 0 ? defaultDomain : draft.domains[index]);
    setEditing(index);
  }
  async function saveDomain() {
    const value = await form.validateFields();
    const old = editing != null && editing >= 0 ? draft.domains[editing] : undefined;
    const domain = DomainFormSchema.parse({
      ...defaultDomain,
      ...value,
      panel: value.mode === 'sub_link_only' ? false : !!value.panel,
      inboundIds: value.mode === 'sub_link_only' ? [] : (value.inboundIds ?? []),
      origin: value.mode === 'relay' ? value.origin : '',
      certFile: value.tls === 'manual' ? value.certFile : '',
      keyFile: value.tls === 'manual' ? value.keyFile : '',
    });
    const domains = [...draft.domains];
    if (editing != null && editing >= 0) domains[editing] = domain;
    else domains.push(domain);
    const canonical =
      draft.subscriptionDomain === old?.name
        ? domain.enabled
          ? domain.name
          : ''
        : draft.subscriptionDomain;
    update({ domains, subscriptionDomain: canonical });
    setEditing(undefined);
  }
  const date = (epoch: number) =>
    new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium' }).format(epoch * 1000);
  const compatible = options.filter(
    (o) =>
      o.enable &&
      !o.nodeId &&
      ['ws', 'httpupgrade', 'xhttp', 'splithttp', 'grpc'].includes(o.network || '') &&
      (!o.security || o.security === 'none' || o.security === 'tls'),
  );

  return (
    <Space orientation="vertical" size={20} style={{ width: '100%' }}>
      {holder}
      {statusQuery.error && (
        <Alert
          type="error"
          showIcon
          title={label('gatewayError')}
          description={statusQuery.error.message}
        />
      )}
      <Typography.Title level={3}>{label('title')}</Typography.Title>
      <Alert
        type="info"
        showIcon
        title={label('automation')}
        description={label('automationDesc')}
      />
      <Collapse
        items={[
          {
            key: 'help',
            label: label('help'),
            children: (
              <Space orientation="vertical" size={12}>
                {['direct', 'cdn', 'relay', 'sub_link_only'].map((key) => (
                  <div key={key}>
                    <Typography.Text strong>{label(`mode.${key}`)}</Typography.Text>
                    <Typography.Paragraph style={{ margin: 0 }}>
                      {label(`modeDesc.${key}`)}
                    </Typography.Paragraph>
                  </div>
                ))}
                <Typography.Paragraph>{label('transportHelp')}</Typography.Paragraph>
                <Typography.Paragraph>{label('dnsHelp')}</Typography.Paragraph>
              </Space>
            ),
          },
        ]}
      />
      {status?.error && (
        <Alert type="error" showIcon title={label('gatewayError')} description={status.error} />
      )}
      <Card
        loading={loading}
        title={
          <Space>
            {label('gateway')}
            <Tag color={status?.running ? 'green' : 'default'}>
              {label(status?.running ? 'running' : 'stopped')}
            </Tag>
          </Space>
        }
      >
        <Form layout="vertical">
          <Form.Item label={label('enabled')} extra={label('enabledDesc')}>
            <Switch
              aria-label={label('enabled')}
              checked={draft.enabled}
              onChange={(v) => update({ enabled: v })}
            />
          </Form.Item>
          <Form.Item label={label('email')} extra={label('emailDesc')}>
            <Input
              aria-label={label('email')}
              type="email"
              value={draft.email}
              onChange={(e) => update({ email: e.target.value })}
            />
          </Form.Item>
          <Form.Item label={label('canonical')} extra={label('canonicalDesc')}>
            <Select
              aria-label={label('canonical')}
              allowClear
              value={draft.subscriptionDomain || undefined}
              onChange={(value) => update({ subscriptionDomain: value || '' })}
              options={draft.domains
                .filter((d) => d.enabled)
                .map((d) => ({ value: d.name, label: d.name }))}
            />
          </Form.Item>
          <Checkbox
            checked={draft.acceptTerms}
            onChange={(e) => update({ acceptTerms: e.target.checked })}
          >
            {label('terms')}{' '}
            <a href="https://letsencrypt.org/repository/" target="_blank" rel="noopener noreferrer">
              Let’s Encrypt
            </a>
          </Checkbox>
          <Collapse
            style={{ marginTop: 16 }}
            items={[
              {
                key: 'advanced',
                label: label('advanced'),
                children: (
                  <>
                    <Form.Item label={label('listen')}>
                      <Input
                        value={draft.listen}
                        onChange={(e) => update({ listen: e.target.value })}
                      />
                    </Form.Item>
                    <Space wrap>
                      <Form.Item label={label('httpPort')}>
                        <InputNumber
                          min={1}
                          max={65535}
                          value={draft.httpPort}
                          onChange={(v) => {
                            if (v != null) update({ httpPort: v });
                          }}
                        />
                      </Form.Item>
                      <Form.Item label={label('httpsPort')}>
                        <InputNumber
                          min={1}
                          max={65535}
                          value={draft.httpsPort}
                          onChange={(v) => {
                            if (v != null) update({ httpsPort: v });
                          }}
                        />
                      </Form.Item>
                      <Form.Item label={label('publicPort')}>
                        <InputNumber
                          min={1}
                          max={65535}
                          value={draft.publicPort}
                          onChange={(v) => {
                            if (v != null) update({ publicPort: v });
                          }}
                        />
                      </Form.Item>
                    </Space>
                    <Typography.Paragraph type="secondary">
                      {label('portsHelp')}
                    </Typography.Paragraph>
                    <Checkbox
                      checked={draft.staging}
                      onChange={(e) => update({ staging: e.target.checked })}
                    >
                      {label('staging')}
                    </Checkbox>
                    <Typography.Paragraph type="secondary">
                      {label('stagingDesc')}
                    </Typography.Paragraph>
                  </>
                ),
              },
            ]}
          />
        </Form>
      </Card>
      <Card
        title={label('list')}
        extra={
          <Button
            icon={<PlusOutlined />}
            disabled={loading || !initialized}
            onClick={() => openEditor(-1)}
          >
            {label('add')}
          </Button>
        }
      >
        <Table
          rowKey="name"
          dataSource={draft.domains}
          pagination={false}
          scroll={{ x: 760 }}
          locale={{ emptyText: label('empty') }}
          columns={[
            {
              title: label('name'),
              dataIndex: 'name',
              render: (name: string, d: DomainForm) => (
                <Space orientation="vertical" size={2}>
                  <Typography.Text strong>{name}</Typography.Text>
                  {!d.enabled && <Tag>{label('disabled')}</Tag>}
                  {d.panel && <Tag>{label('panelAllowed')}</Tag>}
                </Space>
              ),
            },
            {
              title: label('modeLabel'),
              dataIndex: 'mode',
              render: (value: string) => <Tag>{label(`mode.${value}`)}</Tag>,
            },
            {
              title: label('certificate'),
              render: (_, d) => {
                const cert = status?.certificates.find((c) => c.name === d.name);
                return (
                  <Space orientation="vertical" size={2}>
                    <Tag
                      color={
                        cert?.state === 'valid'
                          ? 'green'
                          : cert?.state === 'expired' || cert?.state === 'error'
                            ? 'red'
                            : 'default'
                      }
                    >
                      {label(`state.${cert?.state || 'unsaved'}`)}
                    </Tag>
                    <span>{label(`tls.${d.tls}`)}</span>
                    {cert?.expiresAt ? (
                      <>
                        <span>
                          {label('remaining')}: {cert.daysLeft} {label('days')}
                        </span>
                        <span>
                          {label('expires')}: {date(cert.expiresAt)}
                        </span>
                        <Typography.Text type="secondary">{cert.issuer}</Typography.Text>
                      </>
                    ) : null}
                    {cert?.autoRenew && (
                      <Typography.Text type="secondary">{label('autoRenew')}</Typography.Text>
                    )}
                    {cert?.error && <Typography.Text type="danger">{cert.error}</Typography.Text>}
                  </Space>
                );
              },
            },
            {
              title: label('actions'),
              render: (_, d, index) => (
                <Space orientation="vertical">
                  <Button onClick={() => openEditor(index)}>{label('edit')}</Button>
                  <Button
                    loading={checking === d.name}
                    disabled={dirty || !initialized}
                    onClick={() => void check(d.name)}
                  >
                    {label('check')}
                  </Button>
                  <Popconfirm
                    title={label('removeConfirm')}
                    onConfirm={() =>
                      update({
                        domains: draft.domains.filter((_, i) => i !== index),
                        subscriptionDomain:
                          draft.subscriptionDomain === d.name ? '' : draft.subscriptionDomain,
                      })
                    }
                  >
                    <Button danger>{label('remove')}</Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
          expandable={{
            expandedRowKeys: expanded,
            onExpandedRowsChange: (keys) => setExpanded(keys.map(String)),
            expandedRowRender: (d) => {
              const result = checks[d.name];
              return result ? (
                <Space orientation="vertical">
                  <Typography.Text>
                    {label('dns')}: {result.addresses.join(', ') || '—'}
                  </Typography.Text>
                  {result.expiresAt > 0 && (
                    <Typography.Text>
                      {label('publicTLS')}: {result.issuer} · {result.daysLeft} {label('days')} ·{' '}
                      {date(result.expiresAt)}
                    </Typography.Text>
                  )}
                  {result.error && <Typography.Text type="danger">{result.error}</Typography.Text>}
                  <Typography.Text type="secondary">{label('publicTLSDesc')}</Typography.Text>
                </Space>
              ) : (
                <Typography.Text type="secondary">{label('checkHint')}</Typography.Text>
              );
            },
          }}
        />
      </Card>
      <Space wrap>
        <Button
          type="primary"
          loading={busy}
          aria-label={label('apply')}
          disabled={!dirty || loading}
          onClick={() => void apply()}
        >
          {label('apply')}
        </Button>
        <Button icon={<ReloadOutlined />} onClick={() => void load()}>
          {label('refresh')}
        </Button>
        {dirty && <Typography.Text type="secondary">{label('unsaved')}</Typography.Text>}
      </Space>
      <Modal
        open={editing !== undefined}
        title={label('editor')}
        onCancel={() => setEditing(undefined)}
        onOk={() =>
          saveDomain().catch((error: unknown) => {
            if (error instanceof Error) messageApi.error(error.message);
          })
        }
        forceRender
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="name"
            label={label('name')}
            rules={[
              antdRule(DomainFormSchema.shape.name, t),
              {
                validator: async (_rule, value: string) => {
                  if (
                    draft.domains.some(
                      (d, i) => i !== editing && d.name === value?.trim().toLowerCase(),
                    )
                  )
                    throw new Error(label('duplicate'));
                },
              },
            ]}
          >
            <Input placeholder="sub.example.com" />
          </Form.Item>
          <Form.Item name="enabled" label={label('domainEnabled')} valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="mode"
            label={label('modeLabel')}
            rules={[antdRule(DomainFormSchema.shape.mode, t)]}
          >
            <Select
              onChange={(value) => {
                if (value === 'sub_link_only')
                  form.setFieldsValue({ panel: false, inboundIds: [], origin: '' });
                else if (value !== 'relay') form.setFieldsValue({ origin: '' });
              }}
              options={['direct', 'cdn', 'relay', 'sub_link_only'].map((value) => ({
                value,
                label: label(`mode.${value}`),
              }))}
            />
          </Form.Item>
          <Typography.Paragraph type="secondary">
            {mode && label(`modeDesc.${mode}`)}
          </Typography.Paragraph>
          {mode === 'relay' && (
            <Form.Item
              name="origin"
              label={label('origin')}
              rules={[{ required: true, message: label('originRequired') }]}
              extra={label('originDesc')}
            >
              <Input placeholder="https://origin.example.com" />
            </Form.Item>
          )}
          {mode !== 'sub_link_only' && (
            <>
              <Form.Item name="panel" valuePropName="checked">
                <Checkbox>{label('panelAllowed')}</Checkbox>
              </Form.Item>
              <Form.Item name="inboundIds" label={label('inbounds')} extra={label('inboundsDesc')}>
                <Select
                  mode="multiple"
                  options={compatible.map((o) => ({
                    value: o.id,
                    label: `${o.remark} · ${o.network} · ${o.port}`,
                  }))}
                />
              </Form.Item>
            </>
          )}
          <Form.Item
            name="tls"
            label={label('tlsLabel')}
            rules={[antdRule(DomainFormSchema.shape.tls, t)]}
          >
            <Select
              options={['auto', 'manual'].map((value) => ({ value, label: label(`tls.${value}`) }))}
            />
          </Form.Item>
          <Typography.Paragraph type="secondary">
            {label(tls === 'manual' ? 'manualDesc' : 'autoDesc')}
          </Typography.Paragraph>
          {tls === 'manual' && (
            <>
              <Form.Item name="certFile" label={label('certFile')} rules={[{ required: true }]}>
                <Input placeholder="/etc/ssl/fullchain.pem" />
              </Form.Item>
              <Form.Item name="keyFile" label={label('keyFile')} rules={[{ required: true }]}>
                <Input placeholder="/etc/ssl/privkey.pem" />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>
    </Space>
  );
}
