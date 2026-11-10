import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Input, InputNumber, Modal, Select, Space, Typography } from 'antd';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { isClientAttachableProtocol } from '@/lib/inbounds/client-attachable';
import { HttpUtil } from '@/utils';

interface Props {
  open: boolean;
  onClose: () => void;
  onSuccess: (created: number) => void;
}

export default function BulkSharedModal({ open, onClose, onSuccess }: Props) {
  const { i18n } = useTranslation();
  const russian = i18n.language.startsWith('ru');
  const t = (ru: string, en: string) => (russian ? ru : en);
  const { data: inbounds = [], isFetching, error } = useInboundOptions();
  const [prefix, setPrefix] = useState('');
  const [count, setCount] = useState(5);
  const [ids, setIDs] = useState<number[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState('');
  const options = inbounds
    .filter(
      (ib) =>
        ib.enable !== false &&
        ib.protocol &&
        isClientAttachableProtocol(ib.protocol) &&
        !ib.tag?.includes('manager-'),
    )
    .map((ib) => ({
      value: ib.id,
      label: `${ib.remark || ib.tag || ib.protocol} · ${ib.protocol} :${ib.port || '?'} · ${ib.nodeId ? `#${ib.nodeId}` : t('Локальный', 'Local')}`,
    }));
  const submit = async () => {
    if (!prefix.trim() || ids.length < 1 || count < 1 || count > 100 || submitting) return;
    setSubmitting(true);
    setMessage('');
    try {
      const result = await HttpUtil.post(
        '/panel/api/manager/subscriptions/bulk-shared',
        { namePrefix: prefix.trim(), count, inboundIds: ids },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result?.success) {
        setMessage(result?.msg || t('Не удалось создать подписки', 'Failed to create subscriptions'));
        return;
      }
      const payload = result.obj as { created?: number } | undefined;
      onSuccess(payload?.created ?? count);
      setPrefix('');
      setIDs([]);
      onClose();
    } catch (err) {
      setMessage(String(err));
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <Modal
      open={open}
      onCancel={onClose}
      title={t('Массовое создание клиентов на общих inbound', 'Bulk clients on shared inbounds')}
      okText={t('Создать и запустить', 'Create and queue')}
      okButtonProps={{ disabled: !prefix.trim() || ids.length === 0 || count < 1 || count > 100 }}
      confirmLoading={submitting}
      onOk={() => void submit()}
      destroyOnHidden
    >
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title={t(
            'Каждому клиенту — собственная подписка и учётные данные.',
            'Every client gets a unique subscription and credentials.',
          )}
          description={t(
            'Выбранные inbound используются совместно: новые порты не создаются. Запуск ставится в очередь; прогресс виден в журнале операций. Удаление подписки не удаляет общий inbound.',
            'Existing inbounds are shared without creating new listeners. Deployment is queued; progress is visible in operations. Deleting a subscription never deletes the shared inbound.',
          )}
        />
        <div>
          <Typography.Text>{t('Название клиентов', 'Client name prefix')}</Typography.Text>
          <Input
            value={prefix}
            maxLength={220}
            onChange={(e) => setPrefix(e.target.value)}
            placeholder={t('Например: Пользователь', 'Example: Member')}
          />
        </div>
        <div>
          <Typography.Text>{t('Количество клиентов (1–100)', 'Number of clients (1–100)')}</Typography.Text>
          <InputNumber min={1} max={100} value={count} onChange={(v) => setCount(v ?? 1)}
            style={{ width: '100%' }}
          />
        </div>
        <div>
          <Typography.Text>{t('Общие подключения (одинаковые для всех клиентов)', 'Shared inbounds for every client')}</Typography.Text>
          <Select
            mode="multiple"
            value={ids}
            onChange={setIDs}
            loading={isFetching}
            showSearch={{ optionFilterProp: 'label' }}
            placeholder={t('Выберите готовые inbound', 'Choose existing inbounds')}
            options={options}
            style={{ width: '100%' }}
            maxTagCount="responsive"
          />
        </div>
        {options.length === 0 && !isFetching && (
          <Alert
            type="warning"
            title={t(
              'Сначала создайте общий inbound в разделе «Подключения».',
              'Create a shared inbound in the Inbounds section first.',
            )}
            showIcon
          />
        )}
        {error && <Alert type="error" title={String(error)} showIcon />}
        {message && <Alert type="error" title={message} showIcon />}
      </Space>
    </Modal>
  );
}
