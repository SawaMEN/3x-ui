import OutboundSubscriptions from '@/pages/singbox/OutboundSubscriptions';
import { normalizeOutboundJsonImport } from './singbox-native-outbound';
import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Col,
  Dropdown,
  Modal,
  Popconfirm,
  Radio,
  Row,
  Space,
  Table,
  Tooltip,
  message,
} from 'antd';
import {
  PlusOutlined,
  CloudOutlined,
  ApiOutlined,
  MoreOutlined,
  RetweetOutlined,
  PlayCircleOutlined,
  ExportOutlined,
  ImportOutlined,
} from '@ant-design/icons';

import { isOutboundProtocol } from '@/schemas/primitives';
import PromptModal from '@/components/feedback/PromptModal';
import TextModal from '@/components/feedback/TextModal';

import OutboundFormModal from './OutboundFormModal';
import { propagateOutboundTagRename } from '../basics/helpers';
import { planOutboundDeletion, applyOutboundDeletion } from '../reference-cleanup';
import DeletionImpactList from '../DeletionImpactList';
import { isBalancerLoopbackTag } from '../balancers/balancer-loopback';
import type {
  XraySettingsValue,
  SetTemplate,
  OutboundTestMode,
  OutboundTestState,
  OutboundTrafficRow,
} from '@/hooks/useXraySetting';
import './OutboundsTab.css';

import type { OutboundRow } from './outbounds-tab-types';
import { originalOutboundIndex } from './outbounds-tab-helpers';
import { useOutboundColumns } from './useOutboundColumns';
import OutboundCardList from './OutboundCardList';
import SubscriptionOutbounds from './SubscriptionOutbounds';

interface OutboundsTabProps {
  templateSettings: XraySettingsValue | null;
  setTemplateSettings: SetTemplate;
  outboundsTraffic: OutboundTrafficRow[];
  outboundTestStates: Record<number, OutboundTestState>;
  subscriptionTestStates: Record<string, OutboundTestState>;
  testingAll: boolean;
  inboundTags: string[];
  subscriptionOutbounds?: unknown[];
  subscriptionOutboundTags?: string[];
  isMobile: boolean;
  onResetTraffic: (tag: string) => void;
  onTest: (index: number, mode: string) => void;
  onTestSubscription: (outbound: Record<string, unknown>, mode: string) => void;
  onTestAll: (mode: string) => void;
  onShowWarp: () => void;
  onShowNord: () => void;
  onShowPia: () => void;
  onRefreshXrayData?: () => void;
  onSaveTemplate?: (next?: XraySettingsValue) => Promise<boolean>;
}

export default function OutboundsTab({
  templateSettings,
  setTemplateSettings,
  outboundsTraffic,
  outboundTestStates,
  subscriptionTestStates,
  testingAll,
  inboundTags: _inboundTags,
  subscriptionOutbounds,
  subscriptionOutboundTags,
  isMobile,
  onResetTraffic,
  onTest,
  onTestSubscription,
  onTestAll,
  onShowWarp,
  onShowNord,
  onShowPia,
  onRefreshXrayData,
  onSaveTemplate,
}: OutboundsTabProps) {
  const { t } = useTranslation();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [testMode, setTestMode] = useState<OutboundTestMode>('tcp');
  const [modalOpen, setModalOpen] = useState(false);
  const [editingOutbound, setEditingOutbound] = useState<Record<string, unknown> | null>(null);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);
  const [existingTags, setExistingTags] = useState<string[]>([]);

  const [subDrawerOpen, setSubDrawerOpen] = useState(false);
  const mutationBusy = useRef(false);

  const outbounds = useMemo(
    () => (templateSettings?.outbounds || []) as unknown as OutboundRow[],
    [templateSettings?.outbounds],
  );

  const rows = useMemo(
    () =>
      outbounds
        .map((o, i) => ({ ...o, key: i }))
        .filter((o) => !isBalancerLoopbackTag(o.tag || '')),
    [outbounds],
  );

  const dialerProxyTags = useMemo(() => {
    const tags = new Set<string>();
    (templateSettings?.outbounds || []).forEach((o, i) => {
      if (i === editingIndex) return;
      if (isOutboundProtocol(o, 'blackhole')) return;
      if (o?.tag) tags.add(o.tag);
    });
    for (const tag of subscriptionOutboundTags || []) {
      if (tag) tags.add(tag);
    }
    return [...tags];
  }, [templateSettings?.outbounds, editingIndex, subscriptionOutboundTags]);

  const mutate = useCallback(
    async (mutator: (next: XraySettingsValue) => void): Promise<boolean> => {
      if (!templateSettings || mutationBusy.current) return false;
      const clone = JSON.parse(JSON.stringify(templateSettings)) as XraySettingsValue;
      mutator(clone);
      if (!onSaveTemplate) {
        setTemplateSettings(clone);
        return true;
      }
      mutationBusy.current = true;
      try {
        return await onSaveTemplate(clone);
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : String(error));
        return false;
      } finally {
        mutationBusy.current = false;
      }
    },
    [templateSettings, setTemplateSettings, onSaveTemplate, messageApi],
  );

  function openAdd() {
    setEditingOutbound(null);
    setEditingIndex(null);
    setExistingTags(
      (templateSettings?.outbounds || []).map((o) => o?.tag).filter((tg): tg is string => !!tg),
    );
    setModalOpen(true);
  }

  async function openSubManager() {
    try {
      if (onSaveTemplate && !(await onSaveTemplate())) return;
      setSubDrawerOpen(true);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    }
  }
  function openEdit(idx: number) {
    const target = originalOutboundIndex(rows, idx);
    setEditingOutbound((templateSettings?.outbounds || [])[target] as Record<string, unknown>);
    setEditingIndex(target);
    setExistingTags(
      (templateSettings?.outbounds || [])
        .filter((_, i) => i !== target)
        .map((o) => o?.tag)
        .filter((tg): tg is string => !!tg),
    );
    setModalOpen(true);
  }
  async function onConfirm(outbound: Record<string, unknown>) {
    const saved = await mutate((tt) => {
      if (!Array.isArray(tt.outbounds)) tt.outbounds = [];
      const newTag = typeof outbound.tag === 'string' ? outbound.tag : '';
      if (editingIndex == null) {
        if (!newTag) return;
        tt.outbounds.push(outbound as never);
      } else {
        const oldTag = tt.outbounds[editingIndex]?.tag;
        tt.outbounds[editingIndex] = outbound as never;
        if (oldTag && newTag && oldTag !== newTag) {
          propagateOutboundTagRename(tt, oldTag, newTag);
        }
      }
    });
    if (saved) setModalOpen(false);
  }

  function confirmDelete(idx: number) {
    const target = originalOutboundIndex(rows, idx);
    const impact = templateSettings
      ? planOutboundDeletion(templateSettings, target)
      : { rules: [], balancers: [], observatory: false, burst: false };
    modal.confirm({
      title: `${t('delete')} ${t('pages.xray.Outbounds')} #${idx + 1}?`,
      content: <DeletionImpactList impact={impact} />,
      okText: t('delete'),
      okType: 'danger',
      cancelText: t('cancel'),
      onOk: async () => {
        if (!(await mutate((tt) => applyOutboundDeletion(tt, target))))
          throw new Error('Не удалось удалить подключение');
      },
    });
  }
  function setFirst(idx: number) {
    const target = originalOutboundIndex(rows, idx);
    mutate((tt) => {
      if (!tt.outbounds) return;
      const [moved] = tt.outbounds.splice(target, 1);
      tt.outbounds.unshift(moved);
    });
  }
  function moveUp(idx: number) {
    if (idx <= 0) return;
    const target = originalOutboundIndex(rows, idx);
    const prev = originalOutboundIndex(rows, idx - 1);
    mutate((tt) => {
      if (!tt.outbounds) return;
      [tt.outbounds[prev], tt.outbounds[target]] = [tt.outbounds[target], tt.outbounds[prev]];
    });
  }
  function moveDown(idx: number) {
    if (idx >= rows.length - 1) return;
    const target = originalOutboundIndex(rows, idx);
    const next = originalOutboundIndex(rows, idx + 1);
    mutate((tt) => {
      if (!tt.outbounds) return;
      [tt.outbounds[next], tt.outbounds[target]] = [tt.outbounds[target], tt.outbounds[next]];
    });
  }

  const [importOpen, setImportOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [exportContent, setExportContent] = useState('');

  function exportOutbounds() {
    setExportContent(JSON.stringify(outbounds, null, 2));
    setExportOpen(true);
  }

  function importOutbounds(value: string) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(value);
    } catch {
      messageApi.error(t('pages.xray.importInvalidJson'));
      return;
    }
    let list: Record<string, unknown>[];
    try {
      list = normalizeOutboundJsonImport(
        parsed,
        outbounds.map((outbound) => String(outbound.tag ?? '')),
      );
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('pages.xray.importInvalidJson'));
      return;
    }
    mutate((tt) => {
      if (!Array.isArray(tt.outbounds)) tt.outbounds = [];
      tt.outbounds.push(...(list as never[]));
    });
    setImportOpen(false);
  }

  const columns = useOutboundColumns({
    testMode,
    rows,
    outboundsTraffic,
    outboundTestStates,
    openEdit,
    setFirst,
    moveUp,
    moveDown,
    confirmDelete,
    onResetTraffic,
    onTest,
  });

  return (
    <>
      {modalContextHolder}
      {messageContextHolder}
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Row gutter={[12, 12]} align="middle" justify="space-between">
          <Col xs={24} sm={12}>
            <Space size="small" wrap>
              <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={openAdd}>
                {!isMobile && t('pages.xray.Outbounds')}
              </Button>
              <Button icon={<CloudOutlined aria-hidden />} onClick={openSubManager}>
                {t('pages.xray.outboundSub.manage')}
              </Button>
              <Dropdown
                trigger={['click']}
                menu={{
                  items: [
                    {
                      key: 'warp',
                      icon: <CloudOutlined aria-hidden />,
                      label: 'WARP',
                      onClick: onShowWarp,
                    },
                    {
                      key: 'nord',
                      icon: <ApiOutlined aria-hidden />,
                      label: 'NordVPN',
                      onClick: onShowNord,
                    },
                    {
                      key: 'pia',
                      icon: <ApiOutlined aria-hidden />,
                      label: t('pages.xray.pia.menu'),
                      onClick: onShowPia,
                    },
                    { type: 'divider' },
                    {
                      key: 'import',
                      icon: <ImportOutlined aria-hidden />,
                      label: t('pages.xray.importOutbounds'),
                      onClick: () => setImportOpen(true),
                    },
                    {
                      key: 'export',
                      icon: <ExportOutlined aria-hidden />,
                      label: t('pages.xray.exportOutbounds'),
                      disabled: outbounds.length === 0,
                      onClick: exportOutbounds,
                    },
                  ],
                }}
              >
                <Button icon={<MoreOutlined aria-hidden />}>{t('more')}</Button>
              </Dropdown>
            </Space>
          </Col>
          <Col xs={24} sm={12} className="toolbar-right">
            <Space size="small" wrap>
              <Tooltip title={t('pages.xray.outbound.testModeTooltip')}>
                <Radio.Group
                  value={testMode}
                  onChange={(e) => setTestMode(e.target.value)}
                  buttonStyle="solid"
                  size="small"
                >
                  <Radio.Button value="tcp">TCP</Radio.Button>
                  <Radio.Button value="http">HTTP</Radio.Button>
                  <Radio.Button value="real">{t('pages.xray.outbound.modeRealDelay')}</Radio.Button>
                </Radio.Group>
              </Tooltip>
              <Button
                type="primary"
                loading={testingAll}
                icon={<PlayCircleOutlined aria-hidden />}
                onClick={() => onTestAll(testMode)}
              >
                {!isMobile && t('pages.xray.outbound.testAll')}
              </Button>
              <Popconfirm
                placement="topRight"
                okText={t('reset')}
                cancelText={t('cancel')}
                title={t('pages.inbounds.resetAllTrafficContent')}
                onConfirm={() => onResetTraffic('-alltags-')}
              >
                <Button
                  aria-label={t('pages.inbounds.resetTraffic')}
                  icon={<RetweetOutlined aria-hidden />}
                />
              </Popconfirm>
            </Space>
          </Col>
        </Row>

        {isMobile ? (
          <OutboundCardList
            rows={rows}
            testMode={testMode}
            outboundsTraffic={outboundsTraffic}
            outboundTestStates={outboundTestStates}
            setFirst={setFirst}
            openEdit={openEdit}
            onResetTraffic={onResetTraffic}
            confirmDelete={confirmDelete}
            onTest={onTest}
          />
        ) : (
          <Table
            columns={columns}
            dataSource={rows}
            rowKey={(r) => r.key}
            pagination={false}
            size="small"
            locale={{
              emptyText: (
                <div className="card-empty">
                  <ExportOutlined aria-hidden style={{ fontSize: 32, marginBottom: 8 }} />
                  <div>{t('noData')}</div>
                </div>
              ),
            }}
          />
        )}

        <OutboundFormModal
          open={modalOpen}
          outbound={editingOutbound}
          existingTags={existingTags}
          dialerProxyTags={dialerProxyTags}
          onClose={() => setModalOpen(false)}
          onOpenSubscriptions={() => {
            setModalOpen(false);
            openSubManager();
          }}
          onConfirm={onConfirm}
        />
        <PromptModal
          open={importOpen}
          onClose={() => setImportOpen(false)}
          title={t('pages.xray.importOutbounds')}
          okText={t('pages.xray.importOutbounds')}
          type="textarea"
          json
          onConfirm={importOutbounds}
        />
        <TextModal
          open={exportOpen}
          onClose={() => setExportOpen(false)}
          title={t('pages.xray.exportOutbounds')}
          content={exportContent}
          fileName="outbounds.json"
          json
        />

        {/* Subscription outbounds (read-only, merged at runtime) */}
        {Array.isArray(subscriptionOutbounds) && subscriptionOutbounds.length > 0 && (
          <SubscriptionOutbounds
            subscriptionOutbounds={subscriptionOutbounds}
            outboundsTraffic={outboundsTraffic}
            subscriptionTestStates={subscriptionTestStates}
            testMode={testMode}
            isMobile={isMobile}
            onTestSubscription={onTestSubscription}
          />
        )}
      </Space>

      <OutboundSubscriptions
        open={subDrawerOpen}
        initialURL=""
        coreType="xray"
        onClose={() => setSubDrawerOpen(false)}
        replacementTags={outbounds.map((row) => row.tag || '').filter(Boolean)}
        onTagsChanged={() => onRefreshXrayData?.()}
      />
    </>
  );
}
