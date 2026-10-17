import OutboundPingButton from './OutboundPingButton';
import { prepareRouteRule, routePorts, routeRuleSummary } from './route-rule';
import { useCallback, useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  ConfigProvider,
  Collapse,
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
  Table,
  Radio,
  Tag,
  Empty,
  message,
} from 'antd';
import {
  ArrowUpOutlined,
  ArrowDownOutlined,
  CopyOutlined,
  CodeOutlined,
  DeleteOutlined,
  ExportOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';

import AdBlockTab from '@/pages/adblock/AdBlockTab';
import GatewayModeControl from '@/pages/settings/GatewayModeControl';
import { SettingListItem } from '@/components/ui';
import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import './SingBoxPage.css';
import SimpleSettings from './SimpleSettings';
import NativeRoutingBasic from './NativeRoutingBasic';
import { basicRuleTitle } from './native-routing-basic';
import NativeDnsServers from './NativeDnsServers';
import NativeDnsRules from './NativeDnsRules';
import { putConnection, removeConnection } from './config-references';
import { setDefaultDomainResolver } from './simple-settings';
import { isNativeEndpoint } from './native-import';
import SingBoxOutboundModal from './SingBoxOutboundModal';
import OutboundSubscriptions from './OutboundSubscriptions';
import RoutingCatalogTab from '@/pages/xray/routing/RoutingCatalogTab';
import { catalogAssignedRules, nativeCatalogRule, putNativeCatalogRule } from './catalog-rules';
import { outboundProtocolLabel } from './outbound-editor-model';

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
  inboundTags?: string[];
  subscriptionTags?: string[];
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
  hint,
}: {
  label: string;
  children: ReactNode;
  span?: number;
  hint?: string;
}) {
  return (
    <Col xs={24} md={span}>
      <div className="singbox-field">
        <div className="singbox-field-label">{label}</div>
        {hint && <div className="singbox-field-hint">{hint}</div>}
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
  buttonText = 'JSON',
}: {
  title: string;
  value: unknown;
  onApply: (value: unknown) => void;
  buttonText?: string;
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
      const parsed: unknown = JSON.parse(text);
      if (
        Array.isArray(value)
          ? !Array.isArray(parsed)
          : !parsed || typeof parsed !== 'object' || Array.isArray(parsed)
      )
        throw new Error(Array.isArray(value) ? 'Ожидается JSON-массив.' : 'Ожидается JSON-объект.');
      onApply(parsed);
      setOpen(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <>
      <Button icon={<CodeOutlined />} onClick={openEditor}>
        {buttonText}
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

function singBoxHealthIssues(config: ConfigMap, subscriptionTags: string[] = []) {
  const issues: string[] = [];
  const outbounds = asObjectArray(config.outbounds);
  const tags = [...outbounds, ...asObjectArray(config.endpoints)]
    .map((item) => asString(item.tag))
    .filter(Boolean);
  const seen = new Set<string>();

  tags.forEach((tag) => {
    if (seen.has(tag)) issues.push('Дублирующийся outbound tag: ' + tag);
    seen.add(tag);
  });

  const tagSet = new Set([...tags, ...subscriptionTags]);
  outbounds.forEach((item) => {
    const type = asString(item.type);
    const name = asString(item.tag) || 'без tag';
    if (
      ['vless', 'vmess', 'trojan', 'shadowsocks', 'http', 'socks', 'hysteria2', 'tuic'].includes(
        type,
      )
    ) {
      if (!asString(item.server)) issues.push(type + ' "' + name + '": не указан сервер');
      if (
        !asNumber(item.server_port) &&
        !(type === 'hysteria2' && asStringArray(item.server_ports).length)
      )
        issues.push(type + ' "' + name + '": не указан порт');
    }
    if (['vless', 'vmess'].includes(type) && !asString(item.uuid)) {
      issues.push(type + ' "' + name + '": не указан UUID');
    }
    if (['selector', 'urltest', 'balancer'].includes(type)) {
      asStringArray(item.outbounds).forEach((target) => {
        if (!tagSet.has(target))
          issues.push(type + ' "' + name + '": отсутствует outbound "' + target + '"');
      });
    }
  });

  const route = asObject(config.route);
  if (asString(route.final) && !tagSet.has(asString(route.final))) {
    issues.push('route.final ссылается на отсутствующий outbound "' + asString(route.final) + '"');
  }

  const rules = Array.isArray(route.rules) ? route.rules : [];
  rules.forEach((raw, index) => {
    const rule = asObject(raw);
    if (rule.enabled === false) return;
    if (asString(rule.outbound) && !tagSet.has(asString(rule.outbound))) {
      issues.push(
        'Route rule #' +
          (index + 1) +
          ' ссылается на отсутствующий outbound "' +
          asString(rule.outbound) +
          '"',
      );
    }
  });

  return Array.from(new Set(issues));
}

function SingBoxRouteRuleModal({
  open,
  value,
  inboundTags,
  outboundTags,
  ruleSetTags,
  onCancel,
  onSave,
}: {
  open: boolean;
  value: JsonObject | null;
  inboundTags: string[];
  outboundTags: string[];
  ruleSetTags: string[];
  onCancel: () => void;
  onSave: (value: JsonObject) => void;
}) {
  const [draft, setDraft] = useState<JsonObject>({});
  const [error, setError] = useState('');
  const catalogManaged =
    typeof draft.routingListId === 'number' || typeof draft.routingListCategory === 'string';
  useEffect(() => {
    if (!open) return;
    const nextDraft = value ? JSON.parse(JSON.stringify(value)) : {};
    for (const key of [
      'domain',
      'domain_suffix',
      'domain_keyword',
      'domain_regex',
      'ip_cidr',
      'source_ip_cidr',
      'protocol',
      'network',
      'inbound',
      'rule_set',
    ]) {
      if (typeof nextDraft[key] === 'string') nextDraft[key] = [nextDraft[key]];
    }
    const timer = window.setTimeout(() => {
      setDraft(nextDraft);
      setError('');
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open, value]);

  const patch = (key: string, nextValue: unknown) => {
    setDraft((current) => {
      const next = { ...current };
      if (
        nextValue === '' ||
        nextValue === undefined ||
        (Array.isArray(nextValue) && nextValue.length === 0)
      )
        delete next[key];
      else next[key] = nextValue;
      return next;
    });
  };

  return (
    <Modal
      open={open}
      width={920}
      title={value ? 'Изменение правила' : 'Новое правило'}
      okText="Сохранить"
      cancelText="Отмена"
      onCancel={onCancel}
      onOk={() => {
        try {
          onSave(prepareRouteRule(draft));
        } catch (error) {
          setError(error instanceof Error ? error.message : String(error));
        }
      }}
    >
      <Alert
        type="info"
        showIcon
        message="Фильтр → назначение"
        description="Правила редактируются так же, как в Xray: условия собираются полями, JSON оставлен для редких параметров."
        style={{ marginBottom: 12 }}
      />
      {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 12 }} />}
      {catalogManaged && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 12 }}
          title="Домены и IP управляются каталогом списков"
          description="Содержимое списка изменяется во вкладке «Каталог списков». Назначение и другие условия можно изменить здесь."
        />
      )}
      <Card size="small">
        <Row gutter={[12, 14]}>
          <Field label="Исходящее подключение">
            <Select
              allowClear
              style={{ width: '100%' }}
              disabled={!!draft.action && draft.action !== 'route'}
              value={asString(draft.outbound) || undefined}
              options={outboundTags.map((v) => ({ value: v, label: v }))}
              onChange={(v) => patch('outbound', v)}
            />
          </Field>
          <Field label="Действие">
            <Select
              allowClear
              style={{ width: '100%' }}
              value={asString(draft.action) || 'route'}
              options={[
                { value: 'route', label: 'Через подключение' },
                { value: 'reject', label: 'Блокировать' },
                ...['sniff', 'resolve', 'hijack-dns', 'route-options'].map((value) => ({
                  value,
                  label: value,
                })),
              ]}
              onChange={(v) =>
                setDraft((current) => {
                  const next: JsonObject = { ...current, action: v || 'route' };
                  if (next.action !== 'route') delete next.outbound;
                  return next;
                })
              }
            />
          </Field>
          <Field label="Комментарий" span={24}>
            <Input
              value={basicRuleTitle(asString(draft.comment))}
              disabled={asString(draft.comment).startsWith('panel-basic:')}
              onChange={(e) => patch('comment', e.target.value)}
              placeholder="Telegram через proxy"
            />
          </Field>
        </Row>
      </Card>
      <div className="singbox-rule-condition-editor">
        <div className="singbox-section-description">Условия правила</div>
        <Card size="small">
          <Row gutter={[12, 14]}>
            <Field label="Точные домены">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                disabled={catalogManaged}
                style={{ width: '100%' }}
                value={asStringArray(draft.domain)}
                onChange={(v) => patch('domain', v)}
                placeholder="example.com"
              />
            </Field>
            <Field label="Домены и поддомены">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                disabled={catalogManaged}
                style={{ width: '100%' }}
                value={asStringArray(draft.domain_suffix)}
                onChange={(v) => patch('domain_suffix', v)}
              />
            </Field>
            <Field label="Ключевые слова доменов">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                disabled={catalogManaged}
                style={{ width: '100%' }}
                value={asStringArray(draft.domain_keyword)}
                onChange={(v) => patch('domain_keyword', v)}
              />
            </Field>
            <Field label="Регулярные выражения доменов">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                disabled={catalogManaged}
                style={{ width: '100%' }}
                value={asStringArray(draft.domain_regex)}
                onChange={(v) => patch('domain_regex', v)}
              />
            </Field>
            <Field label="IP / CIDR">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                disabled={catalogManaged}
                style={{ width: '100%' }}
                value={asStringArray(draft.ip_cidr)}
                onChange={(v) => patch('ip_cidr', v)}
              />
            </Field>
            <Field label="IP источника / CIDR">
              <Select
                mode="tags"
                tokenSeparators={[',', '\n']}
                style={{ width: '100%' }}
                value={asStringArray(draft.source_ip_cidr)}
                onChange={(v) => patch('source_ip_cidr', v)}
              />
            </Field>
            <Field label="Порты назначения">
              <Input
                value={routePorts(draft)}
                onChange={(e) =>
                  setDraft((current) => {
                    const next: JsonObject = { ...current, port: e.target.value };
                    delete next.port_range;
                    return next;
                  })
                }
                placeholder="80, 443, 1000:2000"
              />
            </Field>
            <Field label="Порты источника">
              <Input
                value={routePorts(draft, true)}
                onChange={(e) =>
                  setDraft((current) => {
                    const next: JsonObject = { ...current, source_port: e.target.value };
                    delete next.source_port_range;
                    return next;
                  })
                }
              />
            </Field>
            <Field label="Протокол">
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                value={asStringArray(draft.protocol)}
                options={['http', 'tls', 'quic', 'dns', 'bittorrent'].map((v) => ({
                  value: v,
                  label: v,
                }))}
                onChange={(v) => patch('protocol', v)}
              />
            </Field>
            <Field label="Сеть">
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                value={asStringArray(draft.network)}
                options={['tcp', 'udp'].map((v) => ({ value: v, label: v }))}
                onChange={(v) => patch('network', v)}
              />
            </Field>
            <Field label="Входящие подключения">
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                value={asStringArray(draft.inbound)}
                options={inboundTags.map((v) => ({ value: v, label: v }))}
                onChange={(v) => patch('inbound', v)}
              />
            </Field>
            <Field label="Наборы правил">
              <Select
                mode="multiple"
                style={{ width: '100%' }}
                value={asStringArray(draft.rule_set)}
                options={ruleSetTags.map((v) => ({ value: v, label: v }))}
                onChange={(v) => patch('rule_set', v)}
              />
            </Field>
          </Row>
        </Card>
        <div className="singbox-inline-actions">
          <JsonModal
            title="Правило — JSON"
            value={draft}
            onApply={(next) => setDraft(asObject(next))}
            buttonText="Редактировать JSON"
          />
        </div>
      </div>
    </Modal>
  );
}

export default function SingBoxPage() {
  const { t } = useTranslation();
  const [messageApi, contextHolder] = message.useMessage();
  const [modalApi, modalContextHolder] = Modal.useModal();
  const { antdThemeConfig, isDark, isUltra } = useTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [config, setConfig] = useState<ConfigMap>({});
  const sectionSlug =
    location.pathname === '/outbound'
      ? 'outbound'
      : location.pathname === '/routing'
        ? 'routing'
        : location.hash.replace(/^#/, '');
  // Legacy deep links remain valid, with no second layer of section tabs.
  const aliases: Record<string, string> = {
    balancer: 'outbound',
    endpoints: 'outbound',
    certificates: 'advanced',
    network: 'advanced',
  };
  const sectionKeys = new Set([
    'basic',
    'dns',
    'routing',
    'outbound',
    'adblock',
    'gateway',
    'advanced',
  ]);
  const resolvedSection = location.search.includes('routingTab=adblock')
    ? 'adblock'
    : aliases[sectionSlug] || sectionSlug;
  const activeSection = sectionKeys.has(resolvedSection) ? resolvedSection : 'basic';
  const showAdvanced = activeSection === 'advanced';
  const utilitySection = activeSection === 'gateway' || activeSection === 'adblock';
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [fetchError, setFetchError] = useState('');
  const SECTION_LABELS: Record<string, ReactNode> = {
    basic: 'Основные',
    dns: 'DNS',
    routing: 'Маршрутизация',
    outbound: 'Подключения',
    adblock: 'AdBlock',
    gateway: 'Режим шлюза',
    advanced: 'Дополнительно',
  };

  const [subscriptionsOpen, setSubscriptionsOpen] = useState(false);
  const [subscriptionURL, setSubscriptionURL] = useState('');
  const [outboundModalOpen, setOutboundModalOpen] = useState(false);
  const [editingOutbound, setEditingOutbound] = useState<number | null>(null);
  const [newOutboundType, setNewOutboundType] = useState('vless');
  const [routeRuleModalOpen, setRouteRuleModalOpen] = useState(false);
  const [editingRouteRule, setEditingRouteRule] = useState<number | null>(null);
  const [routeSearch, setRouteSearch] = useState('');
  const [outboundSearch, setOutboundSearch] = useState('');

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
  }, [setConfig, setSnapshot]);

  useEffect(() => {
    const initial = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(initial);
  }, [refresh]);

  const updateSection = useCallback(
    (key: SectionKey, value: unknown) => {
      setConfig((current) => ({
        ...current,
        [key === 'schema' ? '$schema' : key]: value,
      }));
    },
    [setConfig],
  );

  const patchSection = useCallback(
    (key: SectionKey, patch: JsonObject) => {
      setConfig((current) => {
        const currentValue = asObject(current[key]);
        return {
          ...current,
          [key]: { ...currentValue, ...patch },
        };
      });
    },
    [setConfig],
  );

  const save = async () => {
    try {
      const next: ConfigMap = { ...config };
      if (asObjectArray(next.outbounds).some((item) => item.type === 'masque')) {
        const experimental = asObject(next.experimental);
        next.experimental = {
          ...experimental,
          cache_file: {
            ...asObject(experimental.cache_file),
            enabled: true,
            store_masque_config: true,
          },
        };
      }
      delete next.inbounds;
      delete next.services;

      setSaving(true);
      const msg = (await HttpUtil.post('/panel/api/setting/singbox/config', {
        config: JSON.stringify(next),
      })) as ApiMsg;
      if (!msg?.success) throw new Error(msg?.msg || 'Не удалось сохранить настройки sing-box');
      // The backend has already validated and published the applied settings.
      // A follow-up GET immediately reruns expensive config generation.
      setSnapshot((current) => (current ? { ...current, config: next } : current));
      // Edits made during an in-flight save must remain unsaved instead of
      // being overwritten by the response from an earlier revision.
      setConfig((current) => (JSON.stringify(current) === JSON.stringify(config) ? next : current));
      messageApi.success(t('pages.singBox.saved'));
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
      <Card title="Логирование">
        <SettingListItem title="Вести журнал">
          <Switch
            checked={!asBoolean(value.disabled)}
            onChange={(enabled) => patchSection('log', { disabled: !enabled })}
          />
        </SettingListItem>
        <SettingListItem
          title="Уровень журнала"
          description="Отладку включайте на время поиска ошибки."
        >
          <Select
            aria-label="Подробность журнала"
            style={{ width: '100%' }}
            value={asString(value.level) || 'info'}
            options={['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'].map((level) => ({
              label: level,
              value: level,
            }))}
            onChange={(level) => patchSection('log', { level })}
          />
        </SettingListItem>
        <SettingListItem
          title="Файл журнала"
          description="Оставьте пустым для вывода в системный журнал."
        >
          <TextField
            value={asString(value.output)}
            onChange={(output) => patchSection('log', { output })}
          />
        </SettingListItem>
        <SettingListItem title="Добавлять время к записям">
          <Switch
            checked={asBoolean(value.timestamp)}
            onChange={(timestamp) => patchSection('log', { timestamp })}
          />
        </SettingListItem>
        {showAdvanced && (
          <JsonModal
            title="Лог — расширенный JSON"
            value={value}
            onApply={(next) => updateSection('log', asObject(next))}
          />
        )}
      </Card>
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
          <Space orientation="vertical" style={{ width: '100%' }} size={12}>
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
          <Space orientation="vertical" style={{ width: '100%' }} size={12}>
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
          <Space orientation="vertical" style={{ width: '100%' }} size={12}>
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

  const renderEndpoints = () => renderOutbounds(true);

  const renderOutbounds = (onlyEndpoints = false, onlyGroups = false) => {
    const items = [
      ...asObjectArray(sectionValue('outbounds', config)),
      ...asObjectArray(config.endpoints),
    ];
    const visible = items
      .map((item, index): JsonObject => ({ ...item, __index: index }))
      .filter(
        (item) =>
          (onlyEndpoints
            ? isNativeEndpoint(item)
            : !isNativeEndpoint(item) &&
              ['selector', 'urltest', 'balancer'].includes(asString(item.type)) === onlyGroups) &&
          [item.tag, item.type, item.server]
            .join(' ')
            .toLowerCase()
            .includes(outboundSearch.toLowerCase()),
      );

    return (
      <Card>
        <div className="singbox-section-header">
          <div>
            <div className="singbox-section-title">
              {onlyEndpoints
                ? 'Туннели и endpoints'
                : onlyGroups
                  ? 'Балансировщики'
                  : 'Исходящие подключения'}
            </div>
            <div className="singbox-section-description">
              {onlyEndpoints
                ? 'Туннельные исходящие подключения sing-box.'
                : onlyGroups
                  ? 'Автоматический выбор и группы подключений.'
                  : 'Серверы и прокси. Группы и туннели доступны ниже в раскрывающихся секциях.'}
            </div>
          </div>
          <Space wrap>
            <Button
              onClick={() => {
                setSubscriptionURL('');
                setSubscriptionsOpen(true);
              }}
            >
              Подписки
            </Button>
            <Button
              icon={<PlusOutlined />}
              type="primary"
              aria-label={
                onlyEndpoints
                  ? 'Добавить endpoint'
                  : onlyGroups
                    ? 'Добавить балансировщик'
                    : 'Добавить outbound'
              }
              onClick={() => {
                setEditingOutbound(null);
                setNewOutboundType(
                  onlyEndpoints ? 'masque-client' : onlyGroups ? 'selector' : 'vless',
                );
                setOutboundModalOpen(true);
              }}
            >
              {onlyEndpoints
                ? 'Добавить endpoint'
                : onlyGroups
                  ? 'Добавить балансировщик'
                  : 'Добавить outbound'}
            </Button>
          </Space>
        </div>
        <Input.Search
          placeholder="Поиск по названию, протоколу или серверу"
          aria-label="Поиск исходящих подключений"
          value={outboundSearch}
          onChange={(event) => setOutboundSearch(event.target.value)}
          allowClear
          style={{ maxWidth: 460, marginBottom: 16 }}
        />
        <Table
          size="small"
          pagination={visible.length > 25 ? { pageSize: 25, showSizeChanger: false } : false}
          scroll={{ x: 850 }}
          rowKey={(row) => `${row.tag}:${row.__index}`}
          dataSource={visible}
          locale={{
            emptyText: (
              <Empty description="Нет исходящих. Начните с direct, VLESS, VMess или Trojan." />
            ),
          }}
          columns={[
            {
              title: 'Tag',
              width: 200,
              render: (_: unknown, row: JsonObject) => (
                <Tag color="blue">{asString(row.tag) || 'без tag'}</Tag>
              ),
            },
            {
              title: 'Протокол',
              width: 190,
              render: (_: unknown, row: JsonObject) =>
                asString(row.type) ? outboundProtocolLabel(asString(row.type)) : '—',
            },
            {
              title: 'Endpoint',
              render: (_: unknown, row: JsonObject) => {
                const type = asString(row.type);
                if (['selector', 'urltest', 'balancer'].includes(type))
                  return asStringArray(row.outbounds).length + ' outbound в группе';
                if (type === 'masque') return 'Cloudflare WARP';
                if (row.server)
                  return asString(row.server) + (row.server_port ? ':' + row.server_port : '');
                return 'Локальный';
              },
            },
            {
              title: 'Защита',
              width: 100,
              align: 'center',
              render: (_: unknown, row: JsonObject) =>
                row.type === 'masque' ? (
                  <Tag
                    color="green"
                    title={
                      row.use_http2
                        ? 'MASQUE поверх HTTP/2 и TLS'
                        : 'MASQUE поверх HTTP/3 и QUIC/TLS'
                    }
                  >
                    {row.use_http2 ? 'H2/TLS' : 'H3/TLS'}
                  </Tag>
                ) : asBoolean(asObject(row.tls).enabled) ? (
                  <Tag color="green">TLS</Tag>
                ) : (
                  '—'
                ),
            },
            {
              title: 'Проверка',
              width: 100,
              render: (_: unknown, row: JsonObject) => {
                const name = asString(row.tag);
                const broken = healthIssues.some((issue) => issue.includes('"' + name + '"'));
                return (
                  <Space orientation="vertical">
                    {broken ? <Tag color="error">Проверить</Tag> : <Tag color="success">ОК</Tag>}
                    <OutboundPingButton
                      key={name}
                      tag={name}
                      disabled={
                        dirty ||
                        broken ||
                        ['direct', 'block', 'bridge'].includes(asString(row.type))
                      }
                    />
                  </Space>
                );
              },
            },
            {
              title: '',
              width: 180,
              render: (_: unknown, row: JsonObject) => {
                const index = asNumber(row.__index);
                return (
                  <Space>
                    <Button
                      size="small"
                      onClick={() => {
                        setEditingOutbound(index);
                        setOutboundModalOpen(true);
                      }}
                    >
                      Изменить
                    </Button>
                    <JsonModal
                      title={(asString(items[index].tag) || 'Outbound') + ' — JSON'}
                      value={items[index]}
                      onApply={(next) => {
                        try {
                          setConfig(putConnection(config, index, asObject(next)));
                        } catch (error) {
                          messageApi.error(error instanceof Error ? error.message : String(error));
                        }
                      }}
                    />
                    <Button
                      size="small"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => {
                        try {
                          const next = removeConnection(config, index);
                          modalApi.confirm({
                            title: `Удалить подключение ${items[index].tag}?`,
                            okText: 'Удалить',
                            cancelText: 'Отмена',
                            okButtonProps: { danger: true },
                            onOk: () => setConfig(next),
                          });
                        } catch (error) {
                          messageApi.error(error instanceof Error ? error.message : String(error));
                        }
                      }}
                    />
                  </Space>
                );
              },
            },
          ]}
        />
        <div className="singbox-inline-actions">
          <Button
            icon={<ExportOutlined />}
            onClick={() => {
              const blob = new Blob([JSON.stringify(items, null, 2)], { type: 'application/json' });
              const url = URL.createObjectURL(blob);
              const anchor = document.createElement('a');
              anchor.href = url;
              anchor.download = 'singbox-outbounds.json';
              anchor.click();
              URL.revokeObjectURL(url);
            }}
          >
            Экспорт JSON
          </Button>
          <JsonModal
            title="Все outbounds"
            value={items}
            onApply={(next) => {
              const items = asObjectArray(next);
              setConfig((current) => ({
                ...current,
                outbounds: items.filter((item) => !isNativeEndpoint(item)),
                endpoints: items.filter(isNativeEndpoint),
              }));
            }}
            buttonText="Массовый JSON"
          />
        </div>{' '}
      </Card>
    );
  };

  const renderRoute = () => {
    const value = asObject(sectionValue('route', config));
    const rules = Array.isArray(value.rules) ? value.rules.map(asObject) : [];
    const outTags = [...asObjectArray(config.outbounds), ...asObjectArray(config.endpoints)]
      .map((item) => asString(item.tag))
      .filter(Boolean)
      .concat(snapshot?.subscriptionTags || []);

    return (
      <>
        <Card>
          <div className="singbox-section-header">
            <div>
              <div className="singbox-section-title">Маршрутизация</div>
              <div className="singbox-section-description">
                Правила проверяются сверху вниз. Первое совпавшее правило определяет назначение;
                если совпадений нет, используется выход по умолчанию.
              </div>
            </div>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => {
                setEditingRouteRule(null);
                setRouteRuleModalOpen(true);
              }}
            >
              Добавить правило
            </Button>
          </div>
          {showAdvanced && (
            <Row gutter={[12, 16]}>
              <Field label="Выход по умолчанию">
                <Select
                  allowClear
                  value={asString(value.final) || undefined}
                  options={outTags.map((v) => ({ value: v, label: v }))}
                  style={{ width: '100%' }}
                  onChange={(v) => patchSection('route', { final: v })}
                />
              </Field>
              <Field label="DNS для адресов серверов">
                <Select
                  allowClear
                  style={{ width: '100%' }}
                  value={
                    asString(value.default_domain_resolver) ||
                    asString(asObject(value.default_domain_resolver).server) ||
                    undefined
                  }
                  options={asObjectArray(asObject(config.dns).servers).map((server) => ({
                    value: asString(server.tag),
                    label: asString(server.tag),
                  }))}
                  onChange={(v) => setConfig(setDefaultDomainResolver(config, v || ''))}
                />
              </Field>
              <Field label="Default HTTP client">
                <TextField
                  value={asString(value.default_http_client)}
                  onChange={(v) => patchSection('route', { default_http_client: v })}
                />
              </Field>
              <Field label="Auto detect interface">
                <ToggleField
                  checked={asBoolean(value.auto_detect_interface)}
                  onChange={(v) => patchSection('route', { auto_detect_interface: v })}
                />
              </Field>
              <Field label="Find process">
                <ToggleField
                  checked={asBoolean(value.find_process)}
                  onChange={(v) => patchSection('route', { find_process: v })}
                />
              </Field>
              <Field label="Find neighbor">
                <ToggleField
                  checked={asBoolean(value.find_neighbor)}
                  onChange={(v) => patchSection('route', { find_neighbor: v })}
                />
              </Field>
            </Row>
          )}

          <Divider />
          <Input.Search
            placeholder="Поиск по условиям, назначению или комментарию"
            aria-label="Поиск правил маршрутизации"
            value={routeSearch}
            onChange={(event) => setRouteSearch(event.target.value)}
            allowClear
            style={{ maxWidth: 460, marginBottom: 16 }}
          />
          <Table
            size="small"
            pagination={false}
            scroll={{ x: 950 }}
            rowKey={(row) => String(row.__index)}
            dataSource={rules
              .map((rule, index): JsonObject => ({ ...rule, __index: index }))
              .filter((rule) =>
                [routeRuleSummary(rule), rule.outbound, rule.action, rule.comment]
                  .join(' ')
                  .toLowerCase()
                  .includes(routeSearch.toLowerCase()),
              )}
            locale={{
              emptyText: (
                <Empty description="Правил нет. Используется выбранный выход в интернет." />
              ),
            }}
            columns={[
              {
                title: '#',
                width: 50,
                render: (_: unknown, row: JsonObject) => asNumber(row.__index) + 1,
              },
              {
                title: 'Включено',
                width: 90,
                render: (_: unknown, row: JsonObject) => (
                  <Switch
                    size="small"
                    checked={row.enabled !== false}
                    aria-label={`Правило ${asNumber(row.__index) + 1}`}
                    onChange={(enabled) =>
                      patchSection('route', {
                        rules: rules.map((rule, index) =>
                          index === asNumber(row.__index) ? { ...rule, enabled } : rule,
                        ),
                      })
                    }
                  />
                ),
              },
              {
                title: 'Условия',
                render: (_: unknown, row: JsonObject) => {
                  return routeRuleSummary(row);
                },
              },
              {
                title: 'Назначение',
                width: 160,
                render: (_: unknown, row: JsonObject) =>
                  asString(row.outbound) ? (
                    <Tag color="blue">{asString(row.outbound)}</Tag>
                  ) : (
                    <Tag>{asString(row.action) || 'route'}</Tag>
                  ),
              },
              {
                title: 'Комментарий',
                render: (_: unknown, row: JsonObject) =>
                  basicRuleTitle(asString(row.comment)) || '—',
              },
              {
                title: '',
                width: 250,
                render: (_: unknown, row: JsonObject) => {
                  const index = asNumber(row.__index);
                  return (
                    <Space>
                      <Button
                        size="small"
                        onClick={() => {
                          if (asString(rules[index].comment).startsWith('panel-basic:')) {
                            const next = new URLSearchParams(location.search);
                            next.set('routingTab', 'basic');
                            navigate({
                              pathname: location.pathname,
                              search: next.toString(),
                              hash: location.hash,
                            });
                            return;
                          }
                          setEditingRouteRule(index);
                          setRouteRuleModalOpen(true);
                        }}
                      >
                        Изменить
                      </Button>
                      <Button
                        size="small"
                        title="Выше"
                        icon={<ArrowUpOutlined />}
                        disabled={index === 0}
                        onClick={() => {
                          const next = [...rules];
                          [next[index - 1], next[index]] = [next[index], next[index - 1]];
                          patchSection('route', { rules: next });
                        }}
                      />
                      <Button
                        size="small"
                        title="Ниже"
                        icon={<ArrowDownOutlined />}
                        disabled={index === rules.length - 1}
                        onClick={() => {
                          const next = [...rules];
                          [next[index], next[index + 1]] = [next[index + 1], next[index]];
                          patchSection('route', { rules: next });
                        }}
                      />
                      <Button
                        size="small"
                        title="Дублировать"
                        icon={<CopyOutlined />}
                        onClick={() =>
                          patchSection('route', {
                            rules: [
                              ...rules.slice(0, index + 1),
                              {
                                ...structuredClone(rules[index]),
                                ...(asString(rules[index].comment).startsWith('panel-basic:')
                                  ? {
                                      comment:
                                        'Копия: ' + basicRuleTitle(asString(rules[index].comment)),
                                    }
                                  : {}),
                              },
                              ...rules.slice(index + 1),
                            ],
                          })
                        }
                      />
                      <Button
                        size="small"
                        danger
                        icon={<DeleteOutlined />}
                        onClick={() =>
                          patchSection('route', { rules: rules.filter((_, i) => i !== index) })
                        }
                      />
                    </Space>
                  );
                },
              },
            ]}
          />
        </Card>

        {showAdvanced && (
          <Card>
            <div className="singbox-card-title">Rule-set и расширенные правила</div>
            <div className="singbox-section-description">
              Сложные наборы остаются редактируемыми вручную, но основная маршрутизация больше не
              требует ручной сборки JSON.
            </div>
            <div className="singbox-inline-actions">
              <JsonModal
                title="Route rules"
                value={rules}
                onApply={(next) =>
                  patchSection('route', { rules: Array.isArray(next) ? next : [] })
                }
                buttonText="Все rules"
              />
              <JsonModal
                title="Rule-set"
                value={Array.isArray(value.rule_set) ? value.rule_set : []}
                onApply={(next) =>
                  patchSection('route', { rule_set: Array.isArray(next) ? next : [] })
                }
                buttonText="Rule-set"
              />
              <JsonModal
                title="Route"
                value={value}
                onApply={(next) => updateSection('route', asObject(next))}
                buttonText="Расширенный JSON"
              />
            </div>
          </Card>
        )}
      </>
    );
  };

  const renderExperimental = () => {
    const value = asObject(sectionValue('experimental', config));
    const cacheFile = asObject(value.cache_file);
    const clash = asObject(value.clash_api);
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
                  <Input value="127.0.0.1:10086" readOnly />
                  <p>Панель управляет API статистики и списками пользователей автоматически.</p>
                </Field>
              </Row>
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
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      {renderLog()}
      {renderNtp()}
      {renderSchema()}
    </Space>
  );

  const renderCertificates = () => (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      {renderCertificate()}
      {renderCertificateProviders()}
    </Space>
  );

  const renderNetwork = () => (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      {renderHttpClients()}
      {renderNamespaces()}
    </Space>
  );

  const renderAdvanced = () => (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      <Card className="singbox-item-card">
        <SectionHeader
          title="Дополнительные настройки"
          description="Редкие параметры скрыты по умолчанию, а полный JSON доступен по кнопке."
        />
        <JsonModal
          title="Вся конфигурация sing-box"
          buttonText="Редактировать JSON"
          value={config}
          onApply={(next) => setConfig(asObject(next))}
        />
      </Card>
      <Collapse
        className="singbox-advanced-groups"
        items={[
          { key: 'cert', label: 'Сертификаты и провайдеры', children: renderCertificates() },
          {
            key: 'network',
            label: 'HTTP-клиенты и сетевые пространства',
            children: renderNetwork(),
          },
          {
            key: 'experimental',
            label: 'Кэш-файл и служебные API',
            children: renderExperimental(),
          },
          { key: 'schema', label: 'Схема JSON', children: renderSchema() },
        ]}
      />
    </Space>
  );

  // Compute once per render, not once for every outbound row.
  const overview = [
    { key: 'dns', label: 'DNS', value: asObjectArray(asObject(config.dns).servers).length, unit: 'серверов' },
    { key: 'routing', label: 'Маршрутизация', value: asObjectArray(asObject(config.route).rules).length, unit: 'правил' },
    { key: 'outbound', label: 'Подключения', value: asObjectArray(config.outbounds).length + asObjectArray(config.endpoints).length, unit: 'исходящих' },
    { key: 'gateway', label: 'Режим шлюза', value: null, unit: 'Автоматическая настройка' },
  ];

  const healthIssues = singBoxHealthIssues(config, snapshot?.subscriptionTags || []);
  const dirty = !!snapshot && JSON.stringify(snapshot.config) !== JSON.stringify(config);

  const sectionBody = (() => {
    switch (activeSection) {
      case 'basic':
        return (
          <Space orientation="vertical" size={12} style={{ width: '100%' }}>
            <Card className="singbox-overview-card">
              <SectionHeader title="Обзор настроек"
                description="Частые действия под рукой; подробные параметры открываются только в выбранном разделе." />
              <div className="singbox-overview-grid">
                {overview.map((entry) => (
                  <button key={entry.key} type="button" className="singbox-overview-link"
                    onClick={() => navigate('/singbox#' + entry.key, { preventScrollReset: true })}>
                    <span className="singbox-overview-label">{entry.label}</span>
                    <strong>{entry.value == null ? 'Настроить' : entry.value}</strong>
                    <span className="singbox-overview-unit">{entry.unit}</span>
                  </button>
                ))}
              </div>
            </Card>
            {renderLog()}
            {renderNtp()}
          </Space>
        );
      case 'dns':
        return (
          <Space orientation="vertical" size={12} style={{ width: '100%' }}>
            <SimpleSettings config={config} onChange={setConfig} />
            <NativeDnsServers
              config={config}
              onChange={setConfig}
              subscriptionTags={snapshot?.subscriptionTags}
            />
            <NativeDnsRules config={config} onChange={setConfig} />
          </Space>
        );
      case 'routing':
        return (
          <Space orientation="vertical" size={12} style={{ width: '100%' }}>
            <Card className="singbox-item-card">
              <NativeRoutingBasic
                config={config}
                onChange={setConfig}
                subscriptionTags={snapshot?.subscriptionTags || []}
              />
            </Card>
            {renderRoute()}
            <Collapse
              className="singbox-advanced-groups"
              items={[
                {
                  key: 'catalog',
                  label: 'Каталог готовых списков',
                  children: (
                    <RoutingCatalogTab
                      outboundTags={Array.from(
                        new Set(
                          [...asObjectArray(config.outbounds), ...asObjectArray(config.endpoints)]
                            .map((o) => asString(o.tag))
                            .filter(Boolean)
                            .concat(snapshot?.subscriptionTags || []),
                        ),
                      )}
                      assignedRules={catalogAssignedRules(
                        asObjectArray(asObject(config.route).rules),
                      )}
                      onAssign={(selection, outbound) =>
                        setConfig((current) => {
                          const route = asObject(current.route);
                          return {
                            ...current,
                            route: {
                              ...route,
                              rules: putNativeCatalogRule(
                                asObjectArray(route.rules),
                                nativeCatalogRule(selection, outbound),
                              ),
                            },
                          };
                        })
                      }
                    />
                  ),
                },
              ]}
            />
          </Space>
        );
      case 'outbound':
        return (
          <Space orientation="vertical" size={12} style={{ width: '100%' }}>
            {renderOutbounds()}
            <Collapse
              className="singbox-advanced-groups"
              defaultActiveKey={
                sectionSlug === 'balancer'
                  ? ['groups']
                  : sectionSlug === 'endpoints'
                    ? ['endpoints']
                    : []
              }
              items={[
                {
                  key: 'groups',
                  label: 'Балансировщики и группы выбора',
                  children: renderOutbounds(false, true),
                },
                { key: 'endpoints', label: 'Туннели и endpoints', children: renderEndpoints() },
              ]}
            />
          </Space>
        );
      case 'adblock':
        return <AdBlockTab />;
      case 'gateway':
        return <GatewayModeControl />;
      case 'advanced':
        return renderAdvanced();
      default:
        return renderBasic();
    }
  })();

  const scrollTarget = () => document.getElementById('content-layout') || window;
  const requestRefresh = () => {
    if (!dirty) {
      void refresh();
      return;
    }
    modalApi.confirm({
      title: 'Отменить несохранённые изменения?',
      content: 'Будут загружены последние сохранённые настройки.',
      okText: 'Отменить изменения',
      cancelText: 'Продолжить настройку',
      onOk: refresh,
    });
  };
  const requestReset = () =>
    modalApi.confirm({
      title: 'Вернуть автоматические настройки sing-box?',
      content:
        'Ручные настройки DNS, исходящих подключений и маршрутизации будут сброшены. Входящие подключения и пользователи сохранятся.',
      okText: 'Сбросить настройки',
      cancelText: 'Отмена',
      okButtonProps: { danger: true },
      onOk: reset,
    });
  const visibleSections = ['basic', 'dns', 'routing', 'outbound', 'adblock', 'gateway', 'advanced'];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {contextHolder}
      {modalContextHolder}
      <Layout
        className={`singbox-page ${isDark ? 'is-dark ' : ''}${isUltra ? 'is-ultra' : ''}`.trim()}
      >
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <FloatButton.BackTop target={scrollTarget} visibilityHeight={200} />
            <Spin
              spinning={loading && !utilitySection}
              delay={150}
              description={t('loading')}
              size="large"
            >
              {fetchError && !utilitySection ? (
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
              ) : snapshot || utilitySection ? (
                <Space orientation="vertical" size={12} style={{ width: '100%' }}>
                  {!utilitySection && (
                    <Card className="singbox-toolbar">
                      <div className="singbox-toolbar-heading">
                        <strong>
                          {t('pages.coreSelection.settings')} · {SECTION_LABELS[activeSection]}
                        </strong>
                        <span>
                          Сохранение проверяет настройки один раз и применяет их к работающему ядру.
                        </span>
                      </div>
                      <Row gutter={[12, 12]} align="middle">
                        <Col xs={24} md={14}>
                          <Space wrap>
                            <Button
                              icon={<SaveOutlined />}
                              type="primary"
                              loading={saving}
                              disabled={!dirty || loading}
                              onClick={save}
                            >
                              {t('pages.singBox.save')}
                            </Button>
                            <Button
                              icon={<ReloadOutlined />}
                              loading={saving}
                              onClick={requestRefresh}
                            >
                              {t('pages.singBox.refresh')}
                            </Button>
                            {showAdvanced && (
                              <Button danger disabled={saving} onClick={requestReset}>
                                {t('pages.singBox.reset')}
                              </Button>
                            )}
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
                              {showAdvanced && (
                                <Tag>
                                  {snapshot?.configSource === 'disk'
                                    ? t('pages.singBox.sourceDisk')
                                    : t('pages.singBox.sourceGenerated')}
                                </Tag>
                              )}
                              {dirty && <Tag color="warning">Есть несохранённые изменения</Tag>}
                            </Space>
                          </div>
                        </Col>
                      </Row>
                    </Card>
                  )}

                  {!utilitySection && healthIssues.length > 0 && (
                    <Alert
                      type="warning"
                      showIcon
                      message={'Проверка конфигурации: ' + healthIssues.length + ' предупреждений'}
                      description={
                        <ul className="singbox-issue-list">
                          {healthIssues.slice(0, 8).map((issue) => (
                            <li key={issue}>{issue}</li>
                          ))}
                          {healthIssues.length > 8 && <li>И ещё {healthIssues.length - 8}...</li>}
                        </ul>
                      }
                    />
                  )}

                  {showAdvanced && (
                    <Alert
                      type="info"
                      showIcon
                      title={t('pages.singBox.actualTitle')}
                      description={
                        <Space orientation="vertical" size={0}>
                          <span>{snapshot?.configPath}</span>
                          {snapshot?.configModified && <span>{snapshot.configModified}</span>}
                        </Space>
                      }
                    />
                  )}

                  {showAdvanced && (
                    <Alert
                      type="info"
                      showIcon
                      title={t('pages.singBox.managedTitle')}
                      description={t('pages.singBox.managedDesc')}
                    />
                  )}

                  <Card className="singbox-section-navigation" size="small">
                    <Radio.Group
                      value={activeSection}
                      buttonStyle="solid"
                      onChange={(event) =>
                        navigate('/singbox#' + event.target.value, { preventScrollReset: true })
                      }
                      options={visibleSections.map((key) => ({
                        value: key,
                        label: SECTION_LABELS[key],
                      }))}
                      optionType="button"
                    />
                  </Card>
                  <div className="singbox-settings-body">{sectionBody}</div>
                </Space>
              ) : (
                <div style={{ minHeight: 200 }} />
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>

      {subscriptionsOpen && (
        <OutboundSubscriptions
          open
          initialURL={subscriptionURL}
          onClose={() => setSubscriptionsOpen(false)}
          onTagsChanged={(tags) =>
            setSnapshot((current) => (current ? { ...current, subscriptionTags: tags } : current))
          }
        />
      )}
      {outboundModalOpen && (
        <SingBoxOutboundModal
          open
          initialType={newOutboundType}
          value={
            editingOutbound == null
              ? null
              : [
                  ...asObjectArray(sectionValue('outbounds', config)),
                  ...asObjectArray(config.endpoints),
                ][editingOutbound]
          }
          existingTags={[
            ...asObjectArray(sectionValue('outbounds', config)),
            ...asObjectArray(config.endpoints),
          ]
            .map((item) => asString(item.tag))
            .filter(Boolean)
            .concat(snapshot?.subscriptionTags || [])}
          onSubscriptionURL={(url) => {
            setOutboundModalOpen(false);
            setSubscriptionURL(url);
            setSubscriptionsOpen(true);
          }}
          onCancel={() => setOutboundModalOpen(false)}
          onSave={(next, additional = []) => {
            try {
              setConfig(putConnection(config, editingOutbound, next, additional));
              setOutboundModalOpen(false);
            } catch (error) {
              messageApi.error(error instanceof Error ? error.message : String(error));
            }
          }}
        />
      )}

      <SingBoxRouteRuleModal
        open={routeRuleModalOpen}
        value={
          editingRouteRule == null
            ? null
            : asObjectArray(asObject(sectionValue('route', config)).rules)[editingRouteRule]
        }
        inboundTags={
          snapshot?.inboundTags ??
          asObjectArray(config.inbounds)
            .map((item) => asString(item.tag))
            .filter(Boolean)
        }
        outboundTags={[
          ...asObjectArray(sectionValue('outbounds', config)),
          ...asObjectArray(config.endpoints),
        ]
          .map((item) => asString(item.tag))
          .filter(Boolean)
          .concat(snapshot?.subscriptionTags || [])}
        ruleSetTags={(() => {
          const ruleSet = asObject(sectionValue('route', config)).rule_set;
          return Array.isArray(ruleSet)
            ? ruleSet
                .map(asObject)
                .map((item) => asString(item.tag))
                .filter(Boolean)
            : [];
        })()}
        onCancel={() => setRouteRuleModalOpen(false)}
        onSave={(next) => {
          const routeValue = asObject(sectionValue('route', config));
          const rules = Array.isArray(routeValue.rules) ? routeValue.rules.map(asObject) : [];
          if (editingRouteRule == null) rules.push(next);
          else rules[editingRouteRule] = next;
          updateSection('route', { ...routeValue, rules });
          setRouteRuleModalOpen(false);
        }}
      />
    </ConfigProvider>
  );
}
