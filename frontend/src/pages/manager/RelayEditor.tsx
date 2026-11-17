import { useState } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { Alert, Form, Input, InputNumber, Modal, Select, Switch } from 'antd';
import { FormField } from '@/components/form/rhf';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { RelaySchema } from './schema';
import type { Relay } from './schema';
export default function RelayEditor({
  value,
  onClose,
  onSave,
}: {
  value: Relay;
  onClose: () => void;
  onSave: (v: Relay) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const m = (k: string) => t('pages.manager.' + k);
  const form = useForm<Relay>({ defaultValues: { ...value, credential: '' } });
  const { nodes } = useNodesQuery();
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const save = async () => {
    const r = RelaySchema.safeParse(form.getValues());
    if (!r.success) {
      setError(r.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`).join('\n'));
      return;
    }
    setSaving(true);
    try {
      if (await onSave(r.data)) onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal
      open
      title={m('editRelay')}
      onCancel={onClose}
      confirmLoading={saving}
      onOk={() => void save()}
    >
      <FormProvider {...form}>
        <Form layout="vertical" component="div">
          <Alert type="info" title={m('relayHint')} />
          {error && <Alert type="error" title={error} />}
          {['name', 'address', 'username', 'hostKey'].map((key) => (
            <FormField
              name={key}
              key={key}
              label={m(key)}
              extra={key === 'hostKey' ? m('hostKeyHint') : undefined}
            >
              <Input autoComplete="off" />
            </FormField>
          ))}
          <FormField name="sshPort" label={m('sshPort')}>
            <InputNumber min={1} max={65535} />
          </FormField>
          <FormField name="nodeId" label={m('node')}>
            <Select options={nodes.map((n) => ({ value: n.id, label: n.name }))} />
          </FormField>
          <FormField name="keyAuth" label={m('keyAuth')} valueProp="checked">
            <Switch />
          </FormField>
          <FormField name="credential" label={m('credential')} extra={m('credentialHint')}>
            <Input.TextArea rows={3} autoComplete="off" />
          </FormField>
        </Form>
      </FormProvider>
    </Modal>
  );
}
