import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Divider,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
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
  const [busy, setBusy] = useState<'apply' | 'disable' | null>(null);
  const [checking, setChecking] = useState(false);
  const [verified, setVerified] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showMaintenance, setShowMaintenance] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [logsBusy, setLogsBusy] = useState(false);
  const [logs, setLogs] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const refresh = useCallback(
    async (keepDraft = false, keepError = false) => {
      try {
        const response = (await HttpUtil.get(
          '/panel/api/oxidns/status',
          undefined,
          { silent: true, timeout: 10000 },
        )) as APIResponse;
        if (!response.success || !response.obj)
          throw new Error(response.msg || 'Не удалось получить состояние DNS.');
        setStatus(response.obj);
        if (!keepDraft) {
          setDraft(getConfig(response.obj));
          if (response.obj.doh || response.obj.dot || response.obj.doq) setShowAdvanced(true);
        }
        if (!keepError) setError('');
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Не удалось получить состояние DNS.');
      } finally {
        setLoading(false);
      }
    },
    [],
  );

  useEffect(() => {
    let active = true;
    void Promise.resolve().then(() => {
      if (active) void refresh();
    });
    const onChanged = () => {
      setNotice('');
      void refresh();
    };
    window.addEventListener('oxidns:changed', onChanged);
    return () => {
      active = false;
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
  const domainInvalid =
    external && !/^(?=.{4,253}$)[a-z0-9-]+(?:\.[a-z0-9-]+)+$/i.test(draft.domain.trim());
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
  const configMode = !external ? 'local' : draft.doh && draft.dot && draft.doq ? 'all' : 'custom';
  const modeDescription = !external
    ? 'Рекомендуется для VPN: OxiDNS будет автоматически использоваться в sing-box. Домен, сертификат и открытые порты не требуются.'
    : draft.doh && !draft.dot && !draft.doq
      ? 'Для телефонов и браузеров: появится личный адрес DNS-over-HTTPS. Потребуется домен, действующий сертификат и открытый HTTPS-порт.'
      : 'Расширенный режим: включены дополнительные DNS-протоколы. Проверьте домен, TLS-сертификат и порты перед запуском.';
  const setupHint = domainInvalid
    ? 'Укажите домен DNS, который уже прописан в действующем TLS-сертификате панели. Для локального режима домен не нужен.'
    : invalidPort || conflictPorts
      ? 'Измените порты: они должны быть допустимыми и не пересекаться у одинакового транспорта.'
      : unused
        ? 'Включите использование в sing-box либо выберите внешний защищённый DNS.'
        : '';
  const chooseMode = (mode: 'local' | 'doh' | 'all') => {
    if (mode === 'local') {
      patch({ useSingBox: true, doh: false, dot: false, doq: false, http3: false });
      setShowAdvanced(false);
    } else if (mode === 'doh') {
      patch({ useSingBox: true, doh: true, http3: false, dot: false, doq: false });
      setShowAdvanced(true);
    } else {
      patch({ useSingBox: true, doh: true, http3: true, dot: true, doq: true });
      setShowAdvanced(true);
    }
  };
  const locked = busy !== null || checking;
  const canApply =
    status.installed &&
    status.supported &&
    !domainInvalid &&
    !invalidPort &&
    !conflictPorts &&
    !unused;
  const isHealthy = status.installed && status.enabled && status.running;
  const primaryLabel = !status.enabled
    ? 'Создать и включить DNS'
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
      const next = await request(
        enable ? 'setup' : 'disable',
        enable ? { ...draft, domain: draft.domain.trim() } : undefined,
      );
      setStatus(next);
      setDraft(getConfig(next));
      setVerified(enable && next.running);
      window.dispatchEvent(new Event('oxidns:changed'));
      setNotice(
        enable
          ? 'OxiDNS запущен и проверен. Новая конфигурация применяется автоматически.'
          : 'DNS отключён. Настройки сохранены для следующего запуска.',
      );
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось изменить DNS.');
      await refresh(true, true);
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
      <Typography.Text strong style={{ display: 'block' }}>
        {title}
      </Typography.Text>
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
          Создайте DNS-сервис на сервере без ручного редактирования конфигурации. OxiDNS получает
          ответы от выбранного защищённого DNS-провайдера и кэширует их. Для VPN через sing-box
          достаточно выбрать провайдера и включить DNS — домен не нужен.
        </Typography.Paragraph>

        {!status.installed ? (
          <>
            {error && <Alert type="error" showIcon title="Не удалось проверить OxiDNS" description={error} />}
            <Alert
              type="info"
              showIcon
              title="Для работы требуется установка OxiDNS"
              description={
                <Typography.Paragraph style={{ marginBottom: 0 }}>
                  Откройте «Настройки → Системные пакеты и ядро Linux → OxiDNS» и нажмите
                  «Установить OxiDNS». После установки вернитесь сюда: появятся настройки для
                  создания собственного DNS. Установка не включает DNS автоматически.
                </Typography.Paragraph>
              }
            />
          </>
        ) : (
          <>

        {status.supported === false && (
          <Alert
            type="warning"
            showIcon
            title="OxiDNS не поддерживается на этой системе"
            description={
              status.unsupportedReason || 'Требуется Linux с запущенным systemd и права root.'
            }
          />
        )}
        {error && (
          <Alert
            type="error"
            showIcon
            title="Операция не выполнена"
            description={error}
            action={
              <Button size="small" onClick={() => void refresh(true)}>
                Обновить статус
              </Button>
            }
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
            description={
              status.useSingBox
                ? 'DNS подключён к sing-box. Существующие правила DNS не удалены.'
                : 'OxiDNS работает отдельно от sing-box. Включите подключение ниже при необходимости.'
            }
          />
        )}

        <Card
          size="small"
          title={isHealthy ? 'Настройки DNS' : 'Создание собственного DNS'}
        >
          <Space orientation="vertical" size={14} style={{ width: '100%' }}>
            <div>
              <Typography.Text strong>Шаг 1. Для чего нужен DNS?</Typography.Text>
              <Typography.Paragraph type="secondary" style={{ marginBottom: 8 }}>
                Выберите задачу: необходимые параметры будут выставлены автоматически.
              </Typography.Paragraph>
              <Space wrap>
                <Button
                  type={configMode === 'local' ? 'primary' : 'default'}
                  disabled={locked}
                  onClick={() => chooseMode('local')}
                >
                  Для VPN / sing-box
                </Button>
                <Button
                  type={draft.doh && !draft.dot && !draft.doq ? 'primary' : 'default'}
                  disabled={locked}
                  onClick={() => chooseMode('doh')}
                >
                  Для телефона и браузера (DoH)
                </Button>
                <Button
                  type={configMode === 'all' ? 'primary' : 'default'}
                  disabled={locked}
                  onClick={() => chooseMode('all')}
                >
                  Все протоколы (опытным)
                </Button>
              </Space>
              <Alert style={{ marginTop: 12 }} type="info" showIcon title={modeDescription} />
            </div>
            <Divider style={{ margin: '2px 0' }} />
            <div>
              <Typography.Text strong>Шаг 2. Откуда получать DNS-ответы?</Typography.Text>
              <Select
                aria-label="DNS-провайдер"
                style={{ width: '100%', marginTop: 8 }}
                value={draft.upstream}
                options={providers}
                disabled={locked}
                onChange={(upstream) => patch({ upstream })}
              />
              <Typography.Text type="secondary">
                Это внешний защищённый DNS, через который OxiDNS разрешает домены. Связь с ним шифруется по DNS-over-TLS (TCP/853).
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

            {setupHint && (
              <Alert type="warning" showIcon title="Проверьте настройку" description={setupHint} />
            )}
            {unused && (
              <Alert
                type="warning"
                showIcon
                title="DNS не будет использоваться"
                description="Подключите sing-box или включите хотя бы один публичный протокол в дополнительных настройках."
              />
            )}
            {dirty && status.enabled && (
              <Alert type="info" showIcon title="Настройки изменены, но ещё не применены" />
            )}
            <Typography.Text strong>Шаг 3. Создайте и запустите DNS</Typography.Text>
            <Space wrap>
              <Button
                type="primary"
                size="large"
                aria-label={primaryLabel}
                icon={<CheckCircleOutlined />}
                loading={busy === 'apply'}
                disabled={(locked && busy !== 'apply') || !canApply}
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
                    description="Для DNS на телефоне или в браузере нужны домен с A/AAAA-записью на сервер, действующий TLS-сертификат панели и открытые порты TCP/UDP. Панель создаёт службу и адрес подключения, но не регистрирует домен, не выпускает сертификат и не меняет firewall. Для VPN через sing-box это не нужно."
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
                            Укажите корректное доменное имя, присутствующее в TLS-сертификате
                            панели.
                          </Typography.Text>
                        )}
                      </div>
                      {draft.doh && (
                        <div>
                          <Typography.Text>
                            HTTPS-порт DoH (TCP; также UDP для HTTP/3)
                          </Typography.Text>
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
                          Порт должен быть от 1 до 65535. Порт 15353 зарезервирован для локального
                          DNS.
                        </Typography.Text>
                      )}
                      {conflictPorts && (
                        <Typography.Text type="danger">
                          Конфликт TCP/UDP-портов. Выберите разные порты для конфликтующих служб.
                        </Typography.Text>
                      )}
                      <Typography.Text type="secondary">
                        Домен проверяется на сервере по TLS-сертификату до запуска. Убедитесь, что
                        DNS домена указывает на сервер и необходимые порты доступны извне.
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
              {status.doh &&
                status.dohUrl &&
                endpoint(
                  'DoH — HTTPS',
                  status.dohUrl,
                  status.http3
                    ? 'По этому же URL доступен HTTP/3, если его поддерживает клиент.'
                    : 'Вставьте URL в настройки DNS-over-HTTPS клиента.',
                )}
              {status.dot &&
                status.dotAddress &&
                endpoint(
                  'DoT — TLS',
                  'tls://' + status.dotAddress,
                  'Используйте домен и указанный TCP-порт.',
                )}
              {status.doq &&
                status.doqAddress &&
                endpoint(
                  'DoQ — QUIC',
                  'quic://' + status.doqAddress,
                  'Используйте домен и указанный UDP-порт.',
                )}
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
                <Button
                  aria-label="Обновить состояние"
                  icon={<ReloadOutlined />}
                  disabled={locked}
                  onClick={() => void refresh()}
                >
                  Обновить состояние
                </Button>
              </Space>
            </Space>
          </Card>
        )}

        <Collapse
          activeKey={showMaintenance ? ['maintenance'] : []}
          onChange={(keys) => setShowMaintenance(keys.includes('maintenance'))}
          items={[{
            key: 'maintenance',
            label: <Space><SettingOutlined /> Диагностика OxiDNS</Space>,
            children: (
              <Space orientation="vertical" style={{ width: '100%' }}>
                <Typography.Text type="secondary">
                  Версия OxiDNS: {status.installedVersion || 'не определена'}. Для обновления или удаления
                  откройте «Настройки → Системные пакеты и ядро Linux».
                </Typography.Text>
                <Button onClick={() => void openLogs()}>Журнал службы</Button>
              </Space>
            ),
          }]}
        />
        </>
        )}
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
          <Button onClick={() => void openLogs()} loading={logsBusy}>
            Обновить журнал
          </Button>
        </Space>
      </Modal>
    </Card>
  );
}
