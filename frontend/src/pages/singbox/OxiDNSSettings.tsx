import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  CloudDownloadOutlined,
  DeleteOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  SyncOutlined,
} from '@ant-design/icons';
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
  installedVersion?: string;
  latestVersion?: string;
  updateAvailable?: boolean;
  error?: string;
  dohUrl?: string;
  dotAddress?: string;
  doqAddress?: string;
  localAddress?: string;
};
type APIResponse<T = DNSState> = { success?: boolean; obj?: T; msg?: string };

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
  { value: 'cloudflare', label: 'Cloudflare — DNS без дополнительной фильтрации' },
  { value: 'adguard', label: 'AdGuard — блокировка рекламы и трекеров' },
  { value: 'google', label: 'Google Public DNS' },
  { value: 'yandex-basic', label: 'Яндекс — Базовый' },
  { value: 'yandex-safe', label: 'Яндекс — Безопасный' },
  { value: 'yandex-family', label: 'Яндекс — Семейный' },
];
const getConfig = (state: DNSState): DNSConfig => ({
  domain: state.domain || '',
  doh: state.doh ?? false,
  http3: state.http3 ?? false,
  dot: state.dot ?? false,
  doq: state.doq ?? false,
  port: state.port || 8443,
  dotPort: state.dotPort || 853,
  doqPort: state.doqPort || 853,
  upstream: state.upstream || 'quad9',
  useSingBox: state.useSingBox ?? true,
});

export default function OxiDNSSettings() {
  const [api, contextHolder] = message.useMessage();
  const [status, setStatus] = useState<DNSState>(defaults);
  const [draft, setDraft] = useState<DNSConfig>(() => getConfig(defaults));
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<'apply' | 'disable' | 'update' | 'remove' | null>(null);
  const [checking, setChecking] = useState(false);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updatesChecked, setUpdatesChecked] = useState(false);
  const [verified, setVerified] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logsBusy, setLogsBusy] = useState(false);
  const [logs, setLogs] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const refresh = useCallback(async (checkUpdates = false, keepDraft = false, keepError = false) => {
    try {
      const response = (await HttpUtil.get(
        '/panel/api/oxidns/status' + (checkUpdates ? '?check=true' : ''),
        undefined,
        { silent: true, timeout: checkUpdates ? 26000 : 10000 },
      )) as APIResponse;
      if (!response.success || !response.obj)
        throw new Error(response.msg || 'Не удалось получить состояние DNS.');
      setStatus(response.obj);
      if (!keepDraft) {
        setDraft(getConfig(response.obj));
        if (response.obj.doh || response.obj.dot || response.obj.doq) setShowAdvanced(true);
      }
      if (checkUpdates) setUpdatesChecked(true);
      if (!keepError) setError('');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось получить состояние DNS.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const onChanged = () => {
      setNotice('');
      setUpdatesChecked(false);
      void refresh();
    };
    window.addEventListener('oxidns:changed', onChanged);
    return () => window.removeEventListener('oxidns:changed', onChanged);
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
  const domainInvalid =
    external &&
    !/^(?=.{4,253}$)[a-z0-9-]+(?:\.[a-z0-9-]+)+$/i.test(draft.domain.trim());
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
  const locked = busy !== null || checking || checkingUpdates;
  const canApply = status.supported && !domainInvalid && !invalidPort && !conflictPorts && !unused;
  const isHealthy = status.installed && status.enabled && status.running;
  const primaryLabel = !status.installed
    ? 'Установить и включить DNS'
    : !status.enabled
      ? 'Включить DNS'
      : dirty
        ? 'Сохранить и применить'
        : status.running
          ? 'Перезапустить DNS'
          : 'Повторить запуск';
  const statusLabel = !status.installed
    ? 'Не установлен'
    : isHealthy
      ? 'Работает'
      : status.enabled
        ? 'Ошибка службы'
        : 'Выключен';

  const request = async (endpoint: string, payload?: unknown, timeout = 180000) => {
    const response = (await HttpUtil.post('/panel/api/oxidns/' + endpoint, payload, {
      silent: true,
      timeout,
    })) as APIResponse;
    if (!response.success || !response.obj)
      throw new Error(response.msg || 'Операция OxiDNS завершилась с ошибкой.');
    return response.obj;
  };

  const apply = async (enable: boolean) => {
    if (enable && !canApply) {
      setError('Проверьте настройки DNS: провайдер, подключение, домен и порты.');
      return;
    }
    setBusy(enable ? 'apply' : 'disable');
    setNotice('');
    setError('');
    try {
      // Setup installs the verified official binary automatically if it is missing.
      const next = await request(
        enable ? 'setup' : 'disable',
        enable ? { ...draft, domain: draft.domain.trim() } : undefined,
      );
      setStatus(next);
      setDraft(getConfig(next));
      setVerified(enable && next.running);
      setUpdatesChecked(false);
      setNotice(
        enable
          ? 'OxiDNS установлен, запущен и проверен. Новая конфигурация применяется автоматически.'
          : 'DNS отключён. Настройки сохранены для следующего запуска.',
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось изменить DNS.');
      await refresh(false, true, true);
    } finally {
      setBusy(null);
    }
  };

  const checkDNS = async () => {
    setChecking(true);
    setError('');
    try {
      const response = (await HttpUtil.post('/panel/api/oxidns/check', undefined, {
        silent: true,
        timeout: 12000,
      })) as APIResponse<{ healthy: boolean }>;
      if (!response.success || !response.obj?.healthy)
        throw new Error(response.msg || 'DNS не ответил на проверочный запрос.');
      setVerified(true);
      api.success('Проверка пройдена: OxiDNS разрешает доменные имена.');
    } catch (cause) {
      setVerified(false);
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setChecking(false);
    }
  };

  const update = async () => {
    setBusy('update');
    setError('');
    setNotice('');
    try {
      const next = await request('update');
      setStatus(next);
      setDraft(getConfig(next));
      setUpdatesChecked(false);
      setNotice('Компонент OxiDNS обновлён. Рабочие настройки сохранены.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось обновить OxiDNS.');
      await refresh(false, true, true);
    } finally {
      setBusy(null);
    }
  };

  const remove = async () => {
    setBusy('remove');
    setError('');
    setNotice('');
    try {
      const next = await request('remove', undefined, 90000);
      setStatus(next);
      setDraft(getConfig(next));
      setVerified(false);
      setUpdatesChecked(false);
      setNotice('OxiDNS удалён. Служба остановлена, настройки компонента очищены.');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось удалить OxiDNS.');
      await refresh(false, true, true);
    } finally {
      setBusy(null);
    }
  };

  const openLogs = async () => {
    setLogsOpen(true);
    setLogsBusy(true);
    setLogs('');
    try {
      const response = (await HttpUtil.get('/panel/api/oxidns/logs', undefined, {
        silent: true,
        timeout: 12000,
      })) as APIResponse<{ logs: string }>;
      if (!response.success || !response.obj)
        throw new Error(response.msg || 'Не удалось прочитать журнал OxiDNS.');
      setLogs(response.obj.logs || 'Записей в журнале пока нет.');
    } catch (cause) {
      setLogs(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLogsBusy(false);
    }
  };

  const endpoint = (title: string, value: string, description: string) => (
    <div style={{ padding: '8px 0' }}>
      <Typography.Text strong style={{ display: 'block' }}>{title}</Typography.Text>
      <Typography.Text code copyable style={{ overflowWrap: 'anywhere' }}>
        {value}
      </Typography.Text>
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 4 }}>
        {description}
      </Typography.Paragraph>
    </div>
  );

  return (
    <Card
      title="OxiDNS — собственный DNS"
      loading={loading}
      extra={
        <Tag color={isHealthy ? 'success' : status.enabled ? 'warning' : 'default'}>
          {statusLabel}
        </Tag>
      }
    >
      {contextHolder}
      <Space orientation="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Собственный DNS на вашем сервере: шифрованный DNS к выбранному провайдеру, кэширование и
          автоматическое подключение к sing-box. Не требует домена или сертификата для локального
          использования.
        </Typography.Paragraph>

        {status.supported === false && (
          <Alert
            type="warning"
            showIcon
            title="Автоматическая установка недоступна"
            description={status.unsupportedReason || 'Требуется Linux с запущенным systemd и права root.'}
          />
        )}
        {error && (
          <Alert
            type="error"
            showIcon
            title="Операция не выполнена"
            description={error}
            action={<Button size="small" onClick={() => void refresh(false, true)}>Обновить статус</Button>}
          />
        )}
        {notice && <Alert type="success" showIcon title={notice} />}
        {status.enabled && !status.running && (
          <Alert
            type="warning"
            showIcon
            title="Служба DNS не запущена"
            description="Можно повторить запуск, сменить провайдера или открыть журнал ошибок ниже."
          />
        )}

        {isHealthy && (
          <Alert
            type={verified ? 'success' : 'info'}
            showIcon
            title={verified ? 'DNS работает — запрос успешно проверен' : 'Служба OxiDNS запущена'}
            description={status.useSingBox
              ? 'DNS подключён к sing-box. Существующие правила DNS не удалены.'
              : 'OxiDNS работает отдельно от sing-box. Включите подключение ниже при необходимости.'}
          />
        )}

        <Card
          size="small"
          title={isHealthy ? 'Настройки DNS' : 'Быстрый запуск'}
          extra={!status.installed && status.supported ? <Tag color="processing">Установка автоматически</Tag> : undefined}
        >
          <Space orientation="vertical" size={14} style={{ width: '100%' }}>
            <div>
              <Typography.Text strong>DNS-провайдер</Typography.Text>
              <Select
                aria-label="DNS-провайдер"
                style={{ width: '100%', marginTop: 8 }}
                value={draft.upstream}
                options={providers}
                disabled={locked}
                onChange={(upstream) => patch({ upstream })}
              />
              <Typography.Text type="secondary">
                Все провайдеры используются через зашифрованный DNS-over-TLS (TCP/853).
              </Typography.Text>
            </div>
            <Space align="start" style={{ display: 'flex' }}>
              <Switch
                aria-label="Подключить OxiDNS к sing-box"
                checked={draft.useSingBox}
                disabled={locked}
                onChange={(useSingBox) => patch({ useSingBox })}
              />
              <div>
                <Typography.Text strong>Использовать в sing-box</Typography.Text>
                <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  Рекомендуется. Панель сама подключит OxiDNS как основной DNS, не удаляя
                  пользовательские DNS-правила.
                </Typography.Paragraph>
              </div>
            </Space>

            {unused && (
              <Alert
                type="warning"
                showIcon
                title="DNS не будет использоваться"
                description="Подключите sing-box или включите хотя бы один публичный протокол в дополнительных настройках."
              />
            )}
            {dirty && status.enabled && (
              <Alert
                type="info"
                showIcon
                title="Настройки изменены, но ещё не применены"
              />
            )}
            <Space wrap>
              <Button
                type="primary"
                size="large"
                aria-label={primaryLabel}
                icon={!status.installed ? <CloudDownloadOutlined /> : <CheckCircleOutlined />}
                loading={busy === 'apply'}
                disabled={locked && busy !== 'apply' || !canApply}
                onClick={() => void apply(true)}
              >
                {primaryLabel}
              </Button>
              {status.enabled && (
                <Button
                  danger
                  aria-label="Выключить"
                  icon={<PoweroffOutlined />}
                  loading={busy === 'disable'}
                  disabled={locked && busy !== 'disable'}
                  onClick={() => void apply(false)}
                >
                  Выключить
                </Button>
              )}
              {dirty && (
                <Button
                  disabled={locked}
                  onClick={() => {
                    setDraft(getConfig(status));
                    setError('');
                    setNotice('');
                  }}
                >
                  Отменить изменения
                </Button>
              )}
            </Space>
            {!status.installed && (
              <Typography.Text type="secondary">
                Одна кнопка скачает официальный OxiDNS, проверит файл, создаст конфигурацию,
                запустит службу и проверит DNS-запрос. Установка производится только на
                поддерживаемом Linux-сервере.
              </Typography.Text>
            )}
          </Space>
        </Card>

        <Collapse
          activeKey={showAdvanced ? ['public'] : []}
          onChange={(keys) => setShowAdvanced(keys.includes('public'))}
          items={[
            {
              key: 'public',
              label: (
                <Space>
                  <SettingOutlined />
                  <span>Дополнительные настройки: DoH, DoT, DoQ и HTTP/3</span>
                  {external && <Tag color="processing">Включены</Tag>}
                </Space>
              ),
              children: (
                <Space orientation="vertical" size={14} style={{ width: '100%' }}>
                  <Alert
                    type="info"
                    showIcon
                    title="Защищённый DNS для других устройств"
                    description="Необязательно. Публичные протоколы требуют домен, действующий сертификат панели и открытые порты. Панель не меняет firewall. Не включайте внешний DNS, если он вам не нужен."
                  />
                  <Space align="center" style={{ display: 'flex' }}>
                    <Switch
                      aria-label="Включить публичный DoH"
                      checked={draft.doh}
                      disabled={locked}
                      onChange={(doh) => patch({ doh, ...(doh ? {} : { http3: false }) })}
                    />
                    <Typography.Text>DoH — DNS-over-HTTPS (TCP)</Typography.Text>
                  </Space>
                  {draft.doh && (
                    <Space align="center" style={{ display: 'flex' }}>
                      <Switch
                        aria-label="Включить HTTP/3"
                        checked={draft.http3}
                        disabled={locked}
                        onChange={(http3) => patch({ http3 })}
                      />
                      <Typography.Text>HTTP/3 для DoH (дополнительно UDP)</Typography.Text>
                    </Space>
                  )}
                  <Space align="center" style={{ display: 'flex' }}>
                    <Switch
                      aria-label="Включить DoT"
                      checked={draft.dot}
                      disabled={locked}
                      onChange={(dot) => patch({ dot })}
                    />
                    <Typography.Text>DoT — DNS-over-TLS (TCP)</Typography.Text>
                  </Space>
                  <Space align="center" style={{ display: 'flex' }}>
                    <Switch
                      aria-label="Включить DoQ"
                      checked={draft.doq}
                      disabled={locked}
                      onChange={(doq) => patch({ doq })}
                    />
                    <Typography.Text>DoQ — DNS-over-QUIC (UDP)</Typography.Text>
                  </Space>
                  {external && (
                    <>
                      <div>
                        <Typography.Text strong>Домен с TLS-сертификатом панели</Typography.Text>
                        <Input
                          aria-label="Домен защищённого DNS"
                          style={{ marginTop: 6 }}
                          placeholder="dns.example.com"
                          value={draft.domain}
                          status={domainInvalid ? 'error' : undefined}
                          disabled={locked}
                          onChange={(event) => patch({ domain: event.target.value })}
                        />
                        {domainInvalid && (
                          <Typography.Text type="danger">
                            Укажите корректное доменное имя, присутствующее в TLS-сертификате панели.
                          </Typography.Text>
                        )}
                      </div>
                      {draft.doh && (
                        <div>
                          <Typography.Text>HTTPS-порт DoH (TCP; также UDP для HTTP/3)</Typography.Text>
                          <InputNumber
                            aria-label="HTTPS-порт DoH"
                            style={{ display: 'block', marginTop: 6, width: 170 }}
                            min={1}
                            max={65535}
                            value={draft.port}
                            disabled={locked}
                            onChange={(port) => patch({ port: port ?? 8443 })}
                          />
                        </div>
                      )}
                      {draft.dot && (
                        <div>
                          <Typography.Text>Порт DoT (TCP)</Typography.Text>
                          <InputNumber
                            aria-label="Порт DoT"
                            style={{ display: 'block', marginTop: 6, width: 170 }}
                            min={1}
                            max={65535}
                            value={draft.dotPort}
                            disabled={locked}
                            onChange={(dotPort) => patch({ dotPort: dotPort ?? 853 })}
                          />
                        </div>
                      )}
                      {draft.doq && (
                        <div>
                          <Typography.Text>Порт DoQ (UDP)</Typography.Text>
                          <InputNumber
                            aria-label="Порт DoQ"
                            style={{ display: 'block', marginTop: 6, width: 170 }}
                            min={1}
                            max={65535}
                            value={draft.doqPort}
                            disabled={locked}
                            onChange={(doqPort) => patch({ doqPort: doqPort ?? 853 })}
                          />
                        </div>
                      )}
                      {invalidPort && (
                        <Typography.Text type="danger">
                          Порт должен быть от 1 до 65535. Порт 15353 зарезервирован для локального DNS.
                        </Typography.Text>
                      )}
                      {conflictPorts && (
                        <Typography.Text type="danger">
                          Конфликт TCP/UDP-портов. Выберите разные порты для конфликтующих служб.
                        </Typography.Text>
                      )}
                      <Typography.Text type="secondary">
                        Домен проверяется на сервере по TLS-сертификату до запуска. Убедитесь, что DNS
                        домена указывает на сервер и необходимые порты доступны извне.
                      </Typography.Text>
                    </>
                  )}
                </Space>
              ),
            },
          ]}
        />

        {isHealthy && (
          <Card size="small" title="Адреса подключения">
            <Space orientation="vertical" size={4} style={{ width: '100%' }}>
              {endpoint(
                'Локальный DNS для приложений сервера',
                status.localAddress || '127.0.0.1:15353',
                'Доступен только на сервере, не подходит для внешних устройств.',
              )}
              {status.doh && status.dohUrl &&
                endpoint('DoH — HTTPS', status.dohUrl, status.http3
                  ? 'По этому же URL доступен HTTP/3, если его поддерживает клиент.'
                  : 'Вставьте URL в настройки DNS-over-HTTPS клиента.')}
              {status.dot && status.dotAddress &&
                endpoint('DoT — TLS', 'tls://' + status.dotAddress, 'Используйте домен и указанный TCP-порт.')}
              {status.doq && status.doqAddress &&
                endpoint('DoQ — QUIC', 'quic://' + status.doqAddress, 'Используйте домен и указанный UDP-порт.')}
              <Space wrap>
                <Button
                  aria-label="Проверить DNS-запрос"
                  icon={<SafetyCertificateOutlined />}
                  loading={checking}
                  disabled={locked && !checking}
                  onClick={() => void checkDNS()}
                >
                  Проверить DNS-запрос
                </Button>
                <Button aria-label="Обновить состояние" icon={<ReloadOutlined />} disabled={locked} onClick={() => void refresh()}>
                  Обновить состояние
                </Button>
              </Space>
            </Space>
          </Card>
        )}

        <Card size="small" title="Обслуживание OxiDNS">
          <Space orientation="vertical" size={10} style={{ width: '100%' }}>
            <Typography.Text type="secondary">
              {status.installed
                ? 'Установленная версия: ' + (status.installedVersion || 'не определена')
                : 'Компонент ещё не установлен. Он установится автоматически при включении DNS.'}
              {updatesChecked && status.latestVersion
                ? '. Последняя версия: ' + status.latestVersion
                : ''}
            </Typography.Text>
            {updatesChecked && status.error && (
              <Alert type="warning" showIcon title={status.error} />
            )}
            {updatesChecked && status.installed && status.updateAvailable === false && !status.error && (
              <Tag color="success">Последняя версия установлена</Tag>
            )}
            {status.updateAvailable && (
              <Alert type="info" showIcon title="Доступно обновление OxiDNS" />
            )}
            <Space wrap>
              <Button
                aria-label="Проверить обновления"
                icon={<SyncOutlined />}
                loading={checkingUpdates}
                disabled={locked && !checkingUpdates}
                onClick={async () => {
                  setCheckingUpdates(true);
                  try {
                    await refresh(true, true);
                  } finally {
                    setCheckingUpdates(false);
                  }
                }}
              >
                Проверить обновления
              </Button>
              {status.installed && status.updateAvailable && (
                <Button
                  type="primary"
                  aria-label="Обновить OxiDNS"
                  icon={<CloudDownloadOutlined />}
                  loading={busy === 'update'}
                  disabled={locked && busy !== 'update'}
                  onClick={() => void update()}
                >
                  Обновить OxiDNS
                </Button>
              )}
              {status.installed && (
                <Button disabled={locked} onClick={() => void openLogs()}>
                  Журнал службы
                </Button>
              )}
              {status.installed && (
                <Popconfirm
                  title="Удалить OxiDNS полностью?"
                  description="Служба будет остановлена, её файлы и настройки удалены, а sing-box переключится с OxiDNS."
                  okText="Удалить"
                  okButtonProps={{ danger: true }}
                  cancelText="Отмена"
                  onConfirm={() => void remove()}
                >
                  <Button
                    danger
                    aria-label="Удалить OxiDNS"
                    icon={<DeleteOutlined />}
                    loading={busy === 'remove'}
                    disabled={locked && busy !== 'remove'}
                  >
                    Удалить OxiDNS
                  </Button>
                </Popconfirm>
              )}
            </Space>
            <Typography.Text type="secondary">
              Установка, обновление и удаление также доступны в «Настройки → Обновления пакетов и ядер Linux».
            </Typography.Text>
          </Space>
        </Card>
      </Space>
      <Modal
        title="Журнал службы OxiDNS — последние 80 записей"
        open={logsOpen}
        width={840}
        footer={<Button onClick={() => setLogsOpen(false)}>Закрыть</Button>}
        onCancel={() => setLogsOpen(false)}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Text type="secondary">
            Журнал предназначен для диагностики. Не публикуйте его без проверки на личные данные.
          </Typography.Text>
          <Input.TextArea
            aria-label="Журнал OxiDNS"
            readOnly
            value={logsBusy ? 'Загрузка журнала...' : logs}
            autoSize={{ minRows: 10, maxRows: 24 }}
            style={{ fontFamily: 'monospace', fontSize: 12 }}
          />
          <Button onClick={() => void openLogs()} loading={logsBusy}>Обновить журнал</Button>
        </Space>
      </Modal>
    </Card>
  );
}
