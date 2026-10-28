import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import { useServerDraft } from '@/hooks/useServerDraft';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  AutoComplete,
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
  Tooltip,
  Typography,
  message,
} from 'antd';
import { PlusOutlined, ReloadOutlined, QuestionCircleOutlined } from '@ant-design/icons';
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
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { redirectSiteOptions } from './redirect-sites';

export default function DomainFrontTab({
  allSetting,
  updateSetting,
  saveSettings,
  settingsDirty = false,
}: {
  saveSettings?: () => Promise<boolean | undefined>;
  settingsDirty?: boolean;
  allSetting?: AllSetting;
  updateSetting?: (patch: Partial<AllSetting>) => void;
}) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const statusQuery = useDomainFrontStatus();
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
  const [separatePublicPort, setSeparatePublicPort] = useState(false);
  const [editing, setEditing] = useState<number>();
  const [primaryDomain, setPrimaryDomain] = useState(false);
  const [form] = Form.useForm<DomainForm>();
  const [messageApi, holder] = message.useMessage();
  const mode = Form.useWatch('mode', form);
  const tls = Form.useWatch('tls', form);
  const label = (key: string) => t(`pages.settings.domains.${key}`);
  const renewalLabel = (value: string) =>
    value === 'external' || value === 'manual' ? label(`renewal.${value}`) : value;
  const certificateError = (value: string) =>
    value === 'acme_schedule_missing' ? label('acmeScheduleMissing') : value;

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
      if (settingsDirty && saveSettings && !(await saveSettings())) return;
      if (!dirty) return;
      const msg = await HttpUtil.post('/panel/api/domains/apply', parsed.data, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      const next = DomainFrontStatusSchema.parse(msg.obj);
      const saved = DomainConfigFormSchema.parse(next.config);
      setDraft(saved);
      markSaved(saved);
      queryClient.setQueryData(keys.settings.domains(), next);
      void queryClient.invalidateQueries({ queryKey: keys.settings.defaults() });
      void queryClient.invalidateQueries({ queryKey: keys.settings.all() });
      messageApi.success(t('pages.settings.domains.applied'));
      if (saved.enabled && saved.subscriptionDomain) void check(saved.subscriptionDomain);
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
    form.setFieldsValue(
      index < 0 ? { ...defaultDomain, mode: 'sub_link_only' } : draft.domains[index],
    );
    setPrimaryDomain(
      index < 0
        ? !draft.subscriptionDomain
        : draft.subscriptionDomain === draft.domains[index].name,
    );
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
      primaryDomain && domain.enabled
        ? domain.name
        : draft.subscriptionDomain === old?.name
          ? ''
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
      {allSetting && updateSetting && (
        <div className="domain-redirect-settings">
          <SettingListItem
            paddings="small"
            title={t('pages.settings.rootRedirectEnable')}
            description={t('pages.settings.rootRedirectDesc')}
          >
            <Switch
              checked={allSetting.rootRedirectEnable}
              onChange={(v) => updateSetting({ rootRedirectEnable: v })}
            />
          </SettingListItem>
          {allSetting.rootRedirectEnable && (
            <SettingListItem paddings="small" title={t('pages.settings.rootRedirectURL')}>
              <AutoComplete
                aria-label={t('pages.settings.rootRedirectURL')}
                style={{ width: '100%' }}
                value={allSetting.rootRedirectURL}
                options={redirectSiteOptions}
                showSearch={{
                  filterOption: (input, option) =>
                    (option?.value ?? '').toLowerCase().includes(input.toLowerCase()),
                }}
                defaultActiveFirstOption={false}
                allowClear
                onChange={(value) => updateSetting({ rootRedirectURL: value })}
                placeholder="https://example.com/"
              />
            </SettingListItem>
          )}
        </div>
      )}
      <Alert
        type="info"
        showIcon
        title={label('automation')}
        description={label('automationDesc')}
      />
      {!!status?.existingCertificates?.length && (
        <Collapse
          items={[
            {
              key: 'certificates',
              label: label('existingCertificates'),
              children: (
                <>
                  <Typography.Paragraph type="secondary">
                    {label('existingDesc')}
                  </Typography.Paragraph>
                  <Space orientation="vertical">
                    {status.existingCertificates.map((cert) => (
                      <div key={cert.name}>
                        <Typography.Text strong>{label(`source.${cert.source}`)}</Typography.Text>
                        {' · '}
                        {cert.issuer}
                        {' · '}
                        {label('remaining')}: {cert.daysLeft} {label('days')}
                        {' · '}
                        {date(cert.expiresAt)}
                        {' · '}
                        {renewalLabel(cert.renewal)}
                        {cert.autoRenew && <Tag color="green">{label('autoRenew')}</Tag>}
                        <div>
                          <Typography.Text copyable>{cert.certFile}</Typography.Text>
                        </div>
                        {cert.error && (
                          <Typography.Text type="warning">
                            {certificateError(cert.error)}
                          </Typography.Text>
                        )}
                      </div>
                    ))}
                  </Space>
                </>
              ),
            },
          ]}
        />
      )}
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
          <Form.Item label={label('enabled')} tooltip={label('enabledDesc')}>
            <Switch
              aria-label={label('enabled')}
              checked={draft.enabled}
              onChange={(v) => update({ enabled: v })}
            />
          </Form.Item>
          <Form.Item label={label('email')} tooltip={label('emailDesc')}>
            <Input
              aria-label={label('email')}
              type="email"
              value={draft.email}
              onChange={(e) => update({ email: e.target.value })}
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
          <Tooltip title={label('termsHelp')}>
            <QuestionCircleOutlined
              aria-label={label('termsHelp')}
              tabIndex={0}
              style={{ marginInlineStart: 8 }}
            />
          </Tooltip>
          <Collapse
            style={{ marginTop: 16 }}
            items={[
              {
                key: 'advanced',
                label: label('advanced'),
                children: (
                  <>
                    <Form.Item label={label('listen')} tooltip={label('listenHelp')}>
                      <Input
                        value={draft.listen}
                        onChange={(e) => update({ listen: e.target.value })}
                      />
                    </Form.Item>
                    <Space wrap>
                      <Form.Item label={label('httpPort')} tooltip={label('httpPortHelp')}>
                        <InputNumber
                          min={1}
                          max={65535}
                          value={draft.httpPort}
                          aria-label={label('httpPort')}
                          onChange={(v) => {
                            if (v != null) update({ httpPort: v });
                          }}
                        />
                      </Form.Item>
                      <Form.Item label={label('httpsPort')} tooltip={label('httpsPortHelp')}>
                        <InputNumber
                          min={1}
                          max={65535}
                          value={draft.httpsPort}
                          aria-label={label('httpsPort')}
                          onChange={(v) => {
                            if (v != null)
                              update({
                                httpsPort: v,
                                ...(!separatePublicPort && draft.publicPort === draft.httpsPort
                                  ? { publicPort: v }
                                  : {}),
                              });
                          }}
                        />
                      </Form.Item>
                      {(separatePublicPort || draft.publicPort !== draft.httpsPort) && (
                        <Form.Item label={label('publicPort')} tooltip={label('publicPortHelp')}>
                          <InputNumber
                            min={1}
                            max={65535}
                            value={draft.publicPort}
                            aria-label={label('publicPort')}
                            onChange={(v) => {
                              if (v != null) update({ publicPort: v });
                            }}
                          />
                        </Form.Item>
                      )}
                    </Space>
                    <Checkbox
                      checked={separatePublicPort || draft.publicPort !== draft.httpsPort}
                      onChange={(event) => {
                        setSeparatePublicPort(event.target.checked);
                        if (!event.target.checked) update({ publicPort: draft.httpsPort });
                      }}
                    >
                      {label('portMapping')}
                    </Checkbox>
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
                    {cert?.source && (
                      <Typography.Text type="secondary">
                        {label(`source.${cert.source}`)} · {renewalLabel(cert.renewal)}
                      </Typography.Text>
                    )}
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
                    {cert?.error && (
                      <Typography.Text type="danger">
                        {certificateError(cert.error)}
                      </Typography.Text>
                    )}
                  </Space>
                );
              },
            },
            {
              title: label('actions'),
              render: (_, d, index) => (
                <Space wrap>
                  <Button size="small" onClick={() => openEditor(index)}>
                    {label('edit')}
                  </Button>
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
                    <Button danger size="small">
                      {label('remove')}
                    </Button>
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
      {allSetting && updateSetting && (
        <Collapse
          items={[
            {
              key: 'manual-tls',
              label: t('pages.settings.domains.manualSettings'),
              children: (
                <>
                  <Link to="/settings#subscription">{t('pages.settings.subscriptionAccess')}</Link>
                  {(['webCertFile', 'webKeyFile', 'subCertFile', 'subKeyFile'] as const).map(
                    (key) => (
                      <SettingListItem
                        key={key}
                        paddings="small"
                        title={t(
                          `pages.settings.${({ webCertFile: 'publicKeyPath', webKeyFile: 'privateKeyPath', subCertFile: 'subCertPath', subKeyFile: 'subKeyPath' } as const)[key]}`,
                        )}
                      >
                        <Input
                          value={allSetting[key]}
                          onChange={(e) => updateSetting({ [key]: e.target.value })}
                        />
                      </SettingListItem>
                    ),
                  )}
                </>
              ),
            },
          ]}
        />
      )}

      <Space wrap>
        <Button
          type="primary"
          loading={busy}
          aria-label={label('apply')}
          disabled={(!dirty && !settingsDirty) || loading}
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
            tooltip={label('nameHelp')}
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
          <Form.Item
            name="enabled"
            label={label('domainEnabled')}
            tooltip={label('domainEnabledHelp')}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          <Form.Item
            name="mode"
            label={label('modeLabel')}
            tooltip={label('modeHelp')}
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
              tooltip={label('originDesc')}
              rules={[{ required: true, message: label('originRequired') }]}
              extra={label('originDesc')}
            >
              <Input placeholder="https://origin.example.com" />
            </Form.Item>
          )}
          {mode !== 'sub_link_only' && (
            <>
              <Form.Item
                name="panel"
                label={label('panelAllowed')}
                tooltip={label('panelHelp')}
                valuePropName="checked"
              >
                <Checkbox />
              </Form.Item>
              <Form.Item
                name="inboundIds"
                label={label('inbounds')}
                tooltip={label('inboundsDesc')}
                extra={label('inboundsDesc')}
              >
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
          <Form.Item label={label('useForSubscription')} tooltip={label('useForSubscriptionHelp')}>
            <Checkbox
              checked={primaryDomain}
              onChange={(event) => setPrimaryDomain(event.target.checked)}
            >
              {label('useForSubscription')}
            </Checkbox>
          </Form.Item>
          <Form.Item
            name="tls"
            label={label('tlsLabel')}
            tooltip={label('tlsHelp')}
            rules={[antdRule(DomainFormSchema.shape.tls, t)]}
          >
            <Select
              options={['auto', 'certmagic', 'existing', 'manual'].map((value) => ({
                value,
                label: label(`tls.${value}`),
              }))}
            />
          </Form.Item>
          <Typography.Paragraph type="secondary">
            {label(
              tls === 'manual'
                ? 'manualDesc'
                : tls === 'certmagic'
                  ? 'certmagicDesc'
                  : tls === 'existing'
                    ? 'existingTLSDesc'
                    : 'autoDesc',
            )}
          </Typography.Paragraph>
          {tls === 'manual' && (
            <>
              <Form.Item
                name="certFile"
                label={label('certFile')}
                tooltip={label('certFileHelp')}
                rules={[{ required: true }]}
              >
                <Input placeholder="/etc/ssl/fullchain.pem" />
              </Form.Item>
              <Form.Item
                name="keyFile"
                label={label('keyFile')}
                tooltip={label('keyFileHelp')}
                rules={[{ required: true }]}
              >
                <Input placeholder="/etc/ssl/privkey.pem" />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>
    </Space>
  );
}
