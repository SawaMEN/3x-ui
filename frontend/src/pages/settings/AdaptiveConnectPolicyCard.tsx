import { useState } from 'react';
import { Alert, Button, Card, Form, InputNumber, Space, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import {
  DEFAULT_ADAPTIVE_POLICY,
  exportAdaptivePolicy,
  type AdaptiveConnectPolicy,
} from './adaptive-connect-policy';

export default function AdaptiveConnectPolicyCard() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => ru ? russian : en;
  const [policy, setPolicy] = useState<AdaptiveConnectPolicy>({ ...DEFAULT_ADAPTIVE_POLICY });
  const [error, setError] = useState('');
  const update = <K extends keyof AdaptiveConnectPolicy>(key: K, value: AdaptiveConnectPolicy[K]) => {
    setError('');
    setPolicy((prev) => ({ ...prev, [key]: value }));
  };
  const fields = [
    { key: 'failureThreshold', label: l('Failures before switching', 'Ошибок до переключения'), min: 1, max: 10 },
    { key: 'recoveryThreshold', label: l('Successful checks required', 'Успешных проверок для резерва'), min: 1, max: 10 },
    { key: 'cooldownSeconds', label: l('Switch cooldown, s', 'Пауза между переключениями, с'), min: 0, max: 300 },
    { key: 'maxAgeSeconds', label: l('Maximum observation age, s', 'Срок годности измерений, с'), min: 5, max: 600 },
  ] as const;

  function download() {
    try {
      const json = exportAdaptivePolicy(policy);
      const url = URL.createObjectURL(new Blob([json], { type: 'application/json' }));
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = 'adaptive-connect-v1.json';
      document.body.appendChild(anchor);
      try { anchor.click(); } finally {
        anchor.remove();
        window.setTimeout(() => URL.revokeObjectURL(url), 1000);
      }
    } catch {
      setError(l('Invalid policy values', 'Недопустимые значения политики'));
    }
  }

  return (
    <Card size="small" title={l('Adaptive Connect policy v1', 'Политика Adaptive Connect v1')}>
      <Alert type="info" showIcon title={l(
        'Experimental export for VetrOFF-Core. This file does not modify subscriptions, save server settings, or activate the runtime. Local user consent is required.',
        'Экспериментальный экспорт для VetrOFF-Core. Файл не изменяет подписки, не сохраняет настройки сервера и не включает ядро. Требуется согласие пользователя.',
      )} />
      <Form layout="vertical" style={{ marginTop: 16 }}>
        <Form.Item label={l('Request automatic mode', 'Запросить автоматический режим')}>
          <Switch checked={policy.enabled} onChange={(v) => update('enabled', v)} />
        </Form.Item>
        {fields.map((field) => (
          <Form.Item key={field.key} label={field.label}>
            <InputNumber min={field.min} max={field.max} precision={0} value={policy[field.key]}
              onChange={(value) => { if (typeof value === 'number') update(field.key, value); }}
              style={{ width: '100%' }} />
          </Form.Item>
        ))}
        <Form.Item label={l('Return to preferred connection', 'Возврат к предпочтительному подключению')}>
          <Switch checked={policy.failback} onChange={(v) => update('failback', v)} />
        </Form.Item>
      </Form>
      {error && <Alert type="error" title={error} />}
      <Space wrap>
        <Button type="primary" onClick={download}>{l('Export policy JSON', 'Экспортировать JSON')}</Button>
        <Button onClick={() => { setPolicy({ ...DEFAULT_ADAPTIVE_POLICY }); setError(''); }}>{l('Reset defaults', 'Сбросить настройки')}</Button>
      </Space>
    </Card>
  );
}
