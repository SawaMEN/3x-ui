import { useEffect, useState } from 'react';
import { FormProvider } from 'react-hook-form';
import {
  Alert,
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
  Table,
  Tag,
  message,
} from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { FormField, useZodForm } from '@/components/form/rhf';
import { object, records, type ConfigObject } from './simple-settings';
import {
  DnsServerFormSchema,
  dnsServerForm,
  buildDnsServer,
  putDnsServer,
  removeDnsServer,
} from './dns-editor';

const types = ['local', 'udp', 'tcp', 'tls', 'quic', 'https', 'h3'];

function DnsEditor({
  value,
  config,
  index,
  onSave,
  onCancel,
  tags,
}: {
  value: ConfigObject;
  config: ConfigObject;
  index: number | null;
  onSave: (config: ConfigObject) => void;
  onCancel: () => void;
  tags: string[];
}) {
  const form = useZodForm(DnsServerFormSchema, { defaultValues: dnsServerForm(value) });
  const type = form.watch('type');
  const network = ['udp', 'tcp', 'tls', 'quic', 'https', 'h3'].includes(type);
  const encrypted = ['tls', 'quic', 'https', 'h3'].includes(type);
  const [error, setError] = useState('');
  useEffect(() => {
    form.reset(dnsServerForm(value));
  }, [value, form]);
  return (
    <Modal
      open
      width={720}
      title={index == null ? 'Добавить DNS-сервер' : 'Изменить DNS-сервер'}
      okText="Сохранить"
      cancelText="Отмена"
      onCancel={onCancel}
      onOk={form.handleSubmit((data) => {
        try {
          onSave(putDnsServer(config, index, buildDnsServer(value, data)));
        } catch (error) {
          setError(error instanceof Error ? error.message : String(error));
        }
      })}
    >
      {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 12 }} />}
      <FormProvider {...form}>
        <Form layout="vertical">
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <FormField name="tag" label="Название" required>
                <Input placeholder="dns-primary" />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField
                name="type"
                label="Тип"
                onAfterChange={() => {
                  form.setValue('server_port', undefined);
                  form.setValue('path', '');
                  form.setValue('server_name', '');
                }}
              >
                <Select
                  options={[...types, ...(!types.includes(type) ? [type] : [])].map((value) => ({
                    value,
                    label: value,
                  }))}
                />
              </FormField>
            </Col>
            {network && (
              <>
                <Col xs={24} sm={16}>
                  <FormField name="server" label="Адрес сервера" required>
                    <Input placeholder="1.1.1.1 или https://dns.example/dns-query" />
                  </FormField>
                </Col>
                <Col xs={24} sm={8}>
                  <FormField
                    name="server_port"
                    label="Порт"
                    transform={{ output: (value) => value ?? undefined }}
                  >
                    <InputNumber
                      min={1}
                      max={65535}
                      style={{ width: '100%' }}
                      placeholder={
                        encrypted ? (type === 'https' || type === 'h3' ? '443' : '853') : '53'
                      }
                    />
                  </FormField>
                </Col>
              </>
            )}
            {encrypted && (
              <Col xs={24}>
                <FormField name="server_name" label="Имя сервера TLS (SNI)">
                  <Input placeholder="dns.example" />
                </FormField>
              </Col>
            )}
            {['https', 'h3'].includes(type) && (
              <Col xs={24}>
                <FormField name="path" label="Путь DNS через HTTPS">
                  <Input placeholder="/dns-query" />
                </FormField>
              </Col>
            )}
            <Col xs={24}>
              <FormField
                name="detour"
                label="Исходящее подключение"
                transform={{ output: (value) => value ?? '' }}
              >
                <Select allowClear options={tags.map((value) => ({ value, label: value }))} />
              </FormField>
            </Col>
            {!network && !['local'].includes(type) && (
              <Col xs={24}>
                <Alert
                  type="info"
                  title="Специальные параметры этого типа сохраняются. Для их изменения используйте расширенный DNS."
                />
              </Col>
            )}
          </Row>
        </Form>
      </FormProvider>
    </Modal>
  );
}

export default function NativeDnsServers({
  config,
  onChange,
  subscriptionTags = [],
}: {
  config: ConfigObject;
  onChange: (config: ConfigObject) => void;
  subscriptionTags?: string[];
}) {
  const [editor, setEditor] = useState<{ index: number | null; value: ConfigObject } | null>(null);
  const [api, holder] = message.useMessage();
  const [modal, modalHolder] = Modal.useModal();
  const dns = object(config.dns),
    servers = records(dns.servers);
  const tags = [
    ...new Set(
      [...records(config.outbounds), ...records(config.endpoints)]
        .map((item) => String(item.tag || ''))
        .filter(Boolean)
        .concat(subscriptionTags),
    ),
  ];
  return (
    <Card
      title="DNS-серверы"
      extra={
        <Button
          type="primary"
          icon={<PlusOutlined />}
          aria-label="Добавить DNS"
          onClick={() => {
            let suffix = servers.length + 1;
            while (servers.some((item) => item.tag === `dns-${suffix}`)) suffix++;
            setEditor({ index: null, value: { type: 'local', tag: `dns-${suffix}` } });
          }}
        >
          Добавить DNS
        </Button>
      }
    >
      {holder}
      {modalHolder}
      <Table
        size="small"
        pagination={false}
        rowKey={(row) => String(row.tag)}
        scroll={{ x: 650 }}
        dataSource={servers}
        columns={[
          {
            title: 'Название',
            render: (_, row: ConfigObject) => (
              <Space>
                <Tag>{String(row.tag || '—')}</Tag>
                {dns.final === row.tag && <Tag color="blue">По умолчанию</Tag>}
              </Space>
            ),
          },
          { title: 'Тип', dataIndex: 'type' },
          {
            title: 'Адрес',
            render: (_, row: ConfigObject) =>
              String(row.server || 'Система') + (row.server_port ? `:${row.server_port}` : ''),
          },
          { title: 'Выход', dataIndex: 'detour' },
          {
            title: 'Действия',
            render: (_, row: ConfigObject, index: number) => (
              <Space>
                <Button size="small" onClick={() => setEditor({ index, value: row })}>
                  Изменить
                </Button>
                <Button
                  size="small"
                  disabled={dns.final === row.tag}
                  onClick={() => onChange({ ...config, dns: { ...dns, final: row.tag } })}
                >
                  По умолчанию
                </Button>
                <Button
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  aria-label={`Удалить DNS ${row.tag}`}
                  onClick={() => {
                    try {
                      const next = removeDnsServer(config, index);
                      modal.confirm({
                        title: `Удалить DNS ${row.tag}?`,
                        okText: 'Удалить',
                        cancelText: 'Отмена',
                        okButtonProps: { danger: true },
                        onOk: () => onChange(next),
                      });
                    } catch (error) {
                      api.error(error instanceof Error ? error.message : String(error));
                    }
                  }}
                />
              </Space>
            ),
          },
        ]}
      />
      {editor && (
        <DnsEditor
          value={editor.value}
          config={config}
          index={editor.index}
          tags={tags}
          onCancel={() => setEditor(null)}
          onSave={(next) => {
            onChange(next);
            setEditor(null);
          }}
        />
      )}
    </Card>
  );
}
