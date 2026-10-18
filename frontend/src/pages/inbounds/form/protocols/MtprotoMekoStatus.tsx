import { useCallback, useEffect, useState } from 'react';
import { Alert, Button } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { HttpUtil } from '@/utils';

type MekoStatus = {
  enabled: boolean;
  applied: boolean;
  healthy: boolean;
  backend?: string;
  statusError?: string;
  ports: number[];
  appliedPorts: number[];
};

export default function MtprotoMekoStatus() {
  const { control } = useFormContext();
  const { i18n } = useTranslation();
  const russian = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const port = useWatch({ control, name: 'port' }) as number | undefined;
  const [status, setStatus] = useState<MekoStatus | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const response = (await HttpUtil.get('/panel/api/telemt/meko/config', undefined, {
        silent: true,
      })) as { success: boolean; msg?: string; obj?: MekoStatus };
      if (!response.success || !response.obj) {
        setError(response.msg || 'MEKO status unavailable');
        return;
      }
      setStatus(response.obj);
      setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Defer the initial network request to avoid synchronous state updates
    // while React is committing the inbound form.
    const timer = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(timer);
  }, [refresh]);

  const wanted = !!port && status?.ports?.includes(port);
  const active = !!port && status?.appliedPorts?.includes(port) && status?.applied;
  const message = error || status?.statusError;
  let type: 'success' | 'warning' | 'error' | 'info' = 'info';
  let description = russian
    ? 'После сохранения прокси проверьте применение правил к выбранному TCP-порту.'
    : 'Save the inbound, then verify the firewall rules for this TCP port.';
  if (message) {
    type = 'error';
    description = message;
  } else if (wanted && active && status?.healthy) {
    type = 'success';
    description = russian
      ? `MEKO применён к TCP-порту ${port}. Firewall: ${status.backend || 'неизвестно'}.`
      : `MEKO applied to TCP port ${port}. Firewall: ${status.backend || 'unknown'}.`;
  } else if (wanted) {
    type = 'warning';
    description = russian
      ? `Для порта ${port} MEKO выбран, но применение правил не подтверждено. Проверьте журнал x-ui.`
      : `MEKO is selected for port ${port}, but firewall rules are not confirmed. Check x-ui logs.`;
  } else if (status) {
    type = 'info';
    description = russian
      ? 'Новые настройки MEKO вступят в силу после сохранения входящего подключения.'
      : 'New MEKO settings take effect after saving the inbound.';
  }

  return (
    <Alert
      showIcon
      type={type}
      message={russian ? 'Фактический статус Fix by MEKO' : 'Fix by MEKO firewall status'}
      description={description}
      action={
        <Button size="small" loading={loading} onClick={() => void refresh()}>
          {russian ? 'Обновить' : 'Refresh'}
        </Button>
      }
    />
  );
}
