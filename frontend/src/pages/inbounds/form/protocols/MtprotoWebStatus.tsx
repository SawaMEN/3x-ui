import { useEffect, useState } from 'react';
import { Alert, Button, Space } from 'antd';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import { TelemtWebProxyRequestSchema } from '@/schemas/telemt-web';

type WebStatus = {
  defaultDomain?: string;
  supported: boolean;
  port443Available: boolean;
  port443Owner?: string;
  error?: string;
};

export default function MtprotoWebStatus() {
  const { getValues, setValue } = useFormContext();
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const [status, setStatus] = useState<WebStatus>();
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    void HttpUtil.get<WebStatus>('/panel/api/telemt/webproxy/status', undefined, { silent: true })
      .then((response) => {
        if (cancelled) return;
        if (!response.success || !response.obj) {
          setError(response.msg || 'WEB Proxy status unavailable');
          return;
        }
        setStatus(response.obj);
        setError('');
        // Never overwrite a saved domain or an edit made while the request was pending.
        const domain = TelemtWebProxyRequestSchema.shape.domain.safeParse(
          response.obj.defaultDomain,
        );
        if (!String(getValues('settings.webProxy.domain') ?? '').trim() && domain.success) {
          setValue('settings.webProxy.domain', domain.data, { shouldValidate: true });
        }
      })
      .catch((cause: unknown) => {
        if (!cancelled) setError(cause instanceof Error ? cause.message : String(cause));
      });
    return () => {
      cancelled = true;
    };
  }, [attempt, getValues, setValue]);

  const problem =
    error ||
    (!status?.supported && status
      ? ru
        ? 'Установите или обновите Telemt в Настройки → Обновления пакетов и ядер Linux.'
        : 'Install or update Telemt in Settings → Linux package and core updates.'
      : status?.error);
  return (
    <Space orientation="vertical" style={{ width: '100%', marginBlock: 12 }}>
      {problem && <Alert type="warning" showIcon message={problem} />}
      {status && !status.port443Available && (
        <Alert
          type="warning"
          showIcon
          message={
            ru
              ? `TCP 443 занят: ${status.port443Owner || 'другой сервис'}. Перенесите этот сервис на другой порт перед сохранением WEB Proxy.`
              : `TCP 443 is occupied: ${status.port443Owner || 'another service'}. Move it to another port before saving WEB Proxy.`
          }
        />
      )}
      <Button size="small" onClick={() => setAttempt((value) => value + 1)}>
        {ru ? 'Проверить готовность WEB Proxy' : 'Check WEB Proxy readiness'}
      </Button>
    </Space>
  );
}
