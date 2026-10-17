import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
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

type DNSConfig = {
  domain: string;
  doh: boolean;
  http3: boolean;
  dot: boolean;
  doq: boolean;
  port: number;
  dotPort: number;
  doqPort: number;
  upstream: string;
  useSingBox: boolean;
};
type DNSState = DNSConfig & {
  enabled: boolean;
  installed: boolean;
  running: boolean;
  supported: boolean;
  unsupportedReason?: string;
  dohUrl?: string;
  dotAddress?: string;
  doqAddress?: string;
  localAddress?: string;
};
type APIResponse = { success?: boolean; obj?: DNSState; msg?: string };
const defaults: DNSState = {
  enabled: false,
  installed: false,
  running: false,
  supported: true,
  domain: '',
  doh: false,
  http3: false,
  dot: false,
  doq: false,
  port: 8443,
  dotPort: 853,
  doqPort: 853,
  upstream: 'quad9',
  useSingBox: true,
};
const providers = [
  { value: 'quad9', label: 'Quad9 — защита от вредоносных доменов' },
  { value: 'cloudflare', label: 'Cloudflare — стандартный DNS' },
  { value: 'adguard', label: 'AdGuard — блокировка рекламы и трекеров' },
  { value: 'google', label: 'Google Public DNS — стандартный DNS' },
  { value: 'yandex-basic', label: 'Яндекс — Базовый' },
  { value: 'yandex-safe', label: 'Яндекс — Безопасный' },
  { value: 'yandex-family', label: 'Яндекс — Семейный' },
];
const getConfig = (state: DNSState): DNSConfig => ({
  domain: state.domain,
  doh: state.doh ?? false,
  http3: state.http3 ?? false,
  dot: state.dot ?? false,
  doq: state.doq ?? false,
  port: state.port || 8443,
  dotPort: state.dotPort || 853,
  doqPort: state.doqPort || 853,
  upstream: state.upstream,
  useSingBox: state.useSingBox,
});

export default function OxiDNSSettings() {
  const [api, contextHolder] = message.useMessage();
  const [status, setStatus] = useState<DNSState>(defaults);
  const [draft, setDraft] = useState<DNSConfig>(() => getConfig(defaults));
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [checking, setChecking] = useState(false);
  const [verified, setVerified] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const refresh = useCallback(async (resetDraft = true, preserveError = false) => {
    try {
      const response = (await HttpUtil.get('/panel/api/oxidns/status', undefined, {
        silent: true,
      })) as APIResponse;
      if (!response.success || !response.obj)
        throw new Error(response.msg || 'Не удалось получить состояние DNS.');
      setStatus(response.obj);
      if (resetDraft) setDraft(getConfig(response.obj));
      if (!preserveError) setError('');
      setVerified(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось получить состояние DNS.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void refresh();
    }, 0);
    const onChanged = () => {
      setNotice('');
      void refresh();
    };
    window.addEventListener('oxidns:changed', onChanged);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener('oxidns:changed', onChanged);
    };
  }, [refresh]);

  const patch = (value: Partial<DNSConfig>) => {
    setDraft((current) => ({ ...current, ...value }));
    setError('');
    setNotice('');
    setVerified(false);
  };
  const applied = getConfig(status);
  const dirty = (Object.keys(applied) as (keyof DNSConfig)[]).some(
    (key) => draft[key] !== applied[key],
  );
  const external = draft.doh || draft.dot || draft.doq;
  const domainMissing = external && !draft.domain.trim();
  const validPort = (port: number) =>
    Number.isInteger(port) && port > 0 && port <= 65535 && port !== 15353;
  const invalidPort =
    (draft.doh && !validPort(draft.port)) ||
    (draft.dot && !validPort(draft.dotPort)) ||
    (draft.doq && !validPort(draft.doqPort));
  const conflictPorts =
    (draft.doh && draft.dot && draft.port === draft.dotPort) ||
    (draft.doh && draft.http3 && draft.doq && draft.port === draft.doqPort);
  const unused = !draft.useSingBox && !external;

  const apply = async (enable: boolean) => {
    if (enable && domainMissing) {
      setError('Для публичных DNS-протоколов укажите домен с действующим сертификатом панели.');
      return;
    }
    setBusy(true);
    setNotice('');
    setError('');
    try {
      const response = (await HttpUtil.post(
        enable ? '/panel/api/oxidns/setup' : '/panel/api/oxidns/disable',
        enable ? { ...draft, domain: draft.domain.trim() } : undefined,
        { silent: true, timeout: 180000 },
      )) as APIResponse;
      if (!response.success || !response.obj)
        throw new Error(response.msg || 'Не удалось применить настройки DNS.');
      setStatus(response.obj);
      setDraft(getConfig(response.obj));
      setVerified(enable && response.obj.running);
      setNotice(
        enable
          ? 'DNS включён. Панель проверила разрешение доменного имени через OxiDNS.'
          : 'OxiDNS остановлен. Настройки сохранены для повторного включения.',
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось изменить DNS.');
      await refresh(false, true);
    } finally {
      setBusy(false);
    }
  };

  const checkDNS = async () => {
    setChecking(true);
    setError('');
    try {
      const response = await HttpUtil.post('/panel/api/oxidns/check', undefined, {
        silent: true,
        timeout: 12000,
      });
      if (!response.success)
        throw new Error(response.msg || 'DNS не ответил на проверочный запрос.');
      setVerified(true);
      api.success('DNS отвечает на запросы и возвращает IP-адрес.');
    } catch (cause) {
      setVerified(false);
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setChecking(false);
    }
  };

  if (!loading && !status.installed) {
    if (!error) return null;
    return (
      <Alert
        type="error"
        showIcon
        title="Не удалось проверить состояние OxiDNS"
        description={error}
        action={<Button onClick={() => void refresh()}>Повторить</Button>}
      />
    );
  }

  return (
    <Card
      title="Собственный DNS — OxiDNS"
      loading={loading}
      extra={
        status.enabled && status.running ? (
          <Tag color="success" icon={<CheckCircleOutlined />}>
            Запущен
          </Tag>
        ) : (
          <Tag>{status.enabled ? 'Ошибка службы' : 'Готов к настройке'}</Tag>
        )
      }
    >
      {contextHolder}
      <Space orientation="vertical" style={{ width: '100%' }} size={16}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Личный DNS-резолвер на вашем сервере. OxiDNS шифрует запросы к выбранному провайдеру через
          DNS-over-TLS. Для обычного DNS домен и сертификат не нужны.
        </Typography.Paragraph>

        {status.enabled && status.running ? (
          <Alert
            type={verified ? 'success' : 'info'}
            showIcon
            title={verified ? 'DNS проверен и работает' : 'Служба DNS запущена'}
            description={
              <Space orientation="vertical" size={4}>
                <Typography.Text>
                  Локальный адрес:{' '}
                  <Typography.Text code copyable>
                    {status.localAddress || '127.0.0.1:15353'}
                  </Typography.Text>{' '}
                  — только для этого сервера.
                </Typography.Text>
                <Typography.Text>
                  {status.useSingBox
                    ? 'Подключён к sing-box. Существующие DNS-правила сохранены.'
                    : 'Не подключён к sing-box. Это можно изменить ниже.'}
                </Typography.Text>
              </Space>
            }
          />
        ) : status.enabled ? (
          <Alert
            type="warning"
            showIcon
            title="DNS включён, но служба не запущена"
            description="Проверьте свободный порт и доступность провайдера, затем повторите применение."
          />
        ) : (
          <Alert
            type="info"
            showIcon
            title="Готово к созданию DNS"
            description="Выберите провайдера и нажмите «Создать и включить DNS». Конфигурация, запуск и проверка запроса выполнятся автоматически."
          />
        )}

        {status.supported === false && (
          <Alert
            type="warning"
            showIcon
            title="Управление OxiDNS недоступно"
            description={status.unsupportedReason || 'Требуется Linux с запущенным systemd.'}
          />
        )}
        {error && <Alert type="error" showIcon title={error} />}
        {notice && <Alert type="success" showIcon title={notice} />}

        <div>
          <Typography.Text strong>1. Выберите DNS-провайдера</Typography.Text>
          <Select
            aria-label="DNS-провайдер"
            style={{ width: '100%', marginTop: 8 }}
            value={draft.upstream}
            options={providers}
            disabled={busy}
            onChange={(upstream) => patch({ upstream })}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 6, marginBottom: 0 }}>
            Провайдер разрешает доменные имена. Серверу нужен доступ к TCP-порту 853.
          </Typography.Paragraph>
        </div>

        <div>
          <Typography.Text strong>2. Подключите к sing-box</Typography.Text>
          <Space align="center" style={{ display: 'flex', marginTop: 8 }}>
            <Switch
              aria-label="Использовать OxiDNS в sing-box"
              checked={draft.useSingBox}
              disabled={busy}
              onChange={(useSingBox) => patch({ useSingBox })}
            />
            <Typography.Text>Использовать в sing-box (рекомендуется)</Typography.Text>
          </Space>
          <Typography.Text type="secondary">
            Панель сама настроит DNS по умолчанию, не удаляя существующие DNS-правила.
          </Typography.Text>
        </div>

        <div>
          <Typography.Text strong>Для других устройств (необязательно)</Typography.Text>
          <Space align="center" style={{ display: 'flex', marginTop: 8 }}>
            <Switch
              aria-label="Включить публичный DoH"
              checked={draft.doh}
              disabled={busy}
              onChange={(doh) => patch({ doh, ...(doh ? {} : { http3: false }) })}
            />
            <Typography.Text>Включить защищённый DNS по HTTPS (DoH)</Typography.Text>
          </Space>
          <Space align="center" style={{ display: 'flex', marginTop: 8 }}>
            <Switch
              aria-label="Включить DoT"
              checked={draft.dot}
              disabled={busy}
              onChange={(dot) => patch({ dot })}
            />
            <Typography.Text>DNS-over-TLS (DoT) — TCP/853</Typography.Text>
          </Space>
          <Space align="center" style={{ display: 'flex', marginTop: 8 }}>
            <Switch
              aria-label="Включить DoQ"
              checked={draft.doq}
              disabled={busy}
              onChange={(doq) => patch({ doq })}
            />
            <Typography.Text>DNS-over-QUIC (DoQ) — UDP/853</Typography.Text>
          </Space>
          {draft.doh && (
            <Space align="center" style={{ display: 'flex', marginTop: 8 }}>
              <Switch
                aria-label="Включить HTTP/3"
                checked={draft.http3}
                disabled={busy}
                onChange={(http3) => patch({ http3 })}
              />
              <Typography.Text>HTTP/3 (H3) для DoH — UDP на том же HTTPS-порту</Typography.Text>
            </Space>
          )}
          {external && (
            <Space orientation="vertical" size={12} style={{ width: '100%', marginTop: 10 }}>
              <Alert
                type="info"
                showIcon
                title="Для DoH, DoT и DoQ нужны домен и сертификат"
                description="Используется действующий TLS-сертификат панели. Домен должен указывать на сервер. Откройте соответствующие TCP/UDP-порты в межсетевом экране; панель не изменяет правила firewall."
              />
              <div>
                <Typography.Text>Домен для DoH</Typography.Text>
                <Input
                  aria-label="Домен для DoH"
                  style={{ marginTop: 6 }}
                  placeholder="dns.example.com"
                  value={draft.domain}
                  status={domainMissing ? 'error' : undefined}
                  disabled={busy}
                  onChange={(event) => patch({ domain: event.target.value })}
                />
                {domainMissing && (
                  <Typography.Text type="danger">
                    Укажите домен с действующим сертификатом панели.
                  </Typography.Text>
                )}
              </div>
              {draft.doh && (
                <div>
                  <Typography.Text>HTTPS-порт DoH (TCP; также UDP при H3)</Typography.Text>
                  <InputNumber
                    aria-label="HTTPS-порт DoH"
                    style={{ display: 'block', marginTop: 6, width: '100%', maxWidth: 180 }}
                    min={1}
                    max={65535}
                    value={draft.port}
                    disabled={busy}
                    onChange={(port) => patch({ port: port ?? 8443 })}
                  />
                  <Typography.Text type="secondary">
                    По умолчанию 8443. Порт 443 — только если свободен.
                  </Typography.Text>
                </div>
              )}
              {draft.dot && (
                <div>
                  <Typography.Text>Порт DoT (TCP)</Typography.Text>
                  <InputNumber
                    aria-label="Порт DoT"
                    style={{ display: 'block', marginTop: 6, width: '100%', maxWidth: 180 }}
                    min={1}
                    max={65535}
                    value={draft.dotPort}
                    disabled={busy}
                    onChange={(dotPort) => patch({ dotPort: dotPort ?? 853 })}
                  />
                </div>
              )}
              {draft.doq && (
                <div>
                  <Typography.Text>Порт DoQ (UDP)</Typography.Text>
                  <InputNumber
                    aria-label="Порт DoQ"
                    style={{ display: 'block', marginTop: 6, width: '100%', maxWidth: 180 }}
                    min={1}
                    max={65535}
                    value={draft.doqPort}
                    disabled={busy}
                    onChange={(doqPort) => patch({ doqPort: doqPort ?? 853 })}
                  />
                </div>
              )}
              {invalidPort && (
                <Typography.Text type="danger">
                  Некорректный порт или порт 15353 зарезервирован локальным DNS.
                </Typography.Text>
              )}
              {conflictPorts && (
                <Typography.Text type="danger">
                  Конфликт портов: DoH/DoT занимают TCP, а HTTP/3/DoQ — UDP. Укажите разные порты
                  для конфликтующих служб.
                </Typography.Text>
              )}
            </Space>
          )}
        </div>

        {unused && (
          <Alert
            type="warning"
            showIcon
            title="DNS пока не подключён ни к одному клиенту"
            description="Включите использование в sing-box или хотя бы один из публичных протоколов: DoH, DoT, DoQ."
          />
        )}

        {status.enabled && status.running && status.doh && status.dohUrl && (
          <Alert
            type="success"
            showIcon
            title="DoH (HTTPS, HTTP/1.1 и HTTP/2)"
            description={<Typography.Text copyable>{status.dohUrl}</Typography.Text>}
          />
        )}
        {status.enabled && status.running && status.doh && status.http3 && status.dohUrl && (
          <Alert
            type="info"
            showIcon
            title="DoH через HTTP/3"
            description={
              <>
                Используется тот же URL, что для DoH, поверх QUIC (UDP/{status.port}). HTTPS-клиенты
                должны поддерживать H3.
              </>
            }
          />
        )}
        {status.enabled && status.running && status.dot && status.dotAddress && (
          <Alert
            type="success"
            showIcon
            title="DoT — DNS-over-TLS"
            description={<Typography.Text copyable>{`tls://${status.dotAddress}`}</Typography.Text>}
          />
        )}
        {status.enabled && status.running && status.doq && status.doqAddress && (
          <Alert
            type="info"
            showIcon
            title="DoQ — DNS-over-QUIC (UDP)"
            description={
              <Typography.Text copyable>{`quic://${status.doqAddress}`}</Typography.Text>
            }
          />
        )}
        {dirty && status.enabled && (
          <Alert
            type="warning"
            showIcon
            title="Есть неприменённые изменения"
            description="Работающая служба пока использует предыдущие настройки."
          />
        )}

        <Space wrap>
          <Button
            type="primary"
            icon={status.enabled ? <ReloadOutlined /> : <CheckCircleOutlined />}
            loading={busy}
            disabled={
              checking ||
              !status.supported ||
              domainMissing ||
              invalidPort ||
              conflictPorts ||
              unused
            }
            onClick={() => void apply(true)}
          >
            {status.enabled
              ? dirty
                ? 'Сохранить и применить'
                : 'Перезапустить DNS'
              : 'Создать и включить DNS'}
          </Button>
          {status.enabled && (
            <Button
              danger
              icon={<PoweroffOutlined />}
              disabled={busy || checking}
              onClick={() => void apply(false)}
            >
              Отключить DNS
            </Button>
          )}
          {status.enabled && status.running && (
            <Button loading={checking} disabled={busy} onClick={() => void checkDNS()}>
              Проверить DNS-запрос
            </Button>
          )}
          {dirty && (
            <Button
              disabled={busy || checking}
              onClick={() => {
                setDraft(getConfig(status));
                setError('');
                setNotice('');
              }}
            >
              Отменить изменения
            </Button>
          )}
          <Button
            icon={<ReloadOutlined />}
            disabled={busy || checking}
            onClick={() => void refresh()}
          >
            Обновить состояние
          </Button>
        </Space>
        <Typography.Text type="secondary">
          Отключение сохранит настройки. Установка, обновление и полное удаление OxiDNS находятся в
          «Настройки → Обновления пакетов и ядер Linux».
        </Typography.Text>
      </Space>
    </Card>
  );
}
