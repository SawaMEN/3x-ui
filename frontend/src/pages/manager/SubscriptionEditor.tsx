import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { FormProvider, useFieldArray, useForm, useWatch } from 'react-hook-form';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Tabs,
} from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { FormField } from '@/components/form/rhf';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { isClientAttachableProtocol } from '@/lib/inbounds/client-attachable';
import { HttpUtil } from '@/utils';
import { ClientOptionsSchema, newEntry, SubscriptionSchema, PreflightSchema } from './schema';
import type { Relay, Subscription } from './schema';
import { MANAGER_PRESETS, createManagerPreset } from './presets';
import { MAX_MANAGER_ENTRIES } from './catalog';
import ProtocolBuilder from './ProtocolBuilder';
import { DBInbound } from '@/models/dbinbound';
const ConfigurationEditor = lazy(() => import('@/pages/inbounds/form/InboundFormModal'));

export default function SubscriptionEditor({
  value,
  relays,
  domains,
  onClose,
  onSave,
  onTemplate,
}: {
  value: Subscription;
  relays: Relay[];
  domains: string[];
  onClose: () => void;
  onSave: (v: Subscription, generate: boolean) => Promise<boolean>;
  onTemplate: () => void;
}) {
  const { t, i18n } = useTranslation();
  const m = (k: string) => t('pages.manager.' + k);
  const form = useForm<Subscription>({ defaultValues: value });
  const { fields, append, remove, replace } = useFieldArray({
    control: form.control,
    name: 'entries',
  });
  const [editingConfig, setEditingConfig] = useState<number | null>(null);
  const entries = useWatch({ control: form.control, name: 'entries' });
  const { data: templates = [] } = useInboundOptions();
  const { nodes } = useNodesQuery();
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [search, setSearch] = useState('');
  const [query, setQuery] = useState('');
  useEffect(() => {
    const timer = setTimeout(() => setQuery(search), 250);
    return () => clearTimeout(timer);
  }, [search]);
  const clients = useQuery({
    queryKey: ['manager-clients', query],
    queryFn: async () => {
      const r = await HttpUtil.get(
        '/panel/api/clients/list/paged',
        { search: query, page: 1, pageSize: 50 },
        { silent: true },
      );
      if (!r.success) throw new Error(r.msg);
      return ClientOptionsSchema.parse(r.obj).items;
    },
  });
  const configDraft = useMemo(() => {
    if (editingConfig === null) return null;
    const entry = form.getValues(`entries.${editingConfig}`);
    const config = entry.config;
    return new DBInbound({
      protocol: config?.protocol ?? 'vless',
      settings: config?.settings ?? '{}',
      streamSettings: config?.streamSettings ?? '{}',
      sniffing: config?.sniffing ?? '{}',
      listen: config?.listen ?? '',
      shareAddrStrategy: config?.shareAddrStrategy ?? 'node',
      shareAddr: config?.shareAddr ?? '',
      subSortIndex: config?.subSortIndex ?? 1,
      port: entry.port === 'random' ? 10000 : Number(entry.port) || 10000,
      nodeId: entry.nodeId || null,
      remark: entry.label,
      disableFlow: config?.disableFlow ?? false,
      enable: true,
    });
  }, [editingConfig, form]);
  const [checked, setChecked] = useState<{ ready: boolean; signature: string } | null>(null);
  const watchedValues = useWatch({ control: form.control });
  const check = async () => {
    const current = form.getValues();
    const signature = JSON.stringify(current);
    const parsed = SubscriptionSchema.safeParse(current);
    setChecked(null);
    if (!parsed.success) {
      setError(
        parsed.error.issues.map((issue) => issue.path.join('.') + ': ' + issue.message).join('\n'),
      );
      return false;
    }
    setSaving(true);
    setError('');
    try {
      const response = await HttpUtil.post('/panel/api/manager/preflight', parsed.data, {
        headers: { 'Content-Type': 'application/json' },
        silent: true,
      });
      // The user may edit fields while a slow preflight request is in flight.
      // Never approve a newer draft using the old server result.
      if (JSON.stringify(form.getValues()) !== signature) {
        setError(m('formChangedDuringCheck'));
        return false;
      }
      if (!response.success) {
        setError(response.msg);
        return false;
      }
      const result = PreflightSchema.parse(response.obj);
      setChecked({ ready: result.ready, signature });
      if (!result.ready) {
        setError(
          result.entries
            .filter((entry) => !entry.ready)
            .map((entry) => entry.index + ': ' + entry.message)
            .join('\n'),
        );
      }
      return result.ready;
    } catch (e) {
      setError(String(e));
      return false;
    } finally {
      setSaving(false);
    }
  };
  const submit = async (generate: boolean) => {
    if (saving) return;
    if (generate && !(await check())) return;
    // Parse only after preflight, so the submitted draft is exactly the
    // version that passed readiness checks.
    const parsed = SubscriptionSchema.safeParse(form.getValues());
    if (!parsed.success) {
      setChecked(null);
      setError(
        parsed.error.issues.map((issue) => issue.path.join('.') + ': ' + issue.message).join('\n'),
      );
      return;
    }
    setSaving(true);
    setError('');
    try {
      if (await onSave(parsed.data, generate)) onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal
      open
      title={m('editSubscription')}
      className="manager-editor"
      width={960}
      style={{ top: 12 }}
      styles={{ body: { maxHeight: 'calc(100dvh - 190px)', overflowY: 'auto' } }}
      mask={{ closable: false }}
      onCancel={() => {
        if (!saving) onClose();
      }}
      footer={
        <Space wrap>
          <Button disabled={saving} onClick={onClose}>
            {t('cancel')}
          </Button>
          <Button loading={saving} disabled={saving} onClick={() => void check()}>
            {m('preflight')}
          </Button>
          <Button loading={saving} disabled={saving} onClick={() => void submit(false)}>
            {t('save')}
          </Button>
          <Button
            type="primary"
            loading={saving}
            disabled={saving || watchedValues?.enabled === false}
            onClick={() => void submit(true)}
          >
            {m('saveGenerate')}
          </Button>
        </Space>
      }
    >
      <FormProvider {...form}>
        <Form layout="vertical" component="div">
          {error && <Alert type="error" showIcon title={error} />}
          {checked?.ready && checked.signature === JSON.stringify(watchedValues) && !error && (
            <Alert type="success" showIcon title={m('ready')} />
          )}
          <Tabs
            defaultActiveKey={value.id === 0 ? 'builder' : 'general'}
            items={[
              {
                key: 'builder',
                label: i18n.language.startsWith('ru')
                  ? 'Конструктор протоколов'
                  : 'Protocol builder',
                children: (
                  <ProtocolBuilder
                    nodes={nodes}
                    domains={domains}
                    currentCount={fields.length}
                    russian={i18n.language.startsWith('ru')}
                    onApply={(recipes, replaceExisting) => {
                      if (replaceExisting) replace(recipes);
                      else append(recipes);
                      setChecked(null);
                    }}
                  />
                ),
              },
              {
                key: 'general',
                label: m('general'),
                children: (
                  <>
                    <Row gutter={16}>
                      <Col xs={24} sm={16}>
                        <FormField name="name" label={m('name')} required>
                          <Input maxLength={256} />
                        </FormField>
                      </Col>
                      <Col xs={24} sm={8}>
                        <FormField name="enabled" label={m('enabled')} valueProp="checked">
                          <Switch />
                        </FormField>
                      </Col>
                    </Row>
                    <FormField name="clientId" label={m('client')} extra={m('clientHint')}>
                      <Select
                        disabled={value.id > 0}
                        showSearch={{ onSearch: setSearch, filterOption: false }}
                        loading={clients.isFetching}
                        options={[
                          { value: 0, label: m('autoClient') },
                          ...(value.clientId > 0 &&
                          !(clients.data ?? []).some((c) => c.id === value.clientId)
                            ? [
                                {
                                  value: value.clientId,
                                  label: m('client') + ' #' + value.clientId,
                                },
                              ]
                            : []),
                          ...(clients.data ?? []).map((c) => ({
                            value: c.id,
                            label: `${c.email} · #${c.id}`,
                          })),
                        ]}
                      />
                    </FormField>
                    {clients.error && <Alert type="error" title={String(clients.error)} />}
                    <Row gutter={16}>
                      <Col xs={24} sm={12}>
                        <FormField name="autoRotate" label={m('autoRotate')} valueProp="checked">
                          <Switch />
                        </FormField>
                      </Col>
                      <Col xs={24} sm={12}>
                        <FormField name="intervalMinutes" label={m('interval')}>
                          <InputNumber min={10} max={525600} style={{ width: '100%' }} />
                        </FormField>
                      </Col>
                    </Row>
                    <Row gutter={16}>
                      <Col xs={24} sm={12}>
                        <FormField
                          name="monitorSni"
                          label={
                            i18n.language.startsWith('ru')
                              ? 'Мониторинг REALITY SNI'
                              : 'REALITY SNI health monitoring'
                          }
                          valueProp="checked"
                          extra={
                            i18n.language.startsWith('ru')
                              ? 'Переключение на проверенный резерв после 3 неудач; ключи сохраняются. Нужен резервный SNI.'
                              : 'Switch to a verified backup after 3 failures; keys remain unchanged. Backup SNI required.'
                          }
                        >
                          <Switch />
                        </FormField>
                      </Col>
                      <Col xs={24} sm={12}>
                        <FormField
                          name="monitorSniIntervalMinutes"
                          label={
                            i18n.language.startsWith('ru')
                              ? 'Интервал проверки SNI, мин.'
                              : 'SNI check interval, minutes'
                          }
                        >
                          <InputNumber min={5} max={1440} style={{ width: '100%' }} />
                        </FormField>
                      </Col>
                    </Row>
                    <FormField name="graceMinutes" label={m('graceMinutes')} extra={m('graceHint')}>
                      <InputNumber min={60} max={1440} style={{ width: '100%' }} />
                    </FormField>
                    <FormField name="note" label={m('note')}>
                      <Input.TextArea rows={3} />
                    </FormField>
                    <Row gutter={16}>
                      {(['supportUrl', 'websiteUrl'] as const).map((key) => (
                        <Col xs={24} sm={12} key={key}>
                          <FormField name={key} label={m(key)}>
                            <Input placeholder="https://" />
                          </FormField>
                        </Col>
                      ))}
                    </Row>
                  </>
                ),
              },
              {
                key: 'connections',
                label: `${m('connections')} (${fields.length}/${MAX_MANAGER_ENTRIES})`,
                children: (
                  <>
                    <Alert
                      type="info"
                      title={m('templateHint')}
                      action={<Button onClick={onTemplate}>{m('newTemplate')}</Button>}
                    />
                    <Space orientation="vertical" style={{ width: '100%', marginTop: 12 }}>
                      {fields.map((f, i) => {
                        const e = entries[i];
                        const template = templates.find((v) => v.id === e?.templateId);
                        return (
                          <Card
                            key={f.id}
                            size="small"
                            title={`${m('connection')} ${i + 1}`}
                            extra={
                              <Button
                                aria-label={t('delete')}
                                type="text"
                                danger
                                icon={<DeleteOutlined />}
                                disabled={fields.length === 1}
                                onClick={() => remove(i)}
                              />
                            }
                          >
                            <Row gutter={12}>
                              <Col xs={24} sm={8}>
                                <FormField
                                  name={`entries.${i}.mode`}
                                  label={m('mode')}
                                  onAfterChange={(mode) => {
                                    if (mode === 'generate' && !e?.config) {
                                      const preset = e?.preset ?? 'vless-tcp-reality';
                                      form.setValue(`entries.${i}.preset`, preset);
                                      form.setValue(
                                        `entries.${i}.config`,
                                        createManagerPreset(preset),
                                      );
                                    }
                                    form.setValue(
                                      `entries.${i}.nodeId`,
                                      mode === 'generate' ? 0 : undefined,
                                    );
                                    form.setValue(
                                      `entries.${i}.sni`,
                                      mode === 'generate' && e?.preset?.endsWith('-reality')
                                        ? 'random'
                                        : '',
                                    );
                                    form.setValue(
                                      `entries.${i}.port`,
                                      mode === 'existing' ? '' : 'random',
                                    );
                                    form.setValue(`entries.${i}.customLink`, '');
                                  }}
                                >
                                  <Select
                                    options={['generate', 'rotate', 'existing', 'custom'].map(
                                      (v) => ({
                                        value: v,
                                        label: m(v),
                                      }),
                                    )}
                                  />
                                </FormField>
                              </Col>
                              <Col xs={24} sm={16}>
                                {e?.mode === 'generate' ? (
                                  <FormField
                                    name={`entries.${i}.preset`}
                                    label={m('preset')}
                                    onAfterChange={(v) => {
                                      const kind = String(v);
                                      form.setValue(
                                        `entries.${i}.config`,
                                        createManagerPreset(kind),
                                      );
                                      form.setValue(
                                        `entries.${i}.sni`,
                                        kind.endsWith('-reality') ? 'random' : '',
                                      );
                                    }}
                                  >
                                    <Select
                                      showSearch
                                      options={[
                                        ...MANAGER_PRESETS.map((value) => ({
                                          value,
                                          label: value,
                                        })),
                                        ...(e?.preset === 'custom-config'
                                          ? [{ value: 'custom-config', label: m('customConfig') }]
                                          : []),
                                      ]}
                                    />
                                  </FormField>
                                ) : e?.mode === 'custom' ? (
                                  <FormField
                                    name={`entries.${i}.customLink`}
                                    label={m('shareLink')}
                                  >
                                    <Input.TextArea autoSize />
                                  </FormField>
                                ) : (
                                  <FormField name={`entries.${i}.templateId`} label={m('template')}>
                                    <Select
                                      showSearch={{ optionFilterProp: 'label' }}
                                      options={templates
                                        .filter(
                                          (v) =>
                                            isClientAttachableProtocol(v.protocol) &&
                                            !v.tag?.includes('manager-'),
                                        )
                                        .map((v) => ({
                                          value: v.id,
                                          label: `${v.remark || v.tag} · ${v.protocol} · ${v.port}`,
                                        }))}
                                    />
                                  </FormField>
                                )}
                              </Col>
                            </Row>
                            <Row gutter={12}>
                              <Col xs={24} sm={16}>
                                <FormField name={`entries.${i}.label`} label={m('label')}>
                                  <Input />
                                </FormField>
                              </Col>
                              <Col xs={24} sm={8}>
                                <FormField name={`entries.${i}.flag`} label={m('flag')}>
                                  <Input maxLength={16} />
                                </FormField>
                              </Col>
                            </Row>
                            {e?.mode === 'generate' && (
                              <>
                                <Button onClick={() => setEditingConfig(i)}>
                                  {m('configureGeneration')}
                                </Button>
                                <Alert
                                  type="info"
                                  title={m('recipeHint')}
                                  style={{ marginBlock: 12 }}
                                />
                              </>
                            )}
                            {e?.mode !== 'custom' && (
                              <Row gutter={12}>
                                {(e?.mode === 'rotate' || e?.mode === 'generate') && (
                                  <>
                                    <Col xs={24} sm={12}>
                                      <FormField name={`entries.${i}.nodeId`} label={m('node')}>
                                        <Select
                                          allowClear
                                          placeholder={m('templateNode')}
                                          options={[
                                            { value: 0, label: m('localNode') },
                                            ...nodes
                                              .filter(
                                                (n) =>
                                                  n.enable &&
                                                  nodeSupportsProtocol(
                                                    n,
                                                    e?.mode === 'generate'
                                                      ? (e.config?.protocol ?? 'vless')
                                                      : (template?.protocol ?? 'vless'),
                                                  ),
                                              )
                                              .map((n) => ({ value: n.id, label: n.name })),
                                          ]}
                                        />
                                      </FormField>
                                    </Col>
                                    <Col xs={24} sm={12}>
                                      <FormField
                                        name={`entries.${i}.port`}
                                        label={m('port')}
                                        extra={m('portHint')}
                                      >
                                        <Input />
                                      </FormField>
                                    </Col>
                                    <Col xs={24} sm={12}>
                                      <FormField name={`entries.${i}.sni`} label="SNI">
                                        <AutoComplete
                                          allowClear
                                          placeholder={m('templateSni')}
                                          options={[
                                            { value: 'random', label: m('randomSni') },
                                            ...domains.map((value) => ({ value })),
                                          ]}
                                        />
                                      </FormField>
                                    </Col>
                                    {e?.mode === 'generate' &&
                                      e.config?.streamSettings?.includes(
                                        '"security":"reality"',
                                      ) && (
                                        <Col xs={24} sm={12}>
                                          <FormField
                                            name={`entries.${i}.realityTarget`}
                                            label={
                                              i18n.language.startsWith('ru')
                                                ? 'Сайт-заглушка (REALITY Target)'
                                                : 'Camouflage website (REALITY Target)'
                                            }
                                            extra={
                                              i18n.language.startsWith('ru')
                                                ? 'Домен:порт; если пусто, используется SNI.'
                                                : 'Hostname:port; empty uses SNI.'
                                            }
                                          >
                                            <Input placeholder="example.com:443" />
                                          </FormField>
                                        </Col>
                                      )}
                                  </>
                                )}
                                <Col xs={24} sm={12}>
                                  <FormField name={`entries.${i}.relayId`} label={m('relay')}>
                                    <Select
                                      options={[
                                        { value: 0, label: m('direct') },
                                        ...relays
                                          .filter((r) => r.installed)
                                          .map((r) => ({ value: r.id, label: r.name })),
                                      ]}
                                    />
                                  </FormField>
                                </Col>
                              </Row>
                            )}
                          </Card>
                        );
                      })}
                      <Button
                        block
                        type="dashed"
                        icon={<PlusOutlined />}
                        disabled={fields.length >= MAX_MANAGER_ENTRIES}
                        onClick={() => append(newEntry())}
                      >
                        {m('addConnection')}
                      </Button>
                    </Space>
                  </>
                ),
              },
              {
                key: 'routing',
                label: m('routing'),
                children: (
                  <>
                    <Alert type="info" title={m('routingHint')} />
                    <FormField name="happ" label="Happ">
                      <Input.TextArea rows={5} />
                    </FormField>
                    <FormField name="incy" label="INCY">
                      <Input.TextArea rows={5} />
                    </FormField>
                  </>
                ),
              },
            ]}
          />
        </Form>
      </FormProvider>
      <Suspense fallback={null}>
        {editingConfig !== null && configDraft && (
          <ConfigurationEditor
            open
            mode="add"
            dbInbound={configDraft}
            dbInbounds={[]}
            availableNodes={nodes}
            availableNodesFetched
            onSaved={() => {}}
            onClose={() => setEditingConfig(null)}
            onDraft={(payload) => {
              const index = editingConfig;
              form.setValue(`entries.${index}.preset`, 'custom-config');
              form.setValue(`entries.${index}.label`, payload.remark);
              form.setValue(`entries.${index}.config`, {
                protocol: payload.protocol,
                shareAddrStrategy: payload.shareAddrStrategy,
                shareAddr: payload.shareAddr,
                subSortIndex: payload.subSortIndex,
                settings:
                  typeof payload.settings === 'string'
                    ? payload.settings
                    : JSON.stringify(payload.settings),
                streamSettings:
                  typeof payload.streamSettings === 'string'
                    ? payload.streamSettings
                    : JSON.stringify(payload.streamSettings ?? {}),
                sniffing:
                  typeof payload.sniffing === 'string'
                    ? payload.sniffing
                    : JSON.stringify(payload.sniffing ?? {}),
                listen: payload.listen ?? '',
                disableFlow: payload.disableFlow ?? false,
              });
              form.setValue(`entries.${index}.nodeId`, payload.nodeId ?? 0);
              // A malformed draft must not crash the entire subscription editor.
              try {
                const stream =
                  typeof payload.streamSettings === 'string'
                    ? JSON.parse(payload.streamSettings || '{}')
                    : payload.streamSettings;
                if (stream?.security !== 'reality') form.setValue(`entries.${index}.sni`, '');
              } catch {
                // Preserve SNI until the malformed settings are corrected.
              }
            }}
          />
        )}
      </Suspense>
    </Modal>
  );
}
