import { useMemo, useState } from 'react';
import {
  Alert, Button, Card, Empty, Input, Popconfirm, Radio, Select, Space,
  Statistic, Switch, Tag, Typography, message,
} from 'antd';
import {
  ArrowRightOutlined, CheckCircleOutlined, DeleteOutlined, PlusOutlined,
  SafetyOutlined, ThunderboltOutlined, WarningOutlined,
} from '@ant-design/icons';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { buildRemarkByTag, formatInboundTag } from './helpers';
import {
  compileQuickRoute, diagnoseRouting, isQuickRule, putQuickRoute,
  removeQuickRoute, setQuickRouteEnabled, setQuickRouteTarget,
  type QuickDomainMode, type QuickMatch, type QuickRouteDraft,
  type RouteRecord, type RoutingEngine,
} from './quick-routing';
import './QuickRoutingPanel.css';

interface Props {
  engine: RoutingEngine;
  rules: RouteRecord[];
  inboundTags: string[];
  outboundTags: string[];
  onChange: (next: RouteRecord[]) => void;
}

function criteriaSummary(rule: RouteRecord, engine: RoutingEngine): string {
  const inbounds = rule[engine === 'xray' ? 'inboundTag' : 'inbound'];
  const toText = (value: unknown): string =>
    Array.isArray(value) ? value.join(', ') : typeof value === 'string' ? value : '';
  const criteria = [
    toText(inbounds) && 'Входящие: ' + toText(inbounds),
    toText(rule.domain) && 'Домены: ' + toText(rule.domain),
    toText(rule.domain_suffix) && 'Домены: ' + toText(rule.domain_suffix),
    toText(rule.ip) && 'IP: ' + toText(rule.ip),
    toText(rule.ip_cidr) && 'IP: ' + toText(rule.ip_cidr),
    toText(rule.port) && 'Порты: ' + toText(rule.port),
    toText(rule.port_range) && 'Порты: ' + toText(rule.port_range),
  ].filter(Boolean);
  return criteria.join(' · ') || 'Любой трафик';
}

function blankDraft(): QuickRouteDraft {
  return { match: 'inbound', inboundTags: [], values: '', domainMode: 'suffix', target: '' };
}

export default function QuickRoutingPanel({ engine, rules, inboundTags, outboundTags, onChange }: Props) {
  const [draft, setDraft] = useState<QuickRouteDraft>(blankDraft);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [messageApi, contextHolder] = message.useMessage();
  const { data: inboundOptions } = useInboundOptions();
  const remarkByTag = useMemo(() => buildRemarkByTag(inboundOptions || []), [inboundOptions]);
  const tags = useMemo(() => [...new Set(outboundTags.filter(Boolean))], [outboundTags]);
  const inTags = useMemo(() => [...new Set(inboundTags.filter(Boolean))], [inboundTags]);
  const managed = useMemo(() => rules.map((rule, index) => ({ rule, index })).filter(({ rule }) => isQuickRule(rule, engine)), [rules, engine]);
  const warnings = useMemo(() => diagnoseRouting(rules, engine, tags), [rules, engine, tags]);

  const targetOptions = [
    ...(engine === 'sing-box' ? [{ value: '@reject', label: 'Блокировать соединение' }] : []),
    ...tags.map((value) => ({
      value,
      label: value === 'direct' ? 'direct · Напрямую' :
        value === 'blocked' ? 'blocked · Заблокировать' : value,
    })),
  ];
  const modes: { value: QuickMatch; label: string; caption: string }[] = [
    { value: 'inbound', label: 'По входящему', caption: 'Направить конкретные входящие подключения через выбранный выход' },
    { value: 'domain', label: 'По домену', caption: 'Сайты и их поддомены — через отдельный выход' },
    { value: 'ip', label: 'По IP / подсети', caption: 'Адреса и диапазоны CIDR' },
    { value: 'port', label: 'По порту', caption: 'Направление по порту назначения' },
  ];
  const activeMode = modes.find((mode) => mode.value === draft.match) ?? modes[0];
  const activeRoutes = rules.filter((rule) => rule.enabled !== false).length;

  function create() {
    setError('');
    setNotice('');
    try {
      const compiled = compileQuickRoute(draft, engine, tags, inTags);
      const result = putQuickRoute(rules, compiled, engine);
      onChange(result.rules);
      const text = result.replaced
        ? 'Маршрут с такими условиями обновлён без дублирования.'
        : 'Маршрут добавлен. Сохраните конфигурацию панели, чтобы применить его.';
      setNotice(text);
      messageApi.success(result.replaced ? 'Маршрут обновлён' : 'Маршрут добавлен');
      if (!result.replaced) setDraft((current) => ({ ...current, values: '' }));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  function markerFor(rule: RouteRecord): string {
    return String(engine === 'xray' ? rule.ruleTag || '' : rule.comment || '');
  }

  return (
    <div className="quick-routing">
      {contextHolder}
      <div className="quick-routing-header">
        <div>
          <Typography.Text type="secondary" className="quick-routing-eyebrow">
            <ThunderboltOutlined aria-hidden="true" /> УМНАЯ МАРШРУТИЗАЦИЯ
          </Typography.Text>
          <Typography.Title level={4} className="quick-routing-title">Направляйте трафик без JSON</Typography.Title>
          <Typography.Paragraph type="secondary" className="quick-routing-subtitle">
            Выберите источник, условие и исходящее подключение. Правила сохраняются в общей конфигурации и не затрагивают сложные настройки.
          </Typography.Paragraph>
        </div>
        <div className="quick-routing-stats">
          <Statistic title="Активных правил" value={activeRoutes} />
          <Statistic title="Быстрых маршрутов" value={managed.length} />
        </div>
      </div>

      <div className="quick-routing-workspace">
        <Card className="quick-routing-composer" title="Новый маршрут" size="small">
          <Typography.Text className="quick-routing-field">1. Когда использовать</Typography.Text>
          <Radio.Group
            className="quick-routing-modes"
            value={draft.match}
            onChange={(event) => {
              setDraft((current) => ({ ...current, match: event.target.value as QuickMatch, values: '' }));
              setError('');
            }}
            optionType="button"
            buttonStyle="solid"
          >
            {modes.map((mode) => <Radio.Button key={mode.value} value={mode.value}>{mode.label}</Radio.Button>)}
          </Radio.Group>
          <Typography.Paragraph type="secondary" className="quick-routing-help">
            {activeMode.caption}
          </Typography.Paragraph>

          <Typography.Text className="quick-routing-field">
            {draft.match === 'inbound' ? '2. Выберите входящие подключения' : '2. Условие маршрута'}
          </Typography.Text>
          {draft.match !== 'inbound' && (
            <div className="quick-routing-input-group">
              <Input.TextArea
                aria-label="Значения условия"
                value={draft.values}
                onChange={(event) => setDraft((current) => ({ ...current, values: event.target.value }))}
                autoSize={{ minRows: 2, maxRows: 4 }}
                placeholder={
                  draft.match === 'domain' ? 'example.com, example.org' :
                    draft.match === 'ip' ? '203.0.113.0/24, 2001:db8::/32' : '443, 8443, 1000-2000'
                }
              />
              {draft.match === 'domain' && (
                <Radio.Group
                  value={draft.domainMode}
                  onChange={(event) => setDraft((current) => ({ ...current, domainMode: event.target.value as QuickDomainMode }))}
                  size="small"
                  options={[
                    { value: 'suffix', label: 'Домен + поддомены' },
                    { value: 'exact', label: 'Точное совпадение' },
                  ]}
                />
              )}
              <Typography.Text type="secondary" className="quick-routing-help">
                Несколько значений через запятую или с новой строки.
              </Typography.Text>
            </div>
          )}
          <Select
            aria-label="Входящие подключения для маршрута"
            mode="multiple"
            showSearch
            allowClear
            className="quick-routing-select"
            placeholder={draft.match === 'inbound' ? 'Выберите одно или несколько входящих' : 'Любое входящее (необязательно)'}
            value={draft.inboundTags}
            onChange={(inboundTags) => setDraft((current) => ({ ...current, inboundTags }))}
            options={inTags.map((value) => ({ value, label: formatInboundTag(value, remarkByTag) }))}
            optionFilterProp="label"
          />

          <Typography.Text className="quick-routing-field">3. Куда направлять</Typography.Text>
          <Select
            showSearch
            allowClear
            className="quick-routing-select"
            aria-label="Исходящее подключение"
            placeholder="Выберите исходящее подключение"
            value={draft.target || undefined}
            onChange={(target) => setDraft((current) => ({ ...current, target: target || '' }))}
            options={targetOptions}
            optionFilterProp="label"
          />
          <div className="quick-routing-flow-preview">
            <Tag bordered={false}>{draft.match === 'inbound'
              ? (draft.inboundTags.length ? draft.inboundTags.join(', ') : 'Входящий')
              : (draft.values.trim() || 'Условие')}</Tag>
            <ArrowRightOutlined aria-hidden="true" />
            <Tag color={draft.target === '@reject' || draft.target === 'blocked' ? 'red' : 'blue'}>
              {draft.target || 'Исходящий'}
            </Tag>
          </div>
          {error && <Alert type="error" showIcon title={error} />}
          {notice && <Alert type="success" showIcon title={notice} />}
          <div className="quick-routing-actions">
            <Button
              type="primary"
              icon={<PlusOutlined aria-hidden="true" />}
              disabled={draft.match === 'inbound' ? draft.inboundTags.length === 0 || !draft.target : !draft.values.trim() || !draft.target}
              onClick={create}
            >
              Создать маршрут
            </Button>
            <Button onClick={() => { setDraft(blankDraft()); setError(''); setNotice(''); }}>Сбросить</Button>
          </div>
          <Typography.Text type="secondary" className="quick-routing-help">
            <SafetyOutlined aria-hidden="true" /> Изменения вступят в силу после общего сохранения настроек.
          </Typography.Text>
        </Card>

        <div className="quick-routing-aside">
          <Card title="Быстрые маршруты" size="small" className="quick-routing-managed">
            {managed.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Пока нет быстрых маршрутов">
                <Typography.Text type="secondary">Создайте первый маршрут слева.</Typography.Text>
              </Empty>
            ) : (
              <div className="quick-routing-managed-list">
                {managed.map(({ rule, index }) => {
                  const marker = markerFor(rule);
                  const destination = engine === 'xray' ? rule.outboundTag : rule.action === 'reject' ? 'Блокировать' : rule.outbound;
                  return (
                    <div className="quick-routing-managed-item" key={marker + ':' + index}>
                      <div className="quick-routing-managed-title">
                        <span>#{index + 1}</span>
                        <Select
                          size="small"
                          showSearch
                          aria-label={'Исходящее маршрута #' + (index + 1)}
                          value={rule.action === 'reject' && engine === 'sing-box' ? '@reject' : String(destination || '')}
                          style={{ minWidth: 105, flex: 1, maxWidth: 230 }}
                          options={targetOptions}
                          optionFilterProp="label"
                          onChange={(target) => onChange(setQuickRouteTarget(rules, marker, target, engine, tags))}
                        />
                        <Space size={5}>
                          <Switch
                            size="small"
                            aria-label={'Включить маршрут #' + (index + 1)}
                            checked={rule.enabled !== false}
                            onChange={(enabled) => onChange(setQuickRouteEnabled(rules, marker, enabled, engine))}
                          />
                          <Popconfirm
                            title="Удалить быстрый маршрут?"
                            description="Остальные правила останутся без изменений."
                            okText="Удалить"
                            cancelText="Отмена"
                            onConfirm={() => onChange(removeQuickRoute(rules, marker, engine))}
                          >
                            <Button size="small" type="text" danger aria-label={'Удалить маршрут #' + (index + 1)} icon={<DeleteOutlined aria-hidden="true" />} />
                          </Popconfirm>
                        </Space>
                      </div>
                      <Typography.Paragraph className="quick-routing-criteria" ellipsis={{ rows: 2, expandable: true, symbol: 'Ещё' }}>
                        {criteriaSummary(rule, engine)}
                      </Typography.Paragraph>
                    </div>
                  );
                })}
              </div>
            )}
          </Card>
          <Card size="small" title="Проверка конфигурации" className="quick-routing-health">
            {warnings.length === 0 ? (
              <div className="quick-routing-healthy">
                <CheckCircleOutlined aria-hidden="true" />
                <span>В выбранных правилах не найдено очевидных конфликтов.</span>
              </div>
            ) : (
              <>
                <Alert
                  type="warning"
                  showIcon
                  title={`Потенциальных проблем: ${warnings.length}`}
                  description="Проверьте порядок и существование исходящих подключений перед сохранением."
                />
                <div className="quick-routing-issue-list">
                  {warnings.slice(0, 8).map((warning, index) => (
                    <div key={warning.code + index}><WarningOutlined aria-hidden="true" /> {warning.detail}</div>
                  ))}
                  {warnings.length > 8 && <Typography.Text type="secondary">И ещё {warnings.length - 8}…</Typography.Text>}
                </div>
              </>
            )}
          </Card>
        </div>
      </div>
    </div>
  );
}
