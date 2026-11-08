import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Space, Tag, Typography, message } from 'antd';
import { HttpUtil } from '@/utils';
import OlcRTCClientsModal from './OlcRTCClientsModal';

type Status = {
  installed?: boolean;
  running?: boolean;
  busy?: boolean;
  operation?: string;
  error?: string;
};
type Response = { success?: boolean; msg?: string; obj?: Status };

export default function OlcRTCManagement() {
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [restarting, setRestarting] = useState(false);
  const [clientsOpen, setClientsOpen] = useState(false);
  const [api, contextHolder] = message.useMessage();

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const response = (await HttpUtil.get('/panel/api/olcrtc/status', undefined, {
        silent: true,
      })) as Response;
      if (!response.success || !response.obj) {
        throw new Error(response.msg || 'Не удалось получить состояние OlcRTC.');
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
    let active = true;
    void Promise.resolve().then(() => {
      if (active) void refresh();
    });
    return () => {
      active = false;
    };
  }, [refresh]);

  useEffect(() => {
    if (!status?.busy) return;
    let active = true;
    let timer: number;
    const poll = async () => {
      await refresh();
      if (active) timer = window.setTimeout(() => void poll(), 1500);
    };
    timer = window.setTimeout(() => void poll(), 1500);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [status?.busy, refresh]);

  const restart = async () => {
    setRestarting(true);
    try {
      const response = (await HttpUtil.post('/panel/api/olcrtc/restart')) as Response;
      if (!response.success) throw new Error(response.msg || 'Не удалось перезапустить OlcRTC.');
      api.success('Перезапуск службы OlcRTC запущен.');
      await refresh();
    } catch (cause) {
      api.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setRestarting(false);
    }
  };

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      {contextHolder}
      <Typography.Title level={4}>OlcRTC — управление клиентами</Typography.Title>
      {error && <Alert type="error" showIcon title={error} />}
      {!loading && !status?.installed ? (
        <Alert
          type="info"
          showIcon
          title="Для работы требуется установка OlcRTC"
          description="Установите OlcRTC через «Настройки → Системные пакеты и ядро Linux». После установки здесь появятся настройки клиентов и управление службой."
        />
      ) : (
        <>
          <Tag color={status?.running ? 'success' : 'warning'}>
            {loading ? 'Загрузка…' : status?.running ? 'Служба работает' : 'Служба остановлена'}
          </Tag>
          {status?.error && <Alert type="warning" showIcon title={status.error} />}
          <Space wrap>
            <Button
              type="primary"
              disabled={!status?.installed || !status?.running || status?.busy || loading}
              onClick={() => setClientsOpen(true)}
            >
              Настроить клиентов
            </Button>
            <Button
              disabled={!status?.installed || status?.busy || restarting || loading}
              loading={restarting}
              onClick={() => void restart()}
            >
              Перезапустить службу
            </Button>
          </Space>
        </>
      )}
      <Button onClick={() => void refresh()} loading={loading}>
        Обновить состояние
      </Button>
      <OlcRTCClientsModal open={clientsOpen} onClose={() => setClientsOpen(false)} />
    </Space>
  );
}
