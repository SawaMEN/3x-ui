import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router';
import {
  Alert, Button, Card, Checkbox, Col, ConfigProvider, Divider, Input, Layout, Modal, Row,
  Select, Space, Statistic, Table, Tabs, Tag, Timeline, Typography, message,
} from 'antd';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { HttpUtil } from '@/utils';
import {
  DIAGNOSTIC_CHECKLIST, reconcileProbeProblem, stages, verifiedForMerge,
  type CascadeCheck, type CascadeProbe, type CascadeProblem,
  type CascadeTarget, type ProblemKind, type ProblemStatus, type RouteResult,
} from './cascade-diagnostics';

const STORAGE_KEY = '3x-ui:cascade-diagnostics:v1';
type AccessLogEntry = {
  DateTime?: string | number;
  Inbound?: string;
  Outbound?: string;
  Event?: number;
};
type StoredState = {
  checks: Record<string, CascadeCheck>;
  problems: CascadeProblem[];
  logs: string[];
  checklist: boolean[];
  liveEvidence: string;
  ciApproved: boolean;
};
const EMPTY_STATE: StoredState = {
  checks: {}, problems: [], logs: [], checklist: [], liveEvidence: '', ciApproved: false,
};
const problemStatuses: { label: string; value: ProblemStatus }[] = [
  { label: 'Открыта', value: 'open' },
  { label: 'Исправлена (не проверена)', value: 'fixed' },
  { label: 'Проверена', value: 'verified' },
  { label: 'На повторную проверку', value: 'retest' },
];
const kinds: { label: string; value: ProblemKind }[] = [
  { label: 'Связность', value: 'connectivity' },
  { label: 'Авторизация', value: 'authentication' },
  { label: 'Маршрутизация', value: 'routing' },
  { label: 'Конфигурация', value: 'configuration' },
  { label: 'Другое', value: 'other' },
];
function readState(): StoredState {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) || 'null') as Partial<StoredState> | null;
    if (!parsed || !Array.isArray(parsed.problems) || !parsed.checks || typeof parsed.checks !== 'object') return EMPTY_STATE;
    return {
      checks: parsed.checks, problems: parsed.problems, logs: Array.isArray(parsed.logs) ? parsed.logs : [],
      checklist: Array.isArray(parsed.checklist) ? parsed.checklist : [],
      liveEvidence: typeof parsed.liveEvidence === 'string' ? parsed.liveEvidence : '',
      ciApproved: parsed.ciApproved === true,
    };
  } catch { return EMPTY_STATE; }
}
function labelStatus(status: string) {
  return problemStatuses.find(s => s.value === status)?.label || status;
}
function evidenceTag(status: string) {
  const color = status === 'ok' || status === 'verified' ? 'success'
    : status === 'failed' || status === 'open' ? 'error'
      : status === 'warning' || status === 'retest' ? 'warning' : 'default';
  const label = ({ ok: 'Подтверждено', failed: 'Ошибка', warning: 'Недостаточно данных',
    unknown: 'Не проверено', open: 'Открыта', fixed: 'Исправлена', verified: 'Проверена',
    retest: 'Повторная проверка' } as Record<string, string>)[status] || status;
  return <Tag color={color}>{label}</Tag>;
}

export default function CascadeDiagnosticsPage() {
  const { antdThemeConfig } = useTheme();
  const [messageApi, contextHolder] = message.useMessage();
  const { nodes } = useNodesQuery();
  const { data: inbounds = [] } = useInboundOptions();
  const [core, setCore] = useState<'xray' | 'sing-box'>('sing-box');
  const [targets, setTargets] = useState<CascadeTarget[]>([]);
  const [fetchError, setFetchError] = useState('');
  const [loading, setLoading] = useState(true);
  const [probing, setProbing] = useState<string | null>(null);
  const [store, setStore] = useState<StoredState>(readState);
  const [selectedTag, setSelectedTag] = useState('');
  const [nodeFilter, setNodeFilter] = useState<number | 'all'>('all');
  const [protocolFilter, setProtocolFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [sourceInbound, setSourceInbound] = useState('');
  const [destination, setDestination] = useState('example.com');
  const [title, setTitle] = useState('');
  const [kind, setKind] = useState<ProblemKind>('routing');
  const [manualEvidence, setManualEvidence] = useState('');
  const [coreLogs, setCoreLogs] = useState<AccessLogEntry[]>([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('overview');

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify(store)); } catch { /* private mode */ }
  }, [store]);
  useEffect(() => {
    let cancelled = false;
    void HttpUtil.get<CascadeTarget[]>('/panel/api/xray/node-route/targets?core=' + encodeURIComponent(core), undefined, { silent: true })
      .then(result => {
        if (cancelled) return;
        if (!result?.success || !Array.isArray(result.obj)) throw new Error(result?.msg || 'Ответ API не содержит списка маршрутов');
        setTargets(result.obj);
        setSelectedTag(old => result.obj?.some(t => t.tag === old) ? old : (result.obj?.[0]?.tag || ''));
      })
      .catch(error => { if (!cancelled) { setTargets([]); setFetchError(String(error)); } })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [core]);

  const visible = useMemo(() => targets.filter(t =>
    (nodeFilter === 'all' || t.nodeId === nodeFilter) &&
    (protocolFilter === 'all' || t.protocol === protocolFilter) &&
    (statusFilter === 'all' || (statusFilter === 'ok' ? t.available : statusFilter === 'failed' ? !t.available : true)) &&
    (kindFilter === 'all' || store.problems.some(p => p.tag === t.tag && p.type === kindFilter))
  ), [targets, nodeFilter, protocolFilter, kindFilter, statusFilter, store.problems]);
  const current = targets.find(t => t.tag === selectedTag) || visible[0];
  const sourceNodeId = inbounds.find(inbound => inbound.tag === sourceInbound)?.nodeId;
  const filteredProblems = store.problems.filter(p =>
    (nodeFilter === 'all' || p.nodeId === nodeFilter) &&
    (protocolFilter === 'all' || p.protocol === protocolFilter) &&
    (kindFilter === 'all' || p.type === kindFilter) &&
    (statusFilter === 'all' || p.status === statusFilter) &&
    (!selectedTag || p.tag === selectedTag)
  );
  const currentStages = current ? stages(current, store.checks[current.tag]) : [];
  const checkedCount = store.checklist.filter(Boolean).length;
  const mergeReady = verifiedForMerge(targets, store.checks, store.problems, store.liveEvidence, store.ciApproved) &&
    checkedCount === DIAGNOSTIC_CHECKLIST.length;
  const locallyVerified = targets.filter(t => stages(t, store.checks[t.tag]).slice(0, 4).every(s => s.status === 'ok')).length;

  function log(message: string) {
    const entry = new Date().toLocaleString('ru-RU') + ' · ' + message.slice(0, 750);
    setStore(s => ({ ...s, logs: [entry, ...s.logs].slice(0, 200) }));
  }
  async function refresh() {
    setLoading(true);
    try {
      const result = await HttpUtil.get<CascadeTarget[]>('/panel/api/xray/node-route/targets?core=' + encodeURIComponent(core), undefined, { silent: true });
      if (!result?.success || !Array.isArray(result.obj)) throw new Error(result?.msg || 'Ошибка API');
      setTargets(result.obj);
      setFetchError('');
      log('Перечень доступных входящих нод обновлён');
    } catch (error) {
      setFetchError(String(error)); log('Ошибка обновления: ' + String(error));
    } finally { setLoading(false); }
  }
  async function runCheck(target: CascadeTarget) {
    const check: CascadeCheck = { at: new Date().toISOString(), tag: target.tag, sourceInbound };
    setProbing(target.tag);
    try {
      const probe = await HttpUtil.post<CascadeProbe>('/panel/api/xray/node-route/probe',
        { inboundId: target.inboundId }, { silent: true });
      if (probe?.success && probe.obj) check.probe = probe.obj;
      else check.probeError = probe?.msg || 'Не удалось проверить исходящий туннель';
    } catch (error) { check.probeError = String(error); }
    if (core === 'xray' && sourceInbound && sourceNodeId == null && destination.trim()) {
      const value = destination.trim();
      const ip = /^(\d{1,3}\.){3}\d{1,3}$/.test(value) || value.includes(':');
      try {
        const result = await HttpUtil.post<RouteResult>('/panel/api/xray/routeTest',
          { domain: ip ? '' : value, ip: ip ? value : '', port: 443,
            network: 'tcp', inboundTag: sourceInbound, protocol: 'tls' }, { silent: true });
        if (result?.success && result.obj) check.route = result.obj;
        else check.routeError = result?.msg || 'Не удалось проверить правило маршрутизации';
      } catch (error) { check.routeError = String(error); }
    } else if (sourceNodeId != null) {
      check.routeError = 'Источник расположен на другой ноде; локальный routeTest не проверяет её маршрутизатор. Необходим тест на ноде-источнике.';
    } else if (core === 'sing-box') {
      check.routeError = 'API проверки совпадения правила доступен только для Xray; требуется отдельная проверка маршрутизации sing-box';
    } else {
      check.routeError = 'Выберите локальное входящее и домен для проверки маршрута';
    }
    setStore(s => ({
      ...s, checks: { ...s.checks, [target.tag]: check },
      problems: reconcileProbeProblem(s.problems, target, check),
    }));
    log(target.tag + ': ' + (check.probe?.detail || check.probeError || 'Нет результата') +
      (check.route ? '; выбран: ' + (check.route.outboundTag || 'по умолчанию') : '; ' + (check.routeError || '')));
    setProbing(null);
    return check;
  }
  async function runAll() {
    if (!visible.length) { messageApi.warning('Нет целей для проверки'); return; }
    for (const target of visible) await runCheck(target);
    messageApi.info('Проверки завершены. Сквозной тест с реальным клиентом выполняется отдельно.');
  }
  function ensure(target: CascadeTarget) {
    Modal.confirm({
      title: 'Применить служебное подключение к удалённой ноде?',
      content: 'Сервис создаст или обновит служебного клиента и перезапустит ядро удалённой ноды. Активные соединения могут прерваться. Резервная копия и окно обслуживания рекомендуются.',
      okText: 'Применить и перезапустить',
      cancelText: 'Отмена',
      onOk: async () => {
        const res = await HttpUtil.post<{ tag: string }>('/panel/api/xray/node-route/ensure',
          { inboundId: target.inboundId, core }, { silent: true });
        if (!res?.success || res.obj?.tag !== target.tag) {
          const reason = res?.msg || 'Ошибка применения служебного подключения';
          log('Исправление ' + target.tag + ': ' + reason);
          throw new Error(reason);
        }
        log('Служебное подключение ' + target.tag + ' применено; нужна повторная проверка.');
        setStore(s => ({ ...s, problems: s.problems.map(p =>
          p.tag === target.tag && p.status !== 'verified' ? { ...p, status: 'retest', updatedAt: new Date().toISOString() } : p) }));
        messageApi.success('Подключение применено. Запустите повторную проверку.');
        await refresh();
      },
    });
  }
  function addProblem() {
    if (!current || !title.trim()) { messageApi.warning('Выберите маршрут и введите описание'); return; }
    const problem: CascadeProblem = {
      id: 'manual:' + Date.now(), tag: current.tag, nodeId: current.nodeId,
      protocol: current.protocol, type: kind, title: title.trim(), status: 'open',
      evidence: manualEvidence.slice(0, 1000), updatedAt: new Date().toISOString(),
    };
    setStore(s => ({ ...s, problems: [problem, ...s.problems] }));
    log('Добавлена проблема: ' + problem.title);
    setTitle(''); setManualEvidence('');
  }
  function setProblemStatus(id: string, status: ProblemStatus) {
    setStore(s => ({ ...s, problems: s.problems.map(p => p.id === id
      ? { ...p, status, updatedAt: new Date().toISOString() } : p) }));
    log('Статус проблемы ' + id + ' изменён: ' + labelStatus(status));
  }
  async function loadCoreLogs() {
    setLogsLoading(true);
    try {
      const result = await HttpUtil.post<AccessLogEntry[]>('/panel/api/server/xraylogs/100',
        { filter: current?.tag || '', showDirect: true, showBlocked: true, showProxy: true },
        { silent: true });
      if (!result?.success || !Array.isArray(result.obj)) throw new Error(result?.msg || 'Логи ядра недоступны');
      setCoreLogs(result.obj.filter(entry => entry && typeof entry === 'object'));
    } catch (error) { messageApi.error('Ошибка чтения логов: ' + String(error)); }
    finally { setLogsLoading(false); }
  }
  function exportReport() {
    const data = { generatedAt: new Date().toISOString(), core,
      targets, ...store, warning: 'Записи браузера не заменяют сквозное тестирование реального клиента' };
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const link = document.createElement('a');
    const url = URL.createObjectURL(blob);
    link.href = url; link.download = 'cascade-diagnostics-' + new Date().toISOString().slice(0, 10) + '.json';
    link.click(); URL.revokeObjectURL(url);
  }

  const columns = [
    { title: 'Нода', key: 'node', render: (_: unknown, target: CascadeTarget) => target.nodeName },
    { title: 'Входящее', key: 'inbound', render: (_: unknown, target: CascadeTarget) =>
      <><strong>{target.remark || target.tag}</strong><div><Typography.Text type="secondary">{target.tag} · :{target.portLabel || target.port}</Typography.Text></div></> },
    { title: 'Протокол', dataIndex: 'protocol' as const },
    { title: 'Состояние', key: 'status', render: (_: unknown, target: CascadeTarget) =>
      evidenceTag(!target.available ? 'failed' : stages(target, store.checks[target.tag]).slice(0, 4).every(s => s.status === 'ok') ? 'ok' : 'unknown') },
    { title: 'Действия', key: 'actions', render: (_: unknown, target: CascadeTarget) =>
      <Space wrap><Button size="small" loading={probing === target.tag} onClick={() => { setSelectedTag(target.tag); void runCheck(target); }}>Проверить</Button>
      <Button size="small" disabled={!target.available} onClick={() => ensure(target)}>Подготовить</Button></Space> },
  ];
  return (
    <ConfigProvider theme={antdThemeConfig}>
    <div className="cascade-diagnostics-page" style={{ minHeight: '100vh' }}>
      {contextHolder}
      <Layout style={{ minHeight: '100vh' }}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
              <Row justify="space-between" align="middle" gutter={[12, 12]}>
                <Col><Typography.Title level={3} style={{ margin: 0 }}>Диагностика каскадов</Typography.Title>
                  <Typography.Text type="secondary">Входящее → маршрутизация → исходящее → удалённая нода</Typography.Text></Col>
                <Col><Space wrap>
                  <Button onClick={() => void refresh()} loading={loading}>Обновить список</Button>
                  <Button type="primary" onClick={() => void runAll()} loading={!!probing} disabled={!visible.length}>Проверить все</Button>
                  <Button onClick={exportReport}>Экспорт отчёта JSON</Button>
                </Space></Col>
              </Row>
              <Alert type="warning" showIcon title="Каскад пока не подтверждён"
                description="По сообщению оператора входящий → другая нода сейчас не работает. Доступность ноды или TCP-порта не считается исправлением. Даже успешный HTTP-тест служебного исходящего не доказывает маршрут реального клиента. Установите статус «Проверена» только после проверки и сохраните доказательства." />
              {fetchError && <Alert type="error" showIcon title="Ошибка загрузки целей" description={fetchError} />}
              <Row gutter={[12, 12]}>
                <Col xs={12} md={6}><Card size="small"><Statistic title="Удалённых входящих" value={targets.length} /></Card></Col>
                <Col xs={12} md={6}><Card size="small"><Statistic title="Исходящие подтверждены*" value={locallyVerified} suffix={'/ ' + targets.length} /></Card></Col>
                <Col xs={12} md={6}><Card size="small"><Statistic title="Нерешённых проблем" value={store.problems.filter(p => p.status !== 'verified').length} /></Card></Col>
                <Col xs={12} md={6}><Card size="small"><Statistic title="Пункты чек-листа" value={checkedCount} suffix={'/ ' + DIAGNOSTIC_CHECKLIST.length} /></Card></Col>
              </Row>
              <Typography.Text type="secondary">* Подтверждены только автоматизированные этапы. Проверка сквозного потока с пользовательского входящего выполняется отдельно.</Typography.Text>
              <Card size="small" title="Фильтры и маршрут">
                <Row gutter={[10, 10]}>
                  <Col xs={24} sm={12} lg={4}><Typography.Text>Ядро</Typography.Text><Select style={{ width: '100%' }} value={core} onChange={value => { setCore(value); setSelectedTag(''); setLoading(true); setFetchError(''); }}
                    options={[{ label: 'sing-box', value: 'sing-box' }, { label: 'Xray', value: 'xray' }]} /></Col>
                  <Col xs={24} sm={12} lg={5}><Typography.Text>Нода</Typography.Text><Select style={{ width: '100%' }} value={nodeFilter} onChange={setNodeFilter}
                    options={[{ label: 'Все ноды', value: 'all' }, ...nodes.map(n => ({ label: n.name || String(n.id), value: n.id }))]} /></Col>
                  <Col xs={24} sm={12} lg={4}><Typography.Text>Протокол</Typography.Text><Select style={{ width: '100%' }} value={protocolFilter} onChange={setProtocolFilter}
                    options={[{ label: 'Все протоколы', value: 'all' }, ...Array.from(new Set(targets.map(t => t.protocol))).map(p => ({ label: p, value: p }))]} /></Col>
                  <Col xs={24} sm={12} lg={5}><Typography.Text>Тип ошибки</Typography.Text><Select style={{ width: '100%' }} value={kindFilter} onChange={setKindFilter}
                    options={[{ label: 'Все типы', value: 'all' }, ...kinds]} /></Col>
                  <Col xs={24} sm={12} lg={6}><Typography.Text>Статус</Typography.Text><Select style={{ width: '100%' }} value={statusFilter} onChange={setStatusFilter}
                    options={[{ label: 'Любой', value: 'all' }, { label: 'Доступно', value: 'ok' }, { label: 'Недоступно', value: 'failed' },
                      ...problemStatuses]} /></Col>
                  <Col span={24}><Typography.Text>Целевой каскад</Typography.Text>
                    <Select style={{ width: '100%' }} showSearch={{ optionFilterProp: 'label' }} value={current?.tag} onChange={setSelectedTag}
                      options={visible.map(t => ({ value: t.tag, label: t.nodeName + ' · ' + (t.remark || t.protocol) + ' · ' + t.tag }))} placeholder="Выберите маршрут" /></Col>
                </Row>
              </Card>
              <Tabs activeKey={activeTab} onChange={setActiveTab} items={[
                { key: 'overview', label: 'Схема и этапы', children:
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Card size="small" title="Схема каскада">
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, alignItems: 'center' }}>
                        {['Клиент', sourceInbound || 'Входящее (выберите ниже)', 'Правило маршрутизации',
                          current?.tag || 'node-route-*', current?.nodeName || 'Удалённая нода',
                          'Интернет'].map((item, index) => (
                          <span key={index} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                            {index > 0 && <span aria-hidden>→</span>}
                            <Tag style={{ padding: '8px 10px', fontSize: 13, whiteSpace: 'normal' }}>{item}</Tag>
                          </span>))}
                      </div>
                    </Card>
                    <Card size="small" title="Параметры проверки">
                      <Row gutter={[12, 12]}>
                        <Col xs={24} md={12}><Typography.Text>Локальное входящее (источник)</Typography.Text>
                          <Select style={{ width: '100%' }} showSearch={{ optionFilterProp: 'label' }} value={sourceInbound || undefined}
                            onChange={setSourceInbound} placeholder="Укажите входящее панели" allowClear
                            options={inbounds.map(x =>
                              ({ label: (x.nodeId == null ? 'Панель' : (nodes.find(n => n.id === x.nodeId)?.name || ('Нода #' + x.nodeId))) + ' · ' + (x.remark || x.tag) + ' · ' + x.protocol, value: x.tag })).filter(x => !!x.value)} /></Col>
                        <Col xs={24} md={8}><Typography.Text>Тестовый домен/IP</Typography.Text>
                          <Input value={destination} onChange={e => setDestination(e.target.value)} placeholder="example.com" /></Col>
                        <Col xs={24} md={4} style={{ display: 'flex', alignItems: 'end' }}>
                          <Button type="primary" block disabled={!current} loading={probing === current?.tag}
                            onClick={() => { if (current) void runCheck(current); }}>Проверить каскад</Button></Col>
                      </Row>
                    </Card>
                    <Card size="small" title="Этапы прохождения трафика">
                      {!current ? <Typography.Text type="secondary">Нет подходящих целей.</Typography.Text> :
                        <Timeline items={currentStages.map(s => ({
                          color: s.status === 'ok' ? 'green' : s.status === 'failed' ? 'red' : 'gray',
                          children: <div><Space wrap><strong>{s.name}</strong>{evidenceTag(s.status)}</Space>
                            <div><Typography.Text type="secondary">Ожидание: {s.expected}</Typography.Text></div>
                            <div style={{ overflowWrap: 'anywhere' }}>Факт: {s.actual}</div></div>,
                        }))} />}
                    </Card>
                    <Card size="small" title="Ожидаемое и фактическое поведение">
                      <Table size="small" rowKey="id" pagination={false} scroll={{ x: 640 }}
                        dataSource={currentStages} columns={[
                          { title: 'Этап', dataIndex: 'name' }, { title: 'Ожидается', dataIndex: 'expected' },
                          { title: 'Фактически', dataIndex: 'actual' },
                          { title: 'Статус', dataIndex: 'status', render: (value: string) => evidenceTag(value) },
                        ]} />
                    </Card>
                  </Space> },
                { key: 'connections', label: 'Входящие / исходящие', children:
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Card size="small" title="Входящие панели и нод-источников">
                      <Table size="small" rowKey="id" pagination={{ pageSize: 8 }} dataSource={inbounds}
                        columns={[{ title: 'Источник', key: 'source', render: (_: unknown, entry: typeof inbounds[number]) => entry.nodeId == null ? 'Панель' : (nodes.find(n => n.id === entry.nodeId)?.name || 'Нода #' + entry.nodeId) },
                          { title: 'Название', dataIndex: 'remark' }, { title: 'Тег', dataIndex: 'tag' },
                          { title: 'Протокол', dataIndex: 'protocol' }, { title: 'Порт', dataIndex: 'port' }]} /></Card>
                    <Card size="small" title="Удалённые входящие и управляемые исходящие">
                      <Table size="small" rowKey="tag" loading={loading} dataSource={visible} scroll={{ x: 720 }}
                        columns={columns} pagination={{ pageSize: 10 }} />
                      <Typography.Paragraph type="secondary">Тег node-route-* создаётся при генерации рабочей конфигурации, а не хранится как обычное исходящее в редакторе. Отображаем только публичные метаданные без учётных данных.</Typography.Paragraph>
                    </Card>
                  </Space> },
                { key: 'problems', label: 'Ошибки и предупреждения', children:
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Card size="small" title="Добавить проблему вручную">
                      <Row gutter={[8, 8]}>
                        <Col xs={24} md={10}><Input aria-label="Описание проблемы" value={title} onChange={e => setTitle(e.target.value)}
                          placeholder="Что не работает?" maxLength={250} onPressEnter={addProblem} /></Col>
                        <Col xs={24} md={5}><Select style={{ width: '100%' }} value={kind} onChange={setKind} options={kinds} /></Col>
                        <Col xs={24} md={6}><Input value={manualEvidence} onChange={e => setManualEvidence(e.target.value)}
                          placeholder="Лог, ссылка или результат теста" maxLength={1000} /></Col>
                        <Col xs={24} md={3}><Button type="primary" block onClick={addProblem}>Добавить</Button></Col>
                      </Row>
                    </Card>
                    <Card size="small" title="Журнал проблем">
                      <Table size="small" rowKey="id" scroll={{ x: 850 }} dataSource={filteredProblems} pagination={{ pageSize: 10 }}
                        columns={[{ title: 'Описание', key: 'title', render: (_: unknown, p: CascadeProblem) =>
                          <div><strong>{p.title}</strong><div><Typography.Text type="secondary">{p.evidence || 'Нет доказательства'}</Typography.Text></div></div> },
                          { title: 'Маршрут', dataIndex: 'tag' }, { title: 'Тип', dataIndex: 'type' },
                          { title: 'Статус', dataIndex: 'status', render: (value: string) => evidenceTag(value) },
                          { title: 'Изменить', key: 'action', render: (_: unknown, p: CascadeProblem) =>
                            <Space wrap>{problemStatuses.filter(s => s.value !== p.status).map(s =>
                              <Button size="small" key={s.value} onClick={() => setProblemStatus(p.id, s.value)}>{s.label}</Button>)}</Space> },
                        ]} />
                    </Card>
                  </Space> },
                { key: 'logs', label: 'Логи и проверки', children:
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Card size="small" title="Доступные логи ядра основной панели">
                      <Button loading={logsLoading} onClick={() => void loadCoreLogs()}>Прочитать последние 100 записей</Button>
                      <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
                        API возвращает события основного ядра; удалённые ноды здесь не опрашиваются.
                        Отображаются только время и теги, без IP-адресов и идентификаторов клиентов.
                      </Typography.Paragraph>
                      <Table size="small" rowKey={(_, index) => index ?? 0} scroll={{ x: 540 }}
                        dataSource={coreLogs} pagination={{ pageSize: 10 }}
                        columns={[
                          { title: 'Время', dataIndex: 'DateTime', render: (value: string | number | undefined) => value ? new Date(value).toLocaleString('ru-RU') : '—' },
                          { title: 'Входящее', dataIndex: 'Inbound' },
                          { title: 'Исходящее', dataIndex: 'Outbound' },
                          { title: 'Результат', dataIndex: 'Event', render: (value: number | undefined) =>
                            <Tag color={value === 2 ? 'success' : value === 1 ? 'error' : 'default'}>{value === 2 ? 'PROXY' : value === 1 ? 'BLOCKED' : value === 0 ? 'DIRECT' : '—'}</Tag> },
                        ]} />
                    </Card>
                    <Card size="small" title="Журнал диагностических запусков">
                      {store.logs.length ? <Timeline items={store.logs.map((entry, i) => ({
                        key: i, children: <Typography.Text style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{entry}</Typography.Text>,
                      }))} /> : <Typography.Text type="secondary">Проверки ещё не запускались.</Typography.Text>}
                      <Divider /><Typography.Text type="secondary">Системные логи удалённых нод необходимо анализировать отдельно.</Typography.Text>
                    </Card>
                  </Space> },
                { key: 'checklist', label: 'Чек-лист', children:
                  <Card size="small" title="Диагностика до и после исправления">
                    <Space orientation="vertical" size="middle">
                      {DIAGNOSTIC_CHECKLIST.map((item, i) =>
                        <Checkbox key={item} checked={store.checklist[i] === true} onChange={e =>
                          setStore(s => ({ ...s, checklist: DIAGNOSTIC_CHECKLIST.map((_, index) =>
                            index === i ? e.target.checked : s.checklist[index] === true) }))}>
                          {i + 1}. {item}
                        </Checkbox>)}
                    </Space>
                    <Alert style={{ marginTop: 16 }} type="info" showIcon title="Повторная проверка после исправления"
                      description="После «Подготовить» заново запустите проверки. Отдельно выполните реальный клиентский тест через исходное входящее и сохраните логи обеих нод." />
                  </Card> },
                { key: 'merge', label: 'Подготовка к main', children:
                  <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
                    <Card size="small" title="Шлюз готовности к слиянию">
                      <Alert type={mergeReady ? 'success' : 'warning'} showIcon
                        title={mergeReady ? 'Локальный чек-лист готовности заполнен' : 'Слияние пока заблокировано проверками'}
                        description="Это операторская готовность, а не проверка CI GitHub. Фактическое слияние происходит отдельно после ревью и успешных Actions." />
                      <Divider />
                      <Typography.Paragraph>Нужно подтвердить: все выбранные маршруты проходят поддерживаемые автоматические этапы; отсутствуют непроверенные проблемы; приложен сквозной тест реального клиента; чек-лист завершён; CI и ревью отмечены как пройденные.</Typography.Paragraph>
                      <Typography.Text strong>Доказательство сквозного теста и ссылки на логи</Typography.Text>
                      <Input.TextArea rows={4} value={store.liveEvidence} onChange={e =>
                        setStore(s => ({ ...s, liveEvidence: e.target.value }))} placeholder="Номер теста, время, хэш конфигурации, ссылку на безопасный журнал; без паролей и ключей" />
                      <Checkbox checked={store.ciApproved} onChange={e =>
                        setStore(s => ({ ...s, ciApproved: e.target.checked }))}>
                        CI завершился успешно и изменения прошли ревью (подтверждается вручную)
                      </Checkbox>
                      <Space wrap>
                        <a href="https://github.com/SawaMEN/3x-ui/pulls" target="_blank" rel="noopener noreferrer">Pull requests проекта</a>
                        <a href="https://github.com/SawaMEN/3x-ui/actions" target="_blank" rel="noopener noreferrer">Проверки GitHub Actions</a>
                        <Link to="/nodes">Перейти к нодам</Link>
                      </Space>
                    </Card>
                  </Space> },
              ]} />
            </Space>
          </Layout.Content>
        </Layout>
      </Layout>
    </div>
    </ConfigProvider>
  );
}
