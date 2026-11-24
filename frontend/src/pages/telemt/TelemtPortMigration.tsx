import { useEffect, useState } from 'react';
import { Alert, Button, Modal, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import type { IngressPreview } from './TelemtIngressPreview';

type Migration = {
  id: string;
  oldPanel: number;
  newPanel: number;
  oldSubscription: number;
  newSubscription: number;
  deadline: string;
  ready: boolean;
};
const endpoint = '/panel/api/telemt/webproxy/port-migration';

export function TelemtPortMigrationButton({
  preview,
  disabled,
}: {
  preview: IngressPreview;
  disabled?: boolean;
}) {
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const [modal, holder] = Modal.useModal();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [next, setNext] = useState('');
  const panelPort = preview.manual.includes('panel-env-port') ? 0 : preview.suggestedPanelPort || 0;
  const subscriptionPort = preview.suggestedSubscriptionPort || 0;
  if (!panelPort && !subscriptionPort) return null;
  const start = () =>
    modal.confirm({
      title: ru ? 'Перенести внутренние порты?' : 'Move internal listener ports?',
      content: ru
        ? 'Панель перезапустится. Разрешите новый порт панели в firewall перед переносом. Откройте новый адрес и подтвердите доступ в течение 3 минут, иначе прежние порты восстановятся. Подписки могут быть временно недоступны до включения WEB Proxy.'
        : 'The panel restarts. Allow the new panel port in your firewall before migration. Open the new address and confirm access within 3 minutes or the previous ports are restored. Subscriptions may be temporarily unavailable until WEB Proxy is enabled.',
      onOk: async () => {
        setBusy(true);
        setError('');
        try {
          const response = await HttpUtil.post<Migration>(
            endpoint,
            {
              panelPort,
              subscriptionPort,
              expectedPanelPort: preview.panelPort,
              expectedSubscriptionPort: preview.subscriptionPort ?? 0,
            },
            { silent: true },
          );
          if (!response?.success || !response.obj)
            throw new Error(response?.msg || 'Port migration failed');
          const url = new URL(window.location.href);
          if (response.obj.newPanel !== response.obj.oldPanel)
            url.port = String(response.obj.newPanel);
          url.pathname = url.pathname.replace(/\/panel\/.*$/, '/panel/settings');
          url.search = '?ingress-port-confirm=1';
          url.hash = '';
          setNext(url.href);
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
          setBusy(false);
        }
      },
    });
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      {holder}
      <Button disabled={disabled || Boolean(next)} loading={busy} onClick={start}>
        {ru ? 'Перенести порты с автоматическим откатом' : 'Move ports with automatic rollback'}
      </Button>
      {error && <Alert type="error" showIcon title={error} />}
      {next && (
        <Alert
          type="warning"
          showIcon
          title={
            ru
              ? 'После перезапуска откройте новый адрес и подтвердите доступ'
              : 'After restart, open the new address and confirm access'
          }
          description={
            <a href={next} style={{ overflowWrap: 'anywhere' }}>
              {next}
            </a>
          }
        />
      )}
    </Space>
  );
}

export function TelemtPortMigrationConfirmation() {
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const [migration, setMigration] = useState<Migration>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  useEffect(() => {
    if (done) return;
    let active = true;
    let timer: number | undefined;
    const read = async () => {
      try {
        const response = await HttpUtil.get<Migration>(endpoint, undefined, { silent: true });
        if (!active) return;
        if (response?.success && response.obj) {
          const migration = response.obj;
          setMigration({
            ...migration,
            ready: migration.ready && Date.parse(migration.deadline) > Date.now(),
          });
        } else setMigration(undefined);
      } catch {
        if (active) setMigration(undefined);
      } finally {
        // Poll only after the previous request finishes; stale responses must not
        // replace the current migration or re-enable an expired confirmation.
        if (active) timer = window.setTimeout(() => void read(), 2000);
      }
    };
    void read();
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [done]);
  const confirm = async () => {
    if (!migration?.ready || busy || Date.parse(migration.deadline) <= Date.now()) return;
    setBusy(true);
    setError('');
    try {
      const response = await HttpUtil.post(
        endpoint + '/confirm',
        { id: migration.id },
        { silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Confirmation failed');
      setDone(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };
  if (done)
    return (
      <Alert
        type="success"
        showIcon
        title={
          ru
            ? 'Новые порты подтверждены. Вернитесь к настройке WEB Proxy.'
            : 'New ports confirmed. Return to WEB Proxy setup.'
        }
      />
    );
  return (
    <Alert
      type="warning"
      showIcon
      title={ru ? 'Подтверждение новых портов' : 'Confirm new listener ports'}
      description={
        <Space orientation="vertical">
          <Typography.Text>
            {migration
              ? (ru ? 'Откат без подтверждения: ' : 'Rollback without confirmation: ') +
                new Date(migration.deadline).toLocaleTimeString()
              : ru
                ? 'Нет активного переноса или сервер ещё запускается.'
                : 'No pending migration, or the server is still starting.'}
          </Typography.Text>
          <Button
            type="primary"
            disabled={!migration?.ready}
            loading={busy}
            onClick={() => void confirm()}
          >
            {ru ? 'Подтвердить доступ к панели' : 'Confirm panel access'}
          </Button>
          {error && <Typography.Text type="danger">{error}</Typography.Text>}
        </Space>
      }
    />
  );
}
