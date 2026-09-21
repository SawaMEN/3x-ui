import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useLocation } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  ConfigProvider,
  Divider,
  FloatButton,
  Input,
  InputNumber,
  Layout,
  Modal,
  Result,
  Row,
  Col,
  Select,
  Space,
  Spin,
  Switch,
  Tag,
  message,
} from 'antd';
import {
  CodeOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import './SingBoxPage.css';

type SectionKey =
  | 'schema'
  | 'log'
  | 'dns'
  | 'ntp'
  | 'certificate'
  | 'certificate_providers'
  | 'http_clients'
  | 'network_namespaces'
  | 'endpoints'
  | 'outbounds'
  | 'route'
  | 'experimental';

type JsonObject = Record<string, unknown>;
type ConfigMap = JsonObject;
type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };

type Snapshot = {
  config: ConfigMap;
  running: boolean;
  version: string;
  configPath: string;
  configSource: 'disk' | 'generated' | string;
  configModified: string;
  managedSections: string[];
};

const sectionFallbacks: Record<SectionKey, unknown> = {
  schema: '',
  log: {},
  dns: { servers: [] },
  ntp: {},
  certificate: {},
  certificate_providers: [],
  http_clients: [],
  network_namespaces: [],
  endpoints: [],
  outbounds: [],
  route: {},
  experimental: {},
};

const outboundTypes = [
  'direct',
  'block',
  'dns',
  'http',
  'socks',
  'shadowsocks',
  'vmess',
  'vless',
  'trojan',
  'hysteria2',
  'tuic',
  'selector',
  'urltest',
  'tun',
  'redirect',
  'tproxy',
  'shadowtls',
  'ssh',
] as const;

const dnsTypes = [
  'local',
  'hosts',
  'tcp',
  'udp',
  'tls',
  'quic',
  'https',
  'h3',
  'dhcp',
  'mdns',
  'fakeip',
  'tailscale',
  'openconnect',
  'openvpn',
  'resolved',
] as const;

function asObject(value: unknown): JsonObject {
  return value && typeof value === 'object' && !Array.isArray(value) ? (value as JsonObject) : {};
}

function asObjectArray(value: unknown): JsonObject[] {
  return Array.isArray(value) ? value.map(asObject) : [];
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function asBoolean(value: unknown, fallback = false): boolean {
  return typeof value === 'boolean' ? value : fallback;
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : [];
}

function prettyJson(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}

function Field({
  label,
  children,
  span = 12,
}: {
  label: string;
  children: ReactNode;
  span?: number;
}) {
  return (
    <Col xs={24} md={span}>
      <div className="singbox-field">
        <div className="singbox-field-label">{label}</div>
        {children}
      </div>
    </Col>
  );
}

function TextField({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <Input
      value={value}
      placeholder={placeholder}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

function NumberField({
  value,
  onChange,
  min = 0,
  max,
}: {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
}) {
  return (
    <InputNumber
      style={{ width: '100%' }}
      value={value}
      min={min}
      max={max}
      onChange={(value) => onChange(typeof value === 'number' ? value : 0)}
    />
  );
}

function ToggleField({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  return <Switch checked={checked} onChange={onChange} />;
}

function StringListField({
  value,
  onChange,
  placeholder,
}: {
  value: string[];
  onChange: (value: string[]) => void;
  placeholder?: string;
}) {
  return (
    <Input
      value={value.join(', ')}
      placeholder={placeholder}
      onChange={(event) =>
        onChange(
          event.target.value
            .split(',')
            .map((item) => item.trim())
            .filter(Boolean),
        )
      }
    />
  );
}

function JsonModal({
  title,
  value,
  onApply,
}: {
  title: string;
  value: unknown;
  onApply: (value: unknown) => void;
}) {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState('');
  const [error, setError] = useState('');

  const openEditor = () => {
    setText(prettyJson(value));
    setError('');
    setOpen(true);
  };

  const apply = () => {
    try {
      const parsed = JSON.parse(text);
      onApply(parsed);
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <>
      <Button icon={<CodeOutlined />} onClick={openEditor}>
        JSON
      </Button>
      <Modal
        open={open}
        title={title}
        width={960}
        okText="Применить"
        cancelText="Отмена"
        onCancel={() => setOpen(false)}
        onOk={apply}
      >
        {error && <Alert type="error" showIcon message={error} style={{ marginBottom: 12 }} />}
        <Input.TextArea
          value={text}
          onChange={(event) => setText(event.target.value)}
          spellCheck={false}
          autoSize={{ minRows: 18, maxRows: 32 }}
          style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}
        />
      </Modal>
    </>
  );
}

function SectionHeader({
  title,
  description,
  onJson,
  onReset,
}: {
  title: string;
  description?: string;
  onJson?: () => void;
  onReset?: () => void;
}) {
  return (
    <div className="singbox-section-header">
      <div>
        <div className="singbox-section-title">{title}</div>
        {description && <div className="singbox-section-description">{description}</div>}
      </div>
      <Space wrap>
        {onReset && (
          <Button icon={<ReloadOutlined />} onClick={onReset}>
            Сбросить раздел
          </Button>
        )}
        {onJson && (
          <Button icon={<CodeOutlined />} onClick={onJson}>
            Расширенный JSON
          </Button>
        )}
      </Space>
    </div>
  );
}

function ListToolbar({ label, onAdd }: { label: string; onAdd: () => void }) {
  return (
    <div className="singbox-list-toolbar">
      <strong>{label}</strong>
      <Button type="primary" icon={<PlusOutlined />} onClick={onAdd}>
        Добавить
      </Button>
    </div>
  );
}

function CommonListItem({
  item,
  index,
  fields,
  onChange,
  onDelete,
  title,
  children,
}: {
  item: JsonObject;
  index: number;
  fields: Array<{ key: string; label: string; type?: 'number' | 'text' }>;
  onChange: (index: number, patch: JsonObject) => void;
  onDelete: (index: number) => void;
  title: string;
  children?: ReactNode;
}) {
  return (
    <Card size="small" className="singbox-item-card">
      <div className="singbox-item-card-header">
        <strong>{title}</strong>
        <Button danger type="text" icon={<DeleteOutlined />} onClick={() => onDelete(index)} />
      </div>
      <Row gutter={[12, 12]}>
        {fields.map((field) => (
          <Field key={field.key} label={field.label}>
            {field.type === 'number' ? (
              <NumberField
                value={asNumber(item[field.key])}
                onChange={(value) => onChange(index, { [field.key]: value })}
                min={0}
                max={65535}
              />
            ) : (
              <TextField
                value={asString(item[field.key])}
                onChange={(value) => onChange(index, { [field.key]: value })}
              />
            )}
          </Field>
        ))}
      </Row>
      {children && <div className="singbox-item-actions">{children}</div>}
    </Card>
  );
}

export default function SingBoxPage() {
  const { t } = useTranslation();
  const [messageApi, contextHolder] = message.useMessage();
  const { antdThemeConfig, isDark, isUltra } = useTheme();
  const location = useLocation();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [config, setConfig] = useState<ConfigMap>({});
  const sectionSlug = location.hash.replace(/^#/, '');
  const sectionKeys = useMemo(
    () =>
      new Set([
        'basic',
        'dns',
        'routing',
        'outbound',
        'endpoints',
        'certificates',
        'network',
        'advanced',
      ]),
    [],
  );
  const activeSection = sectionKeys.has(sectionSlug) ? sectionSlug : 'basic';
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fetchError, setFetchError] = useState('');

  const sectionValue = useCallback((key: SectionKey, source: ConfigMap) => {
    const sourceKey = key === 'schema' ? '$schema' : key;
    return source[sourceKey] ?? sectionFallbacks[key];
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    setFetchError('');
    try {
      const msg = (await HttpUtil.get('/panel/api/setting/singbox/config')) as ApiMsg<Snapshot>;
      if (!msg?.success || !msg.obj?.config) {
        throw new Error(msg?.msg || 'Не удалось загрузить настройки sing-box');
      }
      setSnapshot(msg.obj);
      setConfig(msg.obj.config);
    } catch (error) {
      setFetchError(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const initial = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(initial);
  }, [refresh]);

  const updateSection = useCallback((key: SectionKey, value: unknown) => {
    setConfig((current) => ({
      ...current,
      [key === 'schema' ? '$schema' : key]: value,
    }));
  }, []);

  const patchSection = useCallback((key: SectionKey, patch: JsonObject) => {
    setConfig((current) => {
      const currentValue = asObject(current[key]);
      return {
        ...current,
        [key]: { ...currentValue, ...patch },
      };
    });
  }, []);

  const save = async () => {
    try {
      const next: ConfigMap = { ...config };
      delete next.inbounds;
      delete next.services;

      setSaving(true);
      const msg = (await HttpUtil.post('/panel/api/setting/singbox/config', {
        config: JSON.stringify(next),
      })) as ApiMsg;
      if (!msg?.success) throw new Error(msg?.msg || 'Не удалось сохранить настройки sing-box');
      messageApi.success(t('pages.singBox.saved'));
      await refresh();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    setSaving(true);
    try {
      const msg = (await HttpUtil.post('/panel/api/setting/singbox/config/reset')) as ApiMsg;
      if (!msg?.success)
        throw new Error(msg?.msg || 'Не удалось вернуть сгенерированные настройки');
      messageApi.success(t('pages.singBox.resetDone'));
      await refresh();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };

  const renderLog = () => {
    const value = asObject(sectionValue('log', config));
    return (
      <>
        <SectionHeader
          title="Логирование"
          description="Основные параметры журналирования sing-box."
          onReset={() => updateSection('log', sectionFallbacks.log)}
        />
        <Card>
          <Row gutter={[12, 16]}>
            <Field label="Логирование отключено">
              <ToggleField
                checked={asBoolean(value.disabled)}
                onChange={(next) => patchSection('log', { disabled: next })}
              />
            </Field>
            <Field label="Уровень">
              <Select
                style={{ width: '100%' }}
                value={asString(value.level) || 'info'}
                options={['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'].map((v) => ({
                  label: v,
                  value: v,
                }))}
                onChange={(next) => patchSection('log', { level: next })}
              />
            </Field>
            <Field label="Файл вывода" span={24}>
              <TextField
                value={asString(value.output)}
                placeholder="box.log"
                onChange={(next) => patchSection('log', { output: next })}
              />
            </Field>
            <Field label="Добавлять timestamp">
              <ToggleField
                checked={asBoolean(value.timestamp, true)}
                onChange={(next) => patchSection('log', { timestamp: next })}
              />
            </Field>
          </Row>
        </Card>
        <div className="singbox-json-action">
          <JsonModal
            title="Лог — расширенный JSON"
            value={value}
            onApply={(next) => updateSection('log', asObject(next))}
          />
        </div>
      </>
    );
  };

  const renderDns = () => {
    const value = asObject(sectionValue('dns', config));
    const servers = asObjectArray(value.servers);
    const rules = Array.isArray(value.rules) ? value.rules : [];
    const updateServer = (index: number, patch: JsonObject) => {
      const next = [...servers];
      next[index] = { ...next[index], ...patch };
      updateSection('dns', { ...value, servers: next });
    };
    return (
      <>
        <SectionHeader
          title="DNS"
          description="Серверы DNS, стратегия и основные параметры кэша."
          onReset={() => updateSection('dns', sectionFallbacks.dns)}
        />
        <Card>
          <Row gutter={[12, 16]}>
            <Field label="Финальный DNS">
              <TextField
                value={asString(value.final)}
                onChange={(next) => patchSection('dns', { final: next })}
              />
            </Field>
            <Field label="Стратегия">
              <Select
                style={{ width: '100%' }}
                value={asString(value.strategy) || undefined}
                placeholder="Выберите стратегию"
                options={[
                  { label: 'prefer_ipv4', value: 'prefer_ipv4' },
                  { label: 'prefer_ipv6', value: 'prefer_ipv6' },
                  { label: 'ipv4_only', value: 'ipv4_only' },
                  { label: 'ipv6_only', value: 'ipv6_only' },
                ]}
                onChange={(next) => patchSection('dns', { strategy: next })}
              />
            </Field>
            <Field label="Отключить кэш">
              <ToggleField
                checked={asBoolean(value.disable_cache)}
                onChange={(next) => patchSection('dns', { disable_cache: next })}
              />
            </Field>
            <Field label="Отключить истечение кэша">
              <ToggleField
                checked={asBoolean(value.disable_expire)}
                onChange={(next) => patchSection('dns', { disable_expire: next })}
              />
            </Field>
            <Field label="Оптимистическое кэширование">
              <ToggleField
                checked={asBoolean(value.optimistic)}
                onChange={(next) => patchSection('dns', { optimistic: next })}
              />
            </Field>
            <Field label="Обратное сопоставление">
              <ToggleField
                checked={asBoolean(value.reverse_mapping)}
                onChange={(next) => patchSection('dns', { reverse_mapping: next })}
              />
            </Field>
            <Field label="Client subnet">
              <TextField
                value={asString(value.client_subnet)}
                onChange={(next) => patchSection('dns', { client_subnet: next })}
              />
            </Field>
            <Field label="Кэш, ёмкость">
              <NumberField
                value={asNumber(value.cache_capacity)}
                onChange={(next) => patchSection('dns', { cache_capacity: next })}
              />
            </Field>
          </Row>
        </Card>
        <Card className="singbox-list-card">
          <ListToolbar
            label="DNS-серверы"
            onAdd={() =>
              updateSection('dns', {
                ...value,
                servers: [...servers, { type: 'local', tag: `dns-${servers.length + 1}` }],
              })
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {servers.map((server, index) => (
              <CommonListItem
                key={`dns-${index}`}
                item={server}
                index={index}
                title={`${asString(server.tag) || 'DNS'} #${index + 1}`}
                fields={[
                  { key: 'tag', label: 'Tag' },
                  { key: 'server', label: 'Server' },
                  { key: 'server_port', label: 'Port', type: 'number' },
                  { key: 'detour', label: 'Detour' },
                  { key: 'domain_resolver', label: 'Domain resolver' },
                ]}
                onChange={(i, patch) =>
                  updateServer(i, {
                    ...patch,
                    ...(patch.type ? {} : {}),
                  })
                }
                onDelete={(i) =>
                  updateSection('dns', { ...value, servers: servers.filter((_, j) => j !== i) })
                }
              >
                <Select
                  style={{ width: 220 }}
                  value={asString(server.type) || 'local'}
                  options={dnsTypes.map((item) => ({ label: item, value: item }))}
                  onChange={(next) => updateServer(index, { type: next })}
                />
              </CommonListItem>
            ))}
          </Space>
          {servers.length > 0 && (
            <div className="singbox-inline-extra">
              <Select
                style={{ width: 220 }}
                value={asString(servers[0].type) || 'local'}
                options={dnsTypes.map((item) => ({ label: item, value: item }))}
                onChange={(next) => updateServer(0, { type: next })}
              />
              <JsonModal
                title="DNS-серверы — расширенный JSON"
                value={servers}
                onApply={(next) => updateSection('dns', { ...value, servers: asObjectArray(next) })}
              />
            </div>
          )}
        </Card>
        <Card>
          <SectionHeader
            title="DNS-правила"
            description="Сложные правила можно редактировать полностью в JSON."
          />
          <JsonModal
            title="DNS-правила"
            value={rules}
            onApply={(next) => patchSection('dns', { rules: Array.isArray(next) ? next : [] })}
          />
        </Card>
      </>
    );
  };

  const renderNtp = () => {
    const value = asObject(sectionValue('ntp', config));
    return (
      <>
        <SectionHeader title="NTP" description="Синхронизация времени для протоколов sing-box." />
        <Card>
          <Row gutter={[12, 16]}>
            <Field label="Включено">
              <ToggleField
                checked={asBoolean(value.enabled)}
                onChange={(next) => patchSection('ntp', { enabled: next })}
              />
            </Field>
            <Field label="NTP-сервер">
              <TextField
                value={asString(value.server)}
                onChange={(next) => patchSection('ntp', { server: next })}
              />
            </Field>
            <Field label="Порт">
              <NumberField
                value={asNumber(value.server_port, 123)}
                onChange={(next) => patchSection('ntp', { server_port: next })}
                max={65535}
              />
            </Field>
            <Field label="Интервал">
              <TextField
                value={asString(value.interval)}
                onChange={(next) => patchSection('ntp', { interval: next })}
                placeholder="30m"
              />
            </Field>
            <Field label="Domain resolver">
              <TextField
                value={asString(value.domain_resolver)}
                onChange={(next) => patchSection('ntp', { domain_resolver: next })}
              />
            </Field>
            <Field label="Detour">
              <TextField
                value={asString(value.detour)}
                onChange={(next) => patchSection('ntp', { detour: next })}
              />
            </Field>
          </Row>
        </Card>
        <div className="singbox-json-action">
          <JsonModal
            title="NTP — расширенный JSON"
            value={value}
            onApply={(next) => updateSection('ntp', asObject(next))}
          />
        </div>
      </>
    );
  };

  const renderCertificate = () => {
    const value = asObject(sectionValue('certificate', config));
    return (
      <>
        <SectionHeader title="Сертификаты" description="Доверенные CA и пути к сертификатам." />
        <Card>
          <Row gutter={[12, 16]}>
            <Field label="Хранилище">
              <Select
                style={{ width: '100%' }}
                value={asString(value.store) || 'system'}
                options={['system', 'mozilla', 'chrome', 'none'].map((v) => ({
                  label: v,
                  value: v,
                }))}
                onChange={(next) => patchSection('certificate', { store: next })}
              />
            </Field>
            <Field label="Каталог сертификатов" span={24}>
              <StringListField
                value={asStringArray(value.certificate_directory_path)}
                onChange={(next) =>
                  patchSection('certificate', { certificate_directory_path: next })
                }
              />
            </Field>
            <Field label="Пути сертификатов" span={24}>
              <StringListField
                value={asStringArray(value.certificate_path)}
                onChange={(next) => patchSection('certificate', { certificate_path: next })}
              />
            </Field>
          </Row>
        </Card>
        <div className="singbox-json-action">
          <JsonModal
            title="Сертификаты — расширенный JSON"
            value={value}
            onApply={(next) => updateSection('certificate', asObject(next))}
          />
        </div>
      </>
    );
  };

  const renderCertificateProviders = () => {
    const items = asObjectArray(sectionValue('certificate_providers', config));
    return (
      <>
        <SectionHeader
          title="Провайдеры сертификатов"
          description="ACME, Tailscale и Cloudflare Origin CA."
        />
        <Card className="singbox-list-card">
          <ListToolbar
            label="Провайдеры"
            onAdd={() =>
              updateSection('certificate_providers', [
                ...items,
                { type: 'acme', tag: `cert-${items.length + 1}`, domain: [] },
              ])
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {items.map((item, index) => (
              <Card size="small" className="singbox-item-card" key={`cert-provider-${index}`}>
                <div className="singbox-item-card-header">
                  <strong>{asString(item.tag) || `Провайдер #${index + 1}`}</strong>
                  <Button
                    danger
                    type="text"
                    icon={<DeleteOutlined />}
                    onClick={() =>
                      updateSection(
                        'certificate_providers',
                        items.filter((_, i) => i !== index),
                      )
                    }
                  />
                </div>
                <Row gutter={[12, 12]}>
                  <Field label="Тип">
                    <Select
                      style={{ width: '100%' }}
                      value={asString(item.type) || 'acme'}
                      options={['acme', 'tailscale', 'cloudflare-origin-ca'].map((v) => ({
                        label: v,
                        value: v,
                      }))}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, type: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                  <Field label="Tag">
                    <TextField
                      value={asString(item.tag)}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, tag: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                  <Field label="Домены">
                    <StringListField
                      value={asStringArray(item.domain)}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, domain: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                  <Field label="Email">
                    <TextField
                      value={asString(item.email)}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, email: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                  <Field label="Data directory">
                    <TextField
                      value={asString(item.data_directory)}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, data_directory: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                  <Field label="Provider">
                    <TextField
                      value={asString(item.provider)}
                      onChange={(next) => {
                        const nextItems = [...items];
                        nextItems[index] = { ...item, provider: next };
                        updateSection('certificate_providers', nextItems);
                      }}
                    />
                  </Field>
                </Row>
                <div className="singbox-item-actions">
                  <JsonModal
                    title={`${asString(item.tag) || 'Провайдер'} — JSON`}
                    value={item}
                    onApply={(next) => {
                      const nextItems = [...items];
                      nextItems[index] = asObject(next);
                      updateSection('certificate_providers', nextItems);
                    }}
                  />
                </div>
              </Card>
            ))}
          </Space>
        </Card>
      </>
    );
  };

  const renderHttpClients = () => {
    const items = asObjectArray(sectionValue('http_clients', config));
    const updateItem = (index: number, patch: JsonObject) => {
      const next = [...items];
      next[index] = { ...next[index], ...patch };
      updateSection('http_clients', next);
    };
    return (
      <>
        <SectionHeader
          title="HTTP-клиенты"
          description="Общие HTTP-клиенты для удалённых rule-set и служебных запросов."
        />
        <Card className="singbox-list-card">
          <ListToolbar
            label="HTTP-клиенты"
            onAdd={() =>
              updateSection('http_clients', [
                ...items,
                { tag: `http-${items.length + 1}`, engine: 'go', version: 2 },
              ])
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {items.map((item, index) => {
              const tls = asObject(item.tls);
              return (
                <Card size="small" className="singbox-item-card" key={`http-client-${index}`}>
                  <div className="singbox-item-card-header">
                    <strong>{asString(item.tag) || `HTTP-клиент #${index + 1}`}</strong>
                    <Button
                      danger
                      type="text"
                      icon={<DeleteOutlined />}
                      onClick={() =>
                        updateSection(
                          'http_clients',
                          items.filter((_, i) => i !== index),
                        )
                      }
                    />
                  </div>
                  <Row gutter={[12, 12]}>
                    <Field label="Tag">
                      <TextField
                        value={asString(item.tag)}
                        onChange={(next) => updateItem(index, { tag: next })}
                      />
                    </Field>
                    <Field label="Engine">
                      <Select
                        style={{ width: '100%' }}
                        value={asString(item.engine) || 'go'}
                        options={['go', 'apple'].map((v) => ({ label: v, value: v }))}
                        onChange={(next) => updateItem(index, { engine: next })}
                      />
                    </Field>
                    <Field label="HTTP version">
                      <NumberField
                        value={asNumber(item.version, 2)}
                        min={1}
                        max={3}
                        onChange={(next) => updateItem(index, { version: next })}
                      />
                    </Field>
                    <Field label="Disable fallback">
                      <ToggleField
                        checked={asBoolean(item.disable_version_fallback)}
                        onChange={(next) => updateItem(index, { disable_version_fallback: next })}
                      />
                    </Field>
                    <Field label="TLS server name">
                      <TextField
                        value={asString(tls.server_name)}
                        onChange={(next) =>
                          updateItem(index, { tls: { ...tls, server_name: next } })
                        }
                      />
                    </Field>
                    <Field label="TLS insecure">
                      <ToggleField
                        checked={asBoolean(tls.insecure)}
                        onChange={(next) => updateItem(index, { tls: { ...tls, insecure: next } })}
                      />
                    </Field>
                    <Field label="Headers">
                      <JsonModal
                        title={`${asString(item.tag) || 'HTTP-клиент'} — Headers`}
                        value={item.headers || {}}
                        onApply={(next) => updateItem(index, { headers: asObject(next) })}
                      />
                    </Field>
                  </Row>
                  <div className="singbox-item-actions">
                    <JsonModal
                      title={`${asString(item.tag) || 'HTTP-клиент'} — JSON`}
                      value={item}
                      onApply={(next) => updateItem(index, asObject(next))}
                    />
                  </div>
                </Card>
              );
            })}
          </Space>
        </Card>
      </>
    );
  };

  const renderNamespaces = () => {
    const items = asObjectArray(sectionValue('network_namespaces', config));
    return (
      <>
        <SectionHeader
          title="Сетевые пространства"
          description="Linux network namespaces для tun, listen и dial."
        />
        <Card className="singbox-list-card">
          <ListToolbar
            label="Namespaces"
            onAdd={() =>
              updateSection('network_namespaces', [
                ...items,
                { type: 'default', tag: `ns-${items.length + 1}` },
              ])
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {items.map((item, index) => (
              <CommonListItem
                key={`namespace-${index}`}
                item={item}
                index={index}
                title={asString(item.tag) || `Namespace #${index + 1}`}
                fields={[{ key: 'tag', label: 'Tag' }]}
                onChange={(i, patch) => {
                  const next = [...items];
                  next[i] = { ...next[i], ...patch };
                  updateSection('network_namespaces', next);
                }}
                onDelete={(i) =>
                  updateSection(
                    'network_namespaces',
                    items.filter((_, j) => j !== i),
                  )
                }
              >
                <Select
                  style={{ width: 180 }}
                  value={asString(item.type) || 'default'}
                  options={['default', 'unshare'].map((v) => ({ label: v, value: v }))}
                  onChange={(next) => {
                    const nextItems = [...items];
                    nextItems[index] = { ...nextItems[index], type: next };
                    updateSection('network_namespaces', nextItems);
                  }}
                />
              </CommonListItem>
            ))}
          </Space>
          {items.length > 0 && (
            <div className="singbox-inline-extra">
              <JsonModal
                title="Namespaces — JSON"
                value={items}
                onApply={(next) => updateSection('network_namespaces', asObjectArray(next))}
              />
            </div>
          )}
        </Card>
      </>
    );
  };

  const renderEndpoints = () => {
    const items = asObjectArray(sectionValue('endpoints', config));
    return (
      <>
        <SectionHeader
          title="Endpoints"
          description="WireGuard, Tailscale, OpenConnect и OpenVPN endpoints."
        />
        <Card className="singbox-list-card">
          <ListToolbar
            label="Endpoints"
            onAdd={() =>
              updateSection('endpoints', [
                ...items,
                { type: 'wireguard', tag: `endpoint-${items.length + 1}` },
              ])
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {items.map((item, index) => {
              const nextItems = [...items];
              const patch = (value: JsonObject) => {
                nextItems[index] = { ...nextItems[index], ...value };
                updateSection('endpoints', nextItems);
              };
              return (
                <Card size="small" className="singbox-item-card" key={`endpoint-${index}`}>
                  <div className="singbox-item-card-header">
                    <strong>{asString(item.tag) || `Endpoint #${index + 1}`}</strong>
                    <Button
                      danger
                      type="text"
                      icon={<DeleteOutlined />}
                      onClick={() =>
                        updateSection(
                          'endpoints',
                          items.filter((_, i) => i !== index),
                        )
                      }
                    />
                  </div>
                  <Row gutter={[12, 12]}>
                    <Field label="Тип">
                      <Select
                        style={{ width: '100%' }}
                        value={asString(item.type) || 'wireguard'}
                        options={[
                          'wireguard',
                          'tailscale',
                          'openconnect',
                          'openvpn-client',
                          'openvpn-server',
                        ].map((v) => ({ label: v, value: v }))}
                        onChange={(next) => patch({ type: next })}
                      />
                    </Field>
                    <Field label="Tag">
                      <TextField
                        value={asString(item.tag)}
                        onChange={(next) => patch({ tag: next })}
                      />
                    </Field>
                    <Field label="Server">
                      <TextField
                        value={asString(item.server)}
                        onChange={(next) => patch({ server: next })}
                      />
                    </Field>
                    <Field label="Port">
                      <NumberField
                        value={asNumber(item.server_port)}
                        max={65535}
                        onChange={(next) => patch({ server_port: next })}
                      />
                    </Field>
                    <Field label="Domain resolver">
                      <TextField
                        value={asString(item.domain_resolver)}
                        onChange={(next) => patch({ domain_resolver: next })}
                      />
                    </Field>
                    <Field label="Detour">
                      <TextField
                        value={asString(item.detour)}
                        onChange={(next) => patch({ detour: next })}
                      />
                    </Field>
                  </Row>
                  <div className="singbox-item-actions">
                    <JsonModal
                      title={`${asString(item.tag) || 'Endpoint'} — JSON`}
                      value={item}
                      onApply={(next) => patch(asObject(next))}
                    />
                  </div>
                </Card>
              );
            })}
          </Space>
        </Card>
      </>
    );
  };

  const renderOutbounds = () => {
    const items = asObjectArray(sectionValue('outbounds', config));
    const updateItem = (index: number, patch: JsonObject) => {
      const next = [...items];
      next[index] = { ...next[index], ...patch };
      updateSection('outbounds', next);
    };
    return (
      <>
        <SectionHeader
          title="Исходящие"
          description="Основные поля исходящих здесь; TLS, transport и редкие параметры — в расширенном JSON каждой записи."
        />
        <Card className="singbox-list-card">
          <ListToolbar
            label="Outbounds"
            onAdd={() =>
              updateSection('outbounds', [
                ...items,
                { type: 'direct', tag: `outbound-${items.length + 1}` },
              ])
            }
          />
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {items.map((item, index) => {
              const type = asString(item.type) || 'direct';
              return (
                <Card size="small" className="singbox-item-card" key={`outbound-${index}`}>
                  <div className="singbox-item-card-header">
                    <strong>{asString(item.tag) || `Outbound #${index + 1}`}</strong>
                    <Button
                      danger
                      type="text"
                      icon={<DeleteOutlined />}
                      onClick={() =>
                        updateSection(
                          'outbounds',
                          items.filter((_, i) => i !== index),
                        )
                      }
                    />
                  </div>
                  <Row gutter={[12, 12]}>
                    <Field label="Тип">
                      <Select
                        style={{ width: '100%' }}
                        value={type}
                        options={outboundTypes.map((v) => ({ label: v, value: v }))}
                        onChange={(next) => updateItem(index, { type: next })}
                      />
                    </Field>
                    <Field label="Tag">
                      <TextField
                        value={asString(item.tag)}
                        onChange={(next) => updateItem(index, { tag: next })}
                      />
                    </Field>
                    <Field label="Server">
                      <TextField
                        value={asString(item.server)}
                        onChange={(next) => updateItem(index, { server: next })}
                      />
                    </Field>
                    <Field label="Server port">
                      <NumberField
                        value={asNumber(item.server_port)}
                        max={65535}
                        onChange={(next) => updateItem(index, { server_port: next })}
                      />
                    </Field>
                    <Field label="Domain resolver">
                      <TextField
                        value={asString(item.domain_resolver)}
                        onChange={(next) => updateItem(index, { domain_resolver: next })}
                      />
                    </Field>
                    <Field label="Detour">
                      <TextField
                        value={asString(item.detour)}
                        onChange={(next) => updateItem(index, { detour: next })}
                      />
                    </Field>
                    {['http', 'socks'].includes(type) && (
                      <>
                        <Field label="Username">
                          <TextField
                            value={asString(item.username)}
                            onChange={(next) => updateItem(index, { username: next })}
                          />
                        </Field>
                        <Field label="Password">
                          <TextField
                            value={asString(item.password)}
                            onChange={(next) => updateItem(index, { password: next })}
                          />
                        </Field>
                      </>
                    )}
                    {['shadowsocks'].includes(type) && (
                      <>
                        <Field label="Method">
                          <TextField
                            value={asString(item.method)}
                            onChange={(next) => updateItem(index, { method: next })}
                          />
                        </Field>
                        <Field label="Password">
                          <TextField
                            value={asString(item.password)}
                            onChange={(next) => updateItem(index, { password: next })}
                          />
                        </Field>
                      </>
                    )}
                    {['vmess', 'vless'].includes(type) && (
                      <>
                        <Field label="UUID">
                          <TextField
                            value={asString(item.uuid)}
                            onChange={(next) => updateItem(index, { uuid: next })}
                          />
                        </Field>
                        <Field label="Flow">
                          <TextField
                            value={asString(item.flow)}
                            onChange={(next) => updateItem(index, { flow: next })}
                          />
                        </Field>
                      </>
                    )}
                    {['trojan', 'hysteria2', 'tuic'].includes(type) && (
                      <Field label="Password">
                        <TextField
                          value={asString(item.password)}
                          onChange={(next) => updateItem(index, { password: next })}
                        />
                      </Field>
                    )}
                    {type === 'urltest' && (
                      <>
                        <Field label="URL">
                          <TextField
                            value={asString(item.url)}
                            onChange={(next) => updateItem(index, { url: next })}
                          />
                        </Field>
                        <Field label="Interval">
                          <TextField
                            value={asString(item.interval)}
                            onChange={(next) => updateItem(index, { interval: next })}
                            placeholder="3m"
                          />
                        </Field>
                      </>
                    )}
                    {['selector', 'urltest'].includes(type) && (
                      <Field label="Outbounds" span={24}>
                        <StringListField
                          value={asStringArray(item.outbounds)}
                          onChange={(next) => updateItem(index, { outbounds: next })}
                        />
                      </Field>
                    )}
                    <Field label="Network namespace">
                      <TextField
                        value={asString(item.netns)}
                        onChange={(next) => updateItem(index, { netns: next })}
                      />
                    </Field>
                  </Row>
                  <Divider />
                  <Space wrap>
                    <JsonModal
                      title={`${asString(item.tag) || 'Outbound'} — JSON`}
                      value={item}
                      onApply={(next) => updateItem(index, asObject(next))}
                    />
                  </Space>
                </Card>
              );
            })}
          </Space>
        </Card>
      </>
    );
  };

  const renderRoute = () => {
    const value = asObject(sectionValue('route', config));
    const rules = Array.isArray(value.rules) ? value.rules : [];
    const ruleSets = Array.isArray(value.rule_set) ? value.rule_set : [];
    return (
      <>
        <SectionHeader
          title="Маршрутизация"
          description="Часто используемые параметры route вынесены в форму; правила и rule-set остаются в расширенном редакторе."
        />
        <Card>
          <Row gutter={[12, 16]}>
            <Field label="Final">
              <TextField
                value={asString(value.final)}
                onChange={(next) => patchSection('route', { final: next })}
              />
            </Field>
            <Field label="Default domain resolver">
              <TextField
                value={asString(value.default_domain_resolver)}
                onChange={(next) => patchSection('route', { default_domain_resolver: next })}
              />
            </Field>
            <Field label="Default HTTP client">
              <TextField
                value={asString(value.default_http_client)}
                onChange={(next) => patchSection('route', { default_http_client: next })}
              />
            </Field>
            <Field label="Default interface">
              <TextField
                value={asString(value.default_interface)}
                onChange={(next) => patchSection('route', { default_interface: next })}
              />
            </Field>
            <Field label="Default mark">
              <NumberField
                value={asNumber(value.default_mark)}
                onChange={(next) => patchSection('route', { default_mark: next })}
              />
            </Field>
            <Field label="Default fallback delay">
              <TextField
                value={asString(value.default_fallback_delay)}
                onChange={(next) => patchSection('route', { default_fallback_delay: next })}
              />
            </Field>
            <Field label="Auto detect interface">
              <ToggleField
                checked={asBoolean(value.auto_detect_interface)}
                onChange={(next) => patchSection('route', { auto_detect_interface: next })}
              />
            </Field>
            <Field label="Override Android VPN">
              <ToggleField
                checked={asBoolean(value.override_android_vpn)}
                onChange={(next) => patchSection('route', { override_android_vpn: next })}
              />
            </Field>
            <Field label="Find process">
              <ToggleField
                checked={asBoolean(value.find_process)}
                onChange={(next) => patchSection('route', { find_process: next })}
              />
            </Field>
            <Field label="Find neighbor">
              <ToggleField
                checked={asBoolean(value.find_neighbor)}
                onChange={(next) => patchSection('route', { find_neighbor: next })}
              />
            </Field>
            <Field label="DHCP lease files" span={24}>
              <StringListField
                value={asStringArray(value.dhcp_lease_files)}
                onChange={(next) => patchSection('route', { dhcp_lease_files: next })}
              />
            </Field>
          </Row>
        </Card>
        <Row gutter={[12, 12]}>
          <Col xs={24} lg={12}>
            <Card title="Route rules">
              <JsonModal
                title="Route rules"
                value={rules}
                onApply={(next) =>
                  patchSection('route', { rules: Array.isArray(next) ? next : [] })
                }
              />
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card title="Rule sets">
              <JsonModal
                title="Rule sets"
                value={ruleSets}
                onApply={(next) =>
                  patchSection('route', { rule_set: Array.isArray(next) ? next : [] })
                }
              />
            </Card>
          </Col>
        </Row>
      </>
    );
  };

  const renderExperimental = () => {
    const value = asObject(sectionValue('experimental', config));
    const cacheFile = asObject(value.cache_file);
    const clash = asObject(value.clash_api);
    const v2ray = asObject(value.v2ray_api);
    const patchNested = (key: string, patch: JsonObject) =>
      patchSection('experimental', { [key]: { ...asObject(value[key]), ...patch } });
    return (
      <>
        <SectionHeader title="Experimental" description="Кэш-файл и совместимые API." />
        <Row gutter={[12, 12]}>
          <Col xs={24} lg={12}>
            <Card title="Cache file">
              <Row gutter={[12, 12]}>
                <Field label="Включён" span={24}>
                  <ToggleField
                    checked={asBoolean(cacheFile.enabled)}
                    onChange={(next) => patchNested('cache_file', { enabled: next })}
                  />
                </Field>
                <Field label="Path" span={24}>
                  <TextField
                    value={asString(cacheFile.path)}
                    onChange={(next) => patchNested('cache_file', { path: next })}
                  />
                </Field>
                <Field label="Cache ID" span={24}>
                  <TextField
                    value={asString(cacheFile.cache_id)}
                    onChange={(next) => patchNested('cache_file', { cache_id: next })}
                  />
                </Field>
                <Field label="Store fakeip">
                  <ToggleField
                    checked={asBoolean(cacheFile.store_fakeip)}
                    onChange={(next) => patchNested('cache_file', { store_fakeip: next })}
                  />
                </Field>
                <Field label="Store DNS">
                  <ToggleField
                    checked={asBoolean(cacheFile.store_dns)}
                    onChange={(next) => patchNested('cache_file', { store_dns: next })}
                  />
                </Field>
              </Row>
              <div className="singbox-item-actions">
                <JsonModal
                  title="Cache file — JSON"
                  value={cacheFile}
                  onApply={(next) =>
                    patchSection('experimental', { ...value, cache_file: asObject(next) })
                  }
                />
              </div>
            </Card>
          </Col>
          <Col xs={24} lg={12}>
            <Card title="Clash API">
              <Row gutter={[12, 12]}>
                <Field label="External controller" span={24}>
                  <TextField
                    value={asString(clash.external_controller)}
                    onChange={(next) => patchNested('clash_api', { external_controller: next })}
                  />
                </Field>
                <Field label="External UI" span={24}>
                  <TextField
                    value={asString(clash.external_ui)}
                    onChange={(next) => patchNested('clash_api', { external_ui: next })}
                  />
                </Field>
                <Field label="Secret" span={24}>
                  <Input.Password
                    value={asString(clash.secret)}
                    onChange={(event) => patchNested('clash_api', { secret: event.target.value })}
                  />
                </Field>
                <Field label="Allow private network">
                  <ToggleField
                    checked={asBoolean(clash.access_control_allow_private_network)}
                    onChange={(next) =>
                      patchNested('clash_api', { access_control_allow_private_network: next })
                    }
                  />
                </Field>
              </Row>
              <div className="singbox-item-actions">
                <JsonModal
                  title="Clash API — JSON"
                  value={clash}
                  onApply={(next) =>
                    patchSection('experimental', { ...value, clash_api: asObject(next) })
                  }
                />
              </div>
            </Card>
          </Col>
          <Col xs={24}>
            <Card title="V2Ray API">
              <Row gutter={[12, 12]}>
                <Field label="Listen" span={24}>
                  <TextField
                    value={asString(v2ray.listen)}
                    onChange={(next) => patchNested('v2ray_api', { listen: next })}
                  />
                </Field>
              </Row>
              <div className="singbox-item-actions">
                <JsonModal
                  title="V2Ray API — JSON"
                  value={v2ray}
                  onApply={(next) =>
                    patchSection('experimental', { ...value, v2ray_api: asObject(next) })
                  }
                />
              </div>
            </Card>
          </Col>
        </Row>
      </>
    );
  };

  const renderSchema = () => (
    <Card>
      <SectionHeader
        title="Schema"
        description="URL JSON Schema, используемая для подсказок и проверки структуры конфигурации."
        onReset={() => updateSection('schema', sectionFallbacks.schema)}
      />
      <TextField
        value={asString(sectionValue('schema', config))}
        onChange={(next) => updateSection('schema', next)}
        placeholder="https://sing-box.sagernet.org/schema.json"
      />
    </Card>
  );

  const renderBasic = () => (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      {renderLog()}
      {renderNtp()}
      {renderSchema()}
    </Space>
  );

  const renderCertificates = () => (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      {renderCertificate()}
      {renderCertificateProviders()}
    </Space>
  );

  const renderNetwork = () => (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      {renderHttpClients()}
      {renderNamespaces()}
    </Space>
  );

  const renderAdvanced = () => (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Card>
        <SectionHeader
          title="Расширенная конфигурация"
          description="Редкие параметры и полный JSON доступны только здесь, чтобы основной интерфейс не был перегружен."
        />
        <Space wrap>
          <JsonModal
            title="Вся конфигурация sing-box"
            value={config}
            onApply={(next) => setConfig(asObject(next))}
          />
          <Button icon={<ReloadOutlined />} onClick={() => void refresh()} disabled={saving}>
            Обновить из файла
          </Button>
        </Space>
      </Card>
      {renderExperimental()}
    </Space>
  );

  const sectionBody = (() => {
    switch (activeSection) {
      case 'dns':
        return renderDns();
      case 'routing':
        return renderRoute();
      case 'outbound':
        return renderOutbounds();
      case 'endpoints':
        return renderEndpoints();
      case 'certificates':
        return renderCertificates();
      case 'network':
        return renderNetwork();
      case 'advanced':
        return renderAdvanced();
      default:
        return renderBasic();
    }
  })();

  const scrollTarget = () => document.getElementById('content-layout') || window;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {contextHolder}
      <Layout
        className={`singbox-page ${isDark ? 'is-dark ' : ''}${isUltra ? 'is-ultra' : ''}`.trim()}
      >
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <FloatButton.BackTop target={scrollTarget} visibilityHeight={200} />
            <Spin spinning={loading} delay={150} description={t('loading')} size="large">
              {fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" onClick={() => void refresh()}>
                      {t('check')}
                    </Button>
                  }
                />
              ) : (
                <Space direction="vertical" size={12} style={{ width: '100%' }}>
                  <Card hoverable>
                    <Row gutter={[12, 12]} align="middle">
                      <Col xs={24} md={14}>
                        <Space wrap>
                          <Button
                            icon={<SaveOutlined />}
                            type="primary"
                            loading={saving}
                            onClick={save}
                          >
                            {t('pages.singBox.save')}
                          </Button>
                          <Button
                            icon={<ReloadOutlined />}
                            loading={saving}
                            onClick={() => void refresh()}
                          >
                            {t('pages.singBox.refresh')}
                          </Button>
                          <Button danger loading={saving} onClick={() => void reset()}>
                            {t('pages.singBox.reset')}
                          </Button>
                        </Space>
                      </Col>
                      <Col xs={24} md={10}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                          <Space wrap>
                            <Tag color={snapshot?.running ? 'success' : 'default'}>
                              {snapshot?.running
                                ? t('pages.singBox.running')
                                : t('pages.singBox.stopped')}
                            </Tag>
                            {snapshot?.version && <Tag>{snapshot.version}</Tag>}
                            <Tag color="processing">
                              {snapshot?.configSource === 'disk'
                                ? t('pages.singBox.sourceDisk')
                                : t('pages.singBox.sourceGenerated')}
                            </Tag>
                          </Space>
                        </div>
                      </Col>
                    </Row>
                  </Card>

                  <Alert
                    type="info"
                    showIcon
                    title={t('pages.singBox.actualTitle')}
                    description={
                      <Space direction="vertical" size={0}>
                        <span>{snapshot?.configPath}</span>
                        {snapshot?.configModified && <span>{snapshot.configModified}</span>}
                      </Space>
                    }
                  />

                  <Alert
                    type="info"
                    showIcon
                    title={t('pages.singBox.managedTitle')}
                    description={t('pages.singBox.managedDesc')}
                  />

                  <Card hoverable>{sectionBody}</Card>
                </Space>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
