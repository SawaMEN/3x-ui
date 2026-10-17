import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import { CheckCircleOutlined, PoweroffOutlined, ReloadOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';

type DNSState = {
  enabled: boolean;
  installed: boolean;
  running: boolean;
  supported: boolean;
  unsupportedReason?: string;
  error?: string;
  domain: string;
  doh: boolean;
  port: number;
  upstream: string;
  useSingBox: boolean;
  dohUrl?: string;
  installedVersion?: string;
};

type APIResponse = { success?: boolean; obj?: DNSState; msg?: string };
const defaults: DNSState = {
  enabled: false,
  installed: false,
  running: false,
  supported: true,
  domain: '',
  doh: false,
  port: 8443,
  upstream: 'quad9',
  useSingBox: true,
};

export default function OxiDNSSettings() {
  const [api, contextHolder] = message.useMessage();
  const [settings, setSettings] = useState<DNSState>(defaults);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    const response = (await HttpUtil.get('/panel/api/oxidns/status', undefined, {
      silent: true,
    })) as APIResponse;
    if (response.success && response.obj) {
      setSettings(response.obj);
      setError('');
    } else {
      setError(response.msg || 'Не удалось получить настройки DNS.');
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void refresh();
    }, 0);
    const onComponentChanged = () => {
      void refresh();
    };
    window.addEventListener('oxidns:changed', onComponentChanged);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('oxidns:changed', onComponentChanged);
    };
  }, [refresh]);

  const [checking, setChecking] = useState(false);

  const checkDNS = async () => {
    setChecking(true);
    try {
      const result = await HttpUtil.post('/panel/api/oxidns/check', undefined, {
        silent: true,
        timeout: 12000,
      });
      if (!result.success) {
        throw new Error(result.msg || 'Сервер DNS не отвечает.');
      }
      api.success('DNS работает: доменные имена успешно разрешаются.');
    } catch (cause) {
      api.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setChecking(false);
    }
  };

  const patch = (data: Partial<DNSState>) => setSettings((current) => ({ ...current, ...data }));

  const apply = async (enable: boolean) => {
    setBusy(true);
    setError('');
    try {
      const result = (await HttpUtil.post(
        enable ? '/panel/api/oxidns/setup' : '/panel/api/oxidns/disable',
        enable
          ? {
              domain: settings.domain.trim(),
              doh: settings.doh,
              port: settings.port,
              upstream: settings.upstream,
              useSingBox: settings.useSingBox,
            }
          : undefined,
        { silent: true, timeout: 180000 },
      )) as APIResponse;
      if (!result.success || !result.obj)
        throw new Error(result.msg || 'Не удалось изменить настройки DNS');
      setSettings(result.obj);
      api.success(enable ? 'Собственный DNS включён.' : 'Собственный DNS отключён.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  };

  // Installing/removing the package controls feature visibility. The regular
  // sing-box DNS editor is always available; OxiDNS controls are not.
  if (!loading && !settings.installed && !error) return null;

  return (
    <Card
      title="Собственный DNS — OxiDNS"
      extra={
        settings.enabled && settings.running ? (
          <Tag color="success" icon={<CheckCircleOutlined />}>
            Работает
          </Tag>
        ) : (
          <Tag>{settings.installed ? 'Выключен' : 'Не установлен'}</Tag>
        )
      }
      loading={loading}
    >
      {contextHolder}
      <Space orientation="vertical" style={{ width: '100%' }} size={14}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          OxiDNS установлен. Выберите провайдера и нажмите «Создать и включить DNS». Настройки,
          служба и подключение к sing-box будут созданы автоматически.
        </Typography.Paragraph>
        {error && <Alert type="error" showIcon title={error} />}
        {settings.enabled && !settings.running && (
          <Alert
            type="warning"
            showIcon
            title="DNS включён в настройках, но служба не запущена."
            description="Проверьте свободные порты и повторите применение настроек."
          />
        )}
        {settings.supported === false && (
          <Alert
            type="warning"
            showIcon
            title="Автоматическая установка недоступна"
            description={settings.unsupportedReason || 'Нужен Linux с systemd.'}
          />
        )}
        <div>
          <Typography.Text strong>Какой DNS использовать?</Typography.Text>
          <Select
            style={{ width: '100%', marginTop: 8 }}
            value={settings.upstream}
            disabled={busy}
            options={[
              { value: 'quad9', label: 'Quad9 — защищённый DNS (рекомендуется)' },
              { value: 'cloudflare', label: 'Cloudflare — быстрый DNS' },
              { value: 'adguard', label: 'AdGuard DNS — фильтрация рекламы' },
              { value: 'google', label: 'Google Public DNS — без дополнительной фильтрации' },
              { value: 'yandex-basic', label: 'Яндекс DNS — Базовый' },
              { value: 'yandex-safe', label: 'Яндекс DNS — Безопасный (защита от опасных сайтов)' },
              {
                value: 'yandex-family',
                label: 'Яндекс DNS — Семейный (фильтрация взрослого контента)',
              },
            ]}
            onChange={(value) => patch({ upstream: value })}
          />
        </div>
        <Space align="center" wrap>
          <Switch
            checked={settings.useSingBox}
            disabled={busy}
            onChange={(value) => patch({ useSingBox: value })}
          />
          <Typography.Text>Использовать этот DNS в sing-box</Typography.Text>
        </Space>
        <Space align="center" wrap>
          <Switch
            checked={settings.doh}
            disabled={busy}
            onChange={(value) => patch({ doh: value })}
          />
          <Typography.Text>Включить собственный DoH для других устройств</Typography.Text>
        </Space>
        {settings.doh && (
          <>
            <Alert
              type="info"
              showIcon
              title="Для DoH нужен домен и HTTPS-сертификат."
              description="Укажите домен с действующим сертификатом, уже настроенным в панели. При ошибке сертификата DNS не будет изменён."
            />
            <div>
              <Typography.Text>Ваш домен для DNS</Typography.Text>
              <Input
                style={{ marginTop: 8 }}
                placeholder="dns.example.com"
                value={settings.domain}
                disabled={busy}
                onChange={(event) => patch({ domain: event.target.value })}
              />
            </div>
          </>
        )}
        <Collapse
          size="small"
          items={[
            {
              key: 'advanced',
              label: 'Дополнительные настройки',
              children: (
                <Space orientation="vertical" style={{ width: '100%' }}>
                  <Typography.Text>HTTPS-порт (по умолчанию 8443)</Typography.Text>
                  <InputNumber
                    min={1}
                    max={65535}
                    value={settings.port}
                    disabled={busy}
                    onChange={(value) => patch({ port: value ?? 8443 })}
                  />
                  <Typography.Text type="secondary">
                    Если порт 443 занят VPN, оставьте 8443. При использовании DoH его нужно открыть
                    в межсетевом экране. DNS-правила sing-box продолжают работать.
                  </Typography.Text>
                </Space>
              ),
            },
          ]}
        />
        {settings.dohUrl && settings.enabled && (
          <Alert
            type="success"
            showIcon
            title="Ваш DoH-адрес"
            description={<Typography.Text copyable>{settings.dohUrl}</Typography.Text>}
          />
        )}
        <Space wrap>
          <Button
            type="primary"
            icon={settings.enabled ? <ReloadOutlined /> : <CheckCircleOutlined />}
            loading={busy}
            disabled={settings.supported === false}
            onClick={() => void apply(true)}
          >
            {settings.enabled ? 'Сохранить и применить' : 'Создать и включить DNS'}
          </Button>
          {settings.enabled && (
            <Button
              danger
              icon={<PoweroffOutlined />}
              disabled={busy}
              onClick={() => void apply(false)}
            >
              Выключить
            </Button>
          )}
          {settings.enabled && settings.running && (
            <Button loading={checking} disabled={busy} onClick={() => void checkDNS()}>
              Проверить работу DNS
            </Button>
          )}
          <Button icon={<ReloadOutlined />} onClick={() => void refresh()} disabled={busy}>
            Проверить состояние
          </Button>
        </Space>
        <Typography.Text type="secondary">
          Управление установкой, обновлением и удалением OxiDNS находится в разделе «Обновления
          пакетов и ядер Linux». При удалении функции собственного DNS исчезнут, а sing-box вернётся
          к прежним настройкам.
        </Typography.Text>
      </Space>
    </Card>
  );
}
