import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Modal, Popconfirm, Space, Table, Typography, message } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';

import { useProxyPresetMutations } from '@/api/queries/useProxyPresets';
import {
  ProxyPresetConfigSchema,
  ProxyPresetInputSchema,
  type ProxyPresetView,
} from '@/schemas/api/proxyPreset';

interface Props {
  open: boolean;
  presets: ProxyPresetView[];
  loading?: boolean;
  onOpenChange: (open: boolean) => void;
}

type EditorForm = {
  name: string;
  description: string;
  configJson: string;
};

const EMPTY_CONFIG = '{\n  "security": "tls"\n}';

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export default function ProxyPresetManagerModal({ open, presets, loading, onOpenChange }: Props) {
  const { t } = useTranslation();
  const [messageApi, contextHolder] = message.useMessage();
  const [form] = Form.useForm<EditorForm>();
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<ProxyPresetView | null>(null);
  const { save, update, remove, saving } = useProxyPresetMutations();

  useEffect(() => {
    if (!editorOpen) return;
    form.setFieldsValue({
      name: editing?.name ?? '',
      description: editing?.description ?? '',
      configJson: editing ? JSON.stringify(editing.config, null, 2) : EMPTY_CONFIG,
    });
  }, [editorOpen, editing, form]);

  const openCreate = () => {
    setEditing(null);
    setEditorOpen(true);
  };

  const openEdit = (preset: ProxyPresetView) => {
    setEditing(preset);
    setEditorOpen(true);
  };

  const submit = async () => {
    const values = form.getFieldsValue();
    form.setFields([
      { name: 'name', errors: [] },
      { name: 'description', errors: [] },
      { name: 'configJson', errors: [] },
    ]);

    let raw: unknown;
    try {
      raw = JSON.parse(values.configJson ?? '');
    } catch {
      form.setFields([{ name: 'configJson', errors: ['Invalid JSON'] }]);
      return;
    }
    const configResult = ProxyPresetConfigSchema.safeParse(raw);
    if (!configResult.success) {
      form.setFields([
        {
          name: 'configJson',
          errors: [configResult.error.issues.map((issue) => issue.message).join('; ')],
        },
      ]);
      return;
    }

    const inputResult = ProxyPresetInputSchema.safeParse({
      name: values.name,
      description: values.description ?? '',
      config: configResult.data,
    });
    if (!inputResult.success) {
      const fields = new Map<'name' | 'description', string[]>();
      for (const issue of inputResult.error.issues) {
        const field = issue.path[0];
        if (field !== 'name' && field !== 'description') continue;
        const messages = fields.get(field) ?? [];
        messages.push(issue.message);
        fields.set(field, messages);
      }
      form.setFields(
        [...fields.entries()].map(([name, errors]) => ({
          name,
          errors,
        })),
      );
      return;
    }

    try {
      if (editing) await update(editing.id, inputResult.data);
      else await save(inputResult.data);
      messageApi.success(t('success'));
      setEditorOpen(false);
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };

  const deletePreset = async (preset: ProxyPresetView) => {
    try {
      await remove(preset.id);
      messageApi.success(t('success'));
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };

  return (
    <>
      {contextHolder}
      <Modal
        open={open}
        title="Proxy presets"
        width={760}
        footer={null}
        onCancel={() => onOpenChange(false)}
      >
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          <Typography.Text type="secondary">
            Presets override only fields present in their JSON config. Host addresses, remarks and inbound bindings remain owned by the Host group.
          </Typography.Text>
          <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
            {t('add')}
          </Button>
          <Table<ProxyPresetView>
            rowKey="id"
            size="small"
            loading={loading}
            dataSource={presets}
            pagination={false}
            columns={[
              { title: 'Name', dataIndex: 'name', key: 'name' },
              { title: t('comment'), dataIndex: 'description', key: 'description' },
              {
                title: t('edit'),
                key: 'actions',
                width: 104,
                render: (_, preset) => (
                  <Space size={2}>
                    <Button
                      type="text"
                      size="small"
                      icon={<EditOutlined />}
                      aria-label={t('edit')}
                      onClick={() => openEdit(preset)}
                    />
                    <Popconfirm
                      title={t('sure')}
                      okText={t('delete')}
                      okButtonProps={{ danger: true }}
                      onConfirm={() => deletePreset(preset)}
                    >
                      <Button
                        type="text"
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        aria-label={t('delete')}
                      />
                    </Popconfirm>
                  </Space>
                ),
              },
            ]}
          />
        </Space>
      </Modal>

      <Modal
        open={editorOpen}
        title={editing ? t('edit') : t('add')}
        okText={t('save')}
        cancelText={t('cancel')}
        confirmLoading={saving}
        onOk={submit}
        onCancel={() => setEditorOpen(false)}
        width={720}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Name">
            <Input autoComplete="off" maxLength={120} />
          </Form.Item>
          <Form.Item name="description" label={t('comment')}>
            <Input maxLength={1000} />
          </Form.Item>
          <Form.Item
            name="configJson"
            label={t('jsonEditor')}
            extra="Examples: security, sni, hostHeader, path, alpn, fingerprint, allowInsecure, echConfigList, muxParams, sockoptParams, finalMask. Omit a field to inherit it from Host."
          >
            <Input.TextArea rows={16} spellCheck={false} style={{ fontFamily: 'monospace' }} />
          </Form.Item>
        </Form>
      </Modal>
    </>
  );
}
