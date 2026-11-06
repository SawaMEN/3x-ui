import { useState } from 'react';
import { FormProvider } from 'react-hook-form';
import { z } from 'zod';
import { Alert, Button, Card, Col, Form, Input, Modal, Row, Select, Space, Table } from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  DeleteOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { FormField, useZodForm } from '@/components/form/rhf';
import { object, records, type ConfigObject } from './simple-settings';
import { routeRuleSummary } from './route-rule';

const Schema = z.object({
  domain: z.string(),
  domain_suffix: z.string(),
  server: z.string(),
  action: z.string(),
  query_type: z.array(z.string()),
});
const tokens = (value: string) => value.split(/[\s,]+/).filter(Boolean);
const list = (value: unknown): string[] =>
  (Array.isArray(value) ? value : value == null ? [] : [value]).map(String);

function RuleEditor({
  rule,
  servers,
  onSave,
  onCancel,
}: {
  rule: ConfigObject;
  servers: string[];
  onSave: (rule: ConfigObject) => void;
  onCancel: () => void;
}) {
  const form = useZodForm(Schema, {
    defaultValues: {
      domain: list(rule.domain).join('\n'),
      domain_suffix: list(rule.domain_suffix).join('\n'),
      server: String(rule.server || ''),
      action: String(rule.action || 'route'),
      query_type: list(rule.query_type),
    },
  });
  const [error, setError] = useState('');
  const action = form.watch('action');
  return (
    <Modal
      open
      width={720}
      title="Правило DNS"
      okText="Сохранить"
      cancelText="Отмена"
      onCancel={onCancel}
      onOk={form.handleSubmit((values) => {
        if (values.action === 'route' && !values.server) {
          setError('Выберите DNS-сервер.');
          return;
        }
        const next: ConfigObject = { ...rule, action: values.action };
        for (const key of ['domain', 'domain_suffix'] as const) {
          const entries = tokens(values[key]);
          if (entries.length) next[key] = entries;
          else delete next[key];
        }
        if (values.query_type.length) {
          if (values.query_type.join(',') !== list(rule.query_type).join(','))
            next.query_type = values.query_type;
        } else delete next.query_type;
        if (values.action === 'route') next.server = values.server;
        else delete next.server;
        onSave(next);
      })}
    >
      {error && <Alert type="error" title={error} style={{ marginBottom: 12 }} />}
      <FormProvider {...form}>
        <Form layout="vertical">
          <Row gutter={16}>
            <Col xs={24} sm={12}>
              <FormField name="action" label="Действие">
                <Select
                  options={[
                    { value: 'route', label: 'Запросить DNS-сервер' },
                    { value: 'reject', label: 'Отклонить запрос' },
                  ]}
                />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField name="server" label="DNS-сервер">
                <Select
                  disabled={action !== 'route'}
                  options={servers.map((value) => ({ value, label: value }))}
                />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField name="domain" label="Точные домены">
                <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} placeholder="example.com" />
              </FormField>
            </Col>
            <Col xs={24} sm={12}>
              <FormField name="domain_suffix" label="Домены и поддомены">
                <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} placeholder="example.com" />
              </FormField>
            </Col>
            <Col xs={24}>
              <FormField name="query_type" label="Типы запросов">
                <Select
                  mode="multiple"
                  options={['A', 'AAAA', 'HTTPS', 'SVCB', 'TXT', 'MX', 'PTR'].map((value) => ({
                    value,
                    label: value,
                  }))}
                />
              </FormField>
            </Col>
          </Row>
        </Form>
      </FormProvider>
    </Modal>
  );
}

export default function NativeDnsRules({
  config,
  onChange,
}: {
  config: ConfigObject;
  onChange: (config: ConfigObject) => void;
}) {
  const dns = object(config.dns),
    rules = records(dns.rules),
    servers = records(dns.servers)
      .map((item) => String(item.tag || ''))
      .filter(Boolean);
  const [editor, setEditor] = useState<{ index: number | null; rule: ConfigObject } | null>(null);
  const [modal, holder] = Modal.useModal();
  const update = (next: ConfigObject[]) => onChange({ ...config, dns: { ...dns, rules: next } });
  const move = (index: number, delta: number) => {
    const next = [...rules];
    [next[index], next[index + delta]] = [next[index + delta], next[index]];
    update(next);
  };
  return (
    <Card
      title="Правила DNS"
      extra={
        <Button
          type="primary"
          icon={<PlusOutlined />}
          aria-label="Добавить правило"
          onClick={() =>
            setEditor({
              index: null,
              rule: { action: 'route', server: String(dns.final || servers[0] || '') },
            })
          }
        >
          Добавить правило
        </Button>
      }
    >
      {holder}
      <p className="singbox-description">
        Правила проверяются сверху вниз. Если совпадений нет, используется DNS по умолчанию.
      </p>
      <Table
        size="small"
        pagination={false}
        scroll={{ x: 650 }}
        rowKey={(_, index) => String(index)}
        dataSource={rules}
        columns={[
          { title: '#', render: (_, __, index) => index + 1, width: 45 },
          {
            title: 'Условия',
            render: (_, row: ConfigObject) =>
              [routeRuleSummary(row), list(row.query_type).join(', ')].filter(Boolean).join(' · '),
          },
          {
            title: 'Назначение',
            render: (_, row: ConfigObject) => String(row.server || row.action || '—'),
          },
          {
            title: 'Действия',
            render: (_, row: ConfigObject, index: number) => (
              <Space>
                <Button
                  size="small"
                  onClick={() => {
                    if (
                      (row.type && row.type !== 'default') ||
                      !['route', 'reject', ''].includes(String(row.action || ''))
                    ) {
                      modal.info({
                        title: 'Расширенное правило DNS',
                        content:
                          'Это правило использует специальные параметры. Его можно изменить во вкладке «Дополнительно» в редакторе DNS rules.',
                      });
                      return;
                    }
                    setEditor({ index, rule: row });
                  }}
                >
                  Изменить
                </Button>
                <Button
                  size="small"
                  title="Выше DNS"
                  icon={<ArrowUpOutlined />}
                  disabled={!index}
                  onClick={() => move(index, -1)}
                />
                <Button
                  size="small"
                  title="Ниже DNS"
                  icon={<ArrowDownOutlined />}
                  disabled={index === rules.length - 1}
                  onClick={() => move(index, 1)}
                />
                <Button
                  size="small"
                  danger
                  icon={<DeleteOutlined />}
                  aria-label={`Удалить правило DNS ${index + 1}`}
                  onClick={() =>
                    modal.confirm({
                      title: 'Удалить правило DNS?',
                      okText: 'Удалить',
                      cancelText: 'Отмена',
                      okButtonProps: { danger: true },
                      onOk: () => update(rules.filter((_, i) => i !== index)),
                    })
                  }
                />
              </Space>
            ),
          },
        ]}
      />
      {editor && (
        <RuleEditor
          rule={editor.rule}
          servers={servers}
          onCancel={() => setEditor(null)}
          onSave={(rule) => {
            const next = [...rules];
            if (editor.index == null) next.push(rule);
            else next[editor.index] = rule;
            update(next);
            setEditor(null);
          }}
        />
      )}
    </Card>
  );
}
