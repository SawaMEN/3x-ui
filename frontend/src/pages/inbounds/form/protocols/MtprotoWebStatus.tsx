import { useEffect, useState } from 'react';
import { Alert, Button, Space, Typography } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import { TelemtWebProxyRequestSchema } from '@/schemas/telemt-web';

type WebStatus = {
  defaultDomain?: string;
  supported: boolean;
  port443Available: boolean;
  port443Free?: boolean;
  frontendActive?: boolean;
  port443Owner?: string;
  error?: string;
};

type WebPreflight = {
  ready: boolean;
  checks: { id: string; ok: boolean; message: string }[];
  warnings: string[];
};

export default function MtprotoWebStatus() {
  const { control, getValues, setValue } = useFormContext();
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const [status, setStatus] = useState<WebStatus>();
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const proposed = useWatch({ control, name: 'settings.webProxy' });
  const requestKey = JSON.stringify({
    domain: proposed?.domain ?? '',
    frontend: proposed?.frontend ?? 'nginx',
    carrier: proposed?.carrier ?? 'https',
    carriers: proposed?.carriers ?? [],
    site: proposed?.site,
  });
  const [preflight, setPreflight] = useState<{ key: string; result: WebPreflight }>();
  const result = preflight?.key === requestKey ? preflight.result : undefined;

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      try {
        const response = await HttpUtil.post<WebPreflight>(
          '/panel/api/telemt/webproxy/preflight',
          JSON.parse(requestKey),
          { silent: true, signal: controller.signal },
        );
        if (!active) return;
        if (response?.success && response.obj) {
          setPreflight({ key: requestKey, result: response.obj });
        } else {
          throw new Error(
            response?.msg || (ru ? 'Ошибка проверки WEB Proxy' : 'WEB Proxy preflight failed'),
          );
        }
      } catch (cause: unknown) {
        if (active)
          setPreflight({
            key: requestKey,
            result: {
              ready: false,
              checks: [
                {
                  id: 'request',
                  ok: false,
                  message: cause instanceof Error ? cause.message : String(cause),
                },
              ],
              warnings: [],
            },
          });
      }
    }, 400);
    return () => {
      active = false;
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [requestKey, attempt, ru]);

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
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
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
      : status?.port443Available
        ? status.error
        : undefined);
  const defaultDomain = TelemtWebProxyRequestSchema.shape.domain.safeParse(status?.defaultDomain);
  return (
    <Space orientation="vertical" style={{ width: '100%', marginBlock: 12 }}>
      {problem && <Alert type="warning" showIcon title={problem} />}
      {!loading && !error && status && (
        <>
          <Typography.Text>
            {ru ? 'Домен по умолчанию: ' : 'Default domain: '}
            {defaultDomain.success
              ? defaultDomain.data
              : ru
                ? 'не определён — укажите публичный домен вручную'
                : 'unavailable — enter a public domain manually'}
          </Typography.Text>
          {defaultDomain.success && (
            <Button
              size="small"
              onClick={() =>
                setValue('settings.webProxy.domain', defaultDomain.data, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
            >
              {ru ? 'Использовать домен по умолчанию' : 'Use default domain'}
            </Button>
          )}
          {status.port443Available && (
            <Alert
              type="success"
              showIcon
              title={
                status.port443Free === true
                  ? ru
                    ? 'TCP 443 свободен'
                    : 'TCP 443 is free'
                  : status.frontendActive
                    ? ru
                      ? 'TCP 443 используется действующим WEB Proxy'
                      : 'TCP 443 is used by the active WEB Proxy'
                    : ru
                      ? 'TCP 443 доступен для настройки WEB Proxy'
                      : 'TCP 443 is available for WEB Proxy setup'
              }
              description={
                ru
                  ? 'Проверяется локальный TCP-порт сервера. Внешняя доступность зависит от DNS и firewall сервера и хостера.'
                  : 'Checks the server’s local TCP listener. External reachability depends on DNS and host/provider firewalls.'
              }
            />
          )}
        </>
      )}
      {!loading && !error && status && !status.port443Available && (
        <Alert
          type="warning"
          showIcon
          title={
            ru
              ? status.port443Free === true
                ? `TCP 443 зарезервирован в панели: ${status.port443Owner || 'другое подключение'}. Измените его порт перед сохранением WEB Proxy.`
                : `TCP 443 занят: ${status.port443Owner || 'другой сервис'}. Перенесите этот сервис на другой порт перед сохранением WEB Proxy.`
              : status.port443Free === true
                ? `TCP 443 is reserved in the panel: ${status.port443Owner || 'another listener'}. Change its port before saving WEB Proxy.`
                : `TCP 443 is occupied: ${status.port443Owner || 'another service'}. Move it to another port before saving WEB Proxy.`
          }
        />
      )}
      {result && (
        <Alert
          type={result.ready ? 'success' : 'warning'}
          showIcon
          title={
            result.ready
              ? ru
                ? 'Параметры WEB Proxy прошли проверку'
                : 'Proposed WEB Proxy settings passed preflight'
              : ru
                ? 'Параметры WEB Proxy требуют исправления'
                : 'Proposed WEB Proxy settings need attention'
          }
          description={
            <ul>
              {result.checks.map((check) => (
                <li key={check.id}>{check.message}</li>
              ))}
              {result.warnings.map((warning) => (
                <li key={warning}>{warning}</li>
              ))}
            </ul>
          }
        />
      )}
      {!result && (
        <Typography.Text type="secondary">
          {ru
            ? 'Проверяются введённые параметры WEB Proxy…'
            : 'Checking proposed WEB Proxy settings…'}
        </Typography.Text>
      )}
      <Button
        loading={loading}
        size="small"
        onClick={() => {
          setLoading(true);
          setPreflight(undefined);
          setAttempt((value) => value + 1);
        }}
      >
        {ru ? 'Проверить готовность WEB Proxy' : 'Check WEB Proxy readiness'}
      </Button>
    </Space>
  );
}
