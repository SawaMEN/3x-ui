import { useEffect, useState } from 'react';
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
import { ClientOptionsSchema, newEntry, SubscriptionSchema } from './schema';
import type { Relay, Subscription } from './schema';

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
  const { t } = useTranslation();
  const m = (k: string) => t('pages.manager.' + k);
  const form = useForm<Subscription>({ defaultValues: value });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'entries' });
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
  const submit = async (generate: boolean) => {
    const parsed = SubscriptionSchema.safeParse(form.getValues());
    if (!parsed.success) {
      setError(parsed.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`).join('\n'));
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
      width={900}
      onCancel={onClose}
      footer={
        <Space wrap>
          <Button onClick={onClose}>{t('cancel')}</Button>
          <Button loading={saving} onClick={() => void submit(false)}>
            {t('save')}
          </Button>
          <Button type="primary" loading={saving} onClick={() => void submit(true)}>
            {m('saveGenerate')}
          </Button>
        </Space>
      }
    >
      <FormProvider {...form}>
        <Form layout="vertical" component="div">
          {error && <Alert type="error" showIcon title={error} />}
          <Tabs
            items={[
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
                label: `${m('connections')} (${fields.length}/20)`,
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
                                    form.setValue(`entries.${i}.nodeId`, undefined);
                                    form.setValue(`entries.${i}.sni`, '');
                                    form.setValue(
                                      `entries.${i}.port`,
                                      mode === 'existing' ? '' : 'random',
                                    );
                                    form.setValue(`entries.${i}.customLink`, '');
                                  }}
                                >
                                  <Select
                                    options={['rotate', 'existing', 'custom'].map((v) => ({
                                      value: v,
                                      label: m(v),
                                    }))}
                                  />
                                </FormField>
                              </Col>
                              <Col xs={24} sm={16}>
                                {e?.mode === 'custom' ? (
                                  <FormField
                                    name={`entries.${i}.customLink`}
                                    label={m('shareLink')}
                                  >
                                    <Input.TextArea autoSize />
                                  </FormField>
                                ) : (
                                  <FormField name={`entries.${i}.templateId`} label={m('template')}>
                                    <Select
                                      showSearch
                                      optionFilterProp="label"
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
                            {e?.mode !== 'custom' && (
                              <Row gutter={12}>
                                {e?.mode === 'rotate' && (
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
                                                  !template ||
                                                  nodeSupportsProtocol(n, template.protocol ?? ''),
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
                        disabled={fields.length >= 20}
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
    </Modal>
  );
}
