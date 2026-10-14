import { useCallback, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { DatabaseOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';

import { GeoBrowserModal } from '@/components/geodata';
import { HttpUtil } from '@/utils';
import type { XraySettingsValue, SetTemplate } from '@/hooks/useXraySetting';
import type { RuleObject } from '@/schemas/routing';
import { catalogRule, putCatalogRule, type CatalogSelection } from './catalog-rules';

interface Category {
  id: string;
  token: string;
  available: boolean;
}
interface RoutingList {
  id: number;
  name: string;
  url: string;
  updateInterval: number;
  domainCount: number;
  ipCount: number;
  lastUpdated: number;
  lastError: string;
}
interface Entries {
  domains: string[];
  ips: string[];
}
interface SourceForm {
  id: number;
  name: string;
  url: string;
  updateInterval: number;
}
const API = '/panel/api/xray/routingCatalog';
const EMPTY_SOURCE: SourceForm = { id: 0, name: '', url: '', updateInterval: 86400 };

interface Props {
  templateSettings?: XraySettingsValue | null;
  setTemplateSettings?: SetTemplate;
  assignedRules?: RuleObject[];
  onAssign?: (selection: CatalogSelection, outbound: string) => void;
  outboundTags: string[];
}

export default function RoutingCatalogTab({
  templateSettings,
  setTemplateSettings,
  outboundTags,
  assignedRules,
  onAssign,
}: Props) {
  const { t } = useTranslation();
  const tr = useCallback(
    (key: string, options?: Record<string, unknown>) =>
      t(`pages.xray.routingCatalog.${key}`, options),
    [t],
  );
  const [search, setSearch] = useState('');
  const [pending, setBusy] = useState(false);
  const [source, setSource] = useState<SourceForm | null>(null);
  const [selected, setSelected] = useState<CatalogSelection | null>(null);
  const [outbound, setOutbound] = useState('');
  const [geoToken, setGeoToken] = useState('');
  const [preview, setPreview] = useState<{ title: string; entries: Entries } | null>(null);
  const [previewFilter, setPreviewFilter] = useState('');

  const { data, error, isFetching, refetch } = useQuery({
    queryKey: ['routing-catalog'],
    refetchInterval: 60000,
    queryFn: async () => {
      const [builtins, custom] = await Promise.all([
        HttpUtil.get<Category[]>(`${API}/categories`, undefined, { silent: true }),
        HttpUtil.get<RoutingList[]>(`${API}/list`, undefined, { silent: true }),
      ]);
      if (!builtins?.success || !custom?.success) {
        throw new Error(builtins?.msg || custom?.msg || tr('loadFailed'));
      }
      return { categories: builtins.obj || [], lists: custom.obj || [] };
    },
  });
  const categories = data?.categories || [];
  const lists = data?.lists || [];
  const busy = pending || isFetching;
  const loadError = error?.message || '';
  const currentRules = assignedRules || templateSettings?.routing?.rules || [];
  const canAssign = !!onAssign || !!templateSettings;
  const load = async () => {
    await refetch();
  };

  function select(item: CatalogSelection) {
    const existing = currentRules.find((r) =>
      item.listId ? r.routingListId === item.listId : r.ruleTag === `catalog-${item.token}`,
    );
    setOutbound(existing?.outboundTag || outboundTags.find(Boolean) || '');
    setSelected(item);
  }

  function apply() {
    if (!selected || !outbound || !canAssign || !outboundTags.includes(outbound)) return;
    if (onAssign) {
      onAssign(selected, outbound);
    } else if (setTemplateSettings) {
      const next = catalogRule(selected, outbound);
      setTemplateSettings((prev) =>
        prev
          ? {
              ...prev,
              routing: { ...prev.routing, rules: putCatalogRule(prev.routing?.rules || [], next) },
            }
          : prev,
      );
    }
    setSelected(null);
    message.success(tr('added'));
  }

  async function saveSource() {
    if (!source) return;
    setBusy(true);
    try {
      const result = await HttpUtil.post<RoutingList>(`${API}/save`, source, {
        headers: { 'Content-Type': 'application/json' },
        silent: true,
      });
      if (!result?.success) {
        message.error(result?.msg || tr('saveFailed'));
        return;
      }
      setSource(null);
      await load();
      message.success(tr('sourceSaved'));
    } finally {
      setBusy(false);
    }
  }

  async function refresh(row: RoutingList) {
    setBusy(true);
    try {
      const result = await HttpUtil.post(`${API}/${row.id}/refresh`, undefined, { silent: true });
      if (!result?.success) message.error(result?.msg || tr('refreshFailed'));
      else message.success(tr('refreshed'));
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function remove(row: RoutingList) {
    const staged = currentRules.some((rule) => rule.routingListId === row.id);
    if (staged) {
      message.warning(tr('removeRulesFirst'));
      return;
    }
    setBusy(true);
    try {
      const result = await HttpUtil.post(`${API}/${row.id}/delete`, undefined, { silent: true });
      if (!result?.success) message.error(result?.msg || tr('deleteFailed'));
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function view(row: RoutingList) {
    setBusy(true);
    try {
      const result = await HttpUtil.get<Entries>(`${API}/${row.id}/entries`, undefined, {
        silent: true,
      });
      if (!result?.success || !result.obj) {
        message.error(result?.msg || tr('loadFailed'));
        return;
      }
      setPreviewFilter('');
      setPreview({ title: row.name, entries: result.obj });
    } finally {
      setBusy(false);
    }
  }

  const needle = search.trim().toLowerCase();
  const filteredCategories = categories.filter((row) =>
    `${tr(`categories.${row.id}`)} ${row.token}`.toLowerCase().includes(needle),
  );
  const filteredLists = lists.filter((row) => row.name.toLowerCase().includes(needle));
  const previewRows = useMemo(
    () =>
      [...(preview?.entries.domains || []), ...(preview?.entries.ips || [])]
        .filter((value) => value.toLowerCase().includes(previewFilter.trim().toLowerCase()))
        .map((value) => ({ value })),
    [preview, previewFilter],
  );
  const sourceUsed =
    selected?.listId && currentRules.some((r) => r.routingListId === selected.listId);
  const overlaps =
    selected?.token &&
    currentRules.some(
      (rule) =>
        rule.enabled !== false &&
        rule.ruleTag !== `catalog-${selected.token}` &&
        rule.domain?.includes(selected.token!),
    );

  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Alert type="info" showIcon title={tr('intro')} description={tr('priorityHelp')} />
      {loadError && <Alert type="error" showIcon title={loadError} />}
      <Space wrap>
        <Input.Search
          allowClear
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={tr('search')}
          style={{ width: 280 }}
        />
        <Button icon={<ReloadOutlined />} loading={busy} onClick={() => void load()}>
          {tr('reload')}
        </Button>
        <Button
          icon={<PlusOutlined />}
          disabled={busy}
          onClick={() => setSource({ ...EMPTY_SOURCE })}
        >
          {tr('addSource')}
        </Button>
      </Space>
      <Card title={tr('builtins')} size="small">
        <Typography.Paragraph type="secondary">{tr('geodataHelp')}</Typography.Paragraph>
        <Table<Category>
          rowKey="id"
          size="small"
          loading={busy}
          dataSource={filteredCategories}
          pagination={false}
          scroll={{ x: 640 }}
          columns={[
            {
              title: tr('name'),
              render: (_, row) => (
                <Space orientation="vertical" size={0}>
                  <span>{tr(`categories.${row.id}`)}</span>
                  <Typography.Text type="secondary">{row.token}</Typography.Text>
                </Space>
              ),
            },
            {
              title: tr('status'),
              render: (_, row) => (
                <Tag color={row.available ? 'green' : 'orange'}>
                  {tr(row.available ? 'available' : 'unavailable')}
                </Tag>
              ),
            },
            {
              title: tr('actions'),
              render: (_, row) => (
                <Space wrap>
                  <Button
                    size="small"
                    icon={<DatabaseOutlined />}
                    disabled={!row.available}
                    onClick={() => setGeoToken(row.token)}
                  >
                    {tr('contents')}
                  </Button>
                  <Button
                    size="small"
                    type="primary"
                    disabled={!row.available || !canAssign}
                    onClick={() => select({ name: tr(`categories.${row.id}`), token: row.token })}
                  >
                    {tr('route')}
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Card title={tr('customSources')} size="small">
        <Table<RoutingList>
          rowKey="id"
          size="small"
          loading={busy}
          dataSource={filteredLists}
          pagination={{ pageSize: 10 }}
          scroll={{ x: 850 }}
          columns={[
            { title: tr('name'), dataIndex: 'name' },
            {
              title: tr('entries'),
              render: (_, row) => `${row.domainCount} ${tr('domains')} / ${row.ipCount} IP`,
            },
            {
              title: tr('updated'),
              render: (_, row) => (
                <Space orientation="vertical" size={0}>
                  <span>
                    {row.lastUpdated ? new Date(row.lastUpdated * 1000).toLocaleString() : '—'}
                  </span>
                  <Typography.Text type="secondary">
                    {row.updateInterval
                      ? tr('everyHours', { hours: row.updateInterval / 3600 })
                      : tr('manual')}
                  </Typography.Text>
                  {row.lastError && (
                    <Typography.Text type="danger">
                      {tr('cacheKept')}: {row.lastError}
                    </Typography.Text>
                  )}
                </Space>
              ),
            },
            {
              title: tr('actions'),
              render: (_, row) => (
                <Space wrap>
                  <Button size="small" disabled={busy} onClick={() => void view(row)}>
                    {tr('contents')}
                  </Button>
                  <Button
                    size="small"
                    type="primary"
                    disabled={busy || !canAssign}
                    onClick={() => select({ name: row.name, listId: row.id })}
                  >
                    {tr('route')}
                  </Button>
                  <Button size="small" disabled={busy} onClick={() => void refresh(row)}>
                    {tr('update')}
                  </Button>
                  <Button
                    size="small"
                    disabled={busy}
                    onClick={() =>
                      setSource({
                        id: row.id,
                        name: row.name,
                        url: row.url,
                        updateInterval: row.updateInterval,
                      })
                    }
                  >
                    {t('edit')}
                  </Button>
                  <Button size="small" danger disabled={busy} onClick={() => void remove(row)}>
                    {t('delete')}
                  </Button>
                </Space>
              ),
            },
          ]}
        />
      </Card>
      <Modal
        open={!!source}
        title={tr(source?.id ? 'editSource' : 'addSource')}
        onCancel={() => !busy && setSource(null)}
        onOk={() => void saveSource()}
        confirmLoading={busy}
        okButtonProps={{ disabled: !source?.name.trim() || !source?.url.trim() }}
      >
        {source && (
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Typography.Text>{tr('name')}</Typography.Text>
            <Input
              aria-label={tr('name')}
              maxLength={120}
              value={source.name}
              onChange={(e) => setSource({ ...source, name: e.target.value })}
            />
            <Typography.Text>{tr('sourceURL')}</Typography.Text>
            <Input
              aria-label={tr('sourceURL')}
              maxLength={4096}
              placeholder="https://example.com/domains.txt"
              value={source.url}
              onChange={(e) => setSource({ ...source, url: e.target.value })}
            />
            <Typography.Text>{tr('interval')}</Typography.Text>
            <InputNumber
              aria-label={tr('interval')}
              min={0}
              max={168}
              value={source.updateInterval / 3600}
              onChange={(value) =>
                setSource({ ...source, updateInterval: Math.round((value || 0) * 3600) })
              }
            />
            <Alert type="info" title={tr('formats')} description={tr('sourceHelp')} />
          </Space>
        )}
      </Modal>
      <Modal
        open={!!selected}
        title={`${tr('route')}: ${selected?.name || ''}`}
        onCancel={() => setSelected(null)}
        onOk={apply}
        okText={tr('addRule')}
        okButtonProps={{ disabled: !outbound || !outboundTags.includes(outbound) }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Text>{tr('outbound')}</Typography.Text>
          <Select
            aria-label={tr('outbound')}
            showSearch
            style={{ width: '100%' }}
            value={outbound || undefined}
            onChange={setOutbound}
            options={outboundTags.filter(Boolean).map((value) => ({ value, label: value }))}
          />
          {(overlaps || sourceUsed) && (
            <Alert type="warning" title={tr(sourceUsed ? 'replaceExisting' : 'overlap')} />
          )}
          <Alert type="info" title={tr('saveHint')} description={tr('priorityHelp')} />
        </Space>
      </Modal>
      {geoToken && (
        <GeoBrowserModal
          open
          kind="site"
          value={geoToken}
          readOnly
          onApply={() => {}}
          onClose={() => setGeoToken('')}
        />
      )}
      <Modal
        open={!!preview}
        title={preview?.title}
        footer={null}
        width={720}
        onCancel={() => setPreview(null)}
      >
        <Input.Search
          allowClear
          value={previewFilter}
          onChange={(e) => setPreviewFilter(e.target.value)}
          placeholder={tr('search')}
          style={{ marginBottom: 12 }}
        />
        <Table
          rowKey="value"
          size="small"
          dataSource={previewRows}
          columns={[{ title: tr('entries'), dataIndex: 'value' }]}
          pagination={{ pageSize: 50, showSizeChanger: false }}
          scroll={{ y: 360 }}
        />
      </Modal>
    </Space>
  );
}
