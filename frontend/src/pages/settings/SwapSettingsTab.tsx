import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  InputNumber,
  Result,
  Popconfirm,
  Row,
  Select,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  message,
} from 'antd';
import {
  DeleteOutlined,
  DownloadOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { onNumber } from '@/utils/onNumber';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import SystemUpdateModal from './SystemUpdateModal';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };

type SwapArea = {
  path: string;
  type: string;
  sizeBytes: number;
  usedBytes: number;
  priority: number;
  active: boolean;
  managed: boolean;
};

type ZramArea = {
  device: string;
  id: number;
  sizeBytes: number;
  usedBytes: number;
  compressedBytes: number;
  memoryUsedBytes: number;
  memoryLimitBytes: number;
  algorithm: string;
  algorithms: string[];
  streams: number;
  priority: number;
  active: boolean;
  managed: boolean;
};

type Recommendation = {
  totalRamBytes: number;
  cpuCount: number;
  recommendedSwapMiB: number;
  recommendedZramMiB: number;
  recommendedSwappiness: number;
  reason: string;
};

type ZramInstallInfo = {
  distribution: string;
  version: string;
  packageManager: string;
  package: string;
  installed: boolean;
  supported: boolean;
  usingGenerator: boolean;
  installedPackages: { name: string; version: string; installed: boolean }[];
  recommendedPackage: string;
  recommendedVersion: string;
  recommendedInstalled: boolean;
  activeBackend: string;
  installCommand: string;
  reinstallCommand: string;
  configPath: string;
};

type SwapStatus = {
  totalBytes: number;
  usedBytes: number;
  swappiness: number;
  areas: SwapArea[];
  zram: ZramArea[];
  zramAlgorithms: string[];
  swapFileConfig: { enabled: boolean; path: string; sizeBytes: number; priority: number };
  zramConfig: {
    enabled: boolean;
    device: string;
    sizeBytes: number;
    algorithm: string;
    streams: number;
    memoryLimitBytes: number;
    priority: number;
  };
  ramTotalBytes: number;
  ramAvailableBytes: number;
  cpuCount: number;
  recommendation: Recommendation;
  zramInstall: ZramInstallInfo;
};

function toFiniteNumber(value: unknown, fallback = 0) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function normalizeSwapStatus(value: unknown): SwapStatus {
  const raw =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  const swapFile =
    raw.swapFileConfig && typeof raw.swapFileConfig === 'object'
      ? (raw.swapFileConfig as Record<string, unknown>)
      : {};
  const zramConfig =
    raw.zramConfig && typeof raw.zramConfig === 'object'
      ? (raw.zramConfig as Record<string, unknown>)
      : {};
  const recommendation =
    raw.recommendation && typeof raw.recommendation === 'object'
      ? (raw.recommendation as Record<string, unknown>)
      : {};
  const zramInstall =
    raw.zramInstall && typeof raw.zramInstall === 'object'
      ? (raw.zramInstall as Record<string, unknown>)
      : {};
  const areas = Array.isArray(raw.areas) ? raw.areas : [];
  const zram = Array.isArray(raw.zram) ? raw.zram : [];
  return {
    totalBytes: toFiniteNumber(raw.totalBytes),
    usedBytes: toFiniteNumber(raw.usedBytes),
    swappiness: toFiniteNumber(raw.swappiness, 60),
    areas: areas.map((item) => {
      const row = item && typeof item === 'object' ? (item as Record<string, unknown>) : {};
      return {
        path: typeof row.path === 'string' ? row.path : '',
        type: typeof row.type === 'string' ? row.type : '',
        sizeBytes: toFiniteNumber(row.sizeBytes),
        usedBytes: toFiniteNumber(row.usedBytes),
        priority: toFiniteNumber(row.priority),
        active: row.active === true,
        managed: row.managed === true,
      };
    }),
    zram: zram.map((item) => {
      const row = item && typeof item === 'object' ? (item as Record<string, unknown>) : {};
      return {
        device: typeof row.device === 'string' ? row.device : '',
        id: toFiniteNumber(row.id),
        sizeBytes: toFiniteNumber(row.sizeBytes),
        usedBytes: toFiniteNumber(row.usedBytes),
        compressedBytes: toFiniteNumber(row.compressedBytes),
        memoryUsedBytes: toFiniteNumber(row.memoryUsedBytes),
        memoryLimitBytes: toFiniteNumber(row.memoryLimitBytes),
        algorithm: typeof row.algorithm === 'string' ? row.algorithm : '',
        algorithms: Array.isArray(row.algorithms)
          ? row.algorithms.filter((item): item is string => typeof item === 'string')
          : [],
        streams: toFiniteNumber(row.streams),
        priority: toFiniteNumber(row.priority),
        active: row.active === true,
        managed: row.managed === true,
      };
    }),
    zramAlgorithms: Array.isArray(raw.zramAlgorithms)
      ? raw.zramAlgorithms.filter((item): item is string => typeof item === 'string')
      : [],
    swapFileConfig: {
      enabled: swapFile.enabled === true,
      path: typeof swapFile.path === 'string' ? swapFile.path : '',
      sizeBytes: toFiniteNumber(swapFile.sizeBytes),
      priority: toFiniteNumber(swapFile.priority, 100),
    },
    zramConfig: {
      enabled: zramConfig.enabled === true,
      device: typeof zramConfig.device === 'string' ? zramConfig.device : '',
      sizeBytes: toFiniteNumber(zramConfig.sizeBytes),
      algorithm: typeof zramConfig.algorithm === 'string' ? zramConfig.algorithm : '',
      streams: toFiniteNumber(zramConfig.streams, 4),
      memoryLimitBytes: toFiniteNumber(zramConfig.memoryLimitBytes),
      priority: toFiniteNumber(zramConfig.priority, 200),
    },
    ramTotalBytes: toFiniteNumber(raw.ramTotalBytes),
    ramAvailableBytes: toFiniteNumber(raw.ramAvailableBytes),
    cpuCount: toFiniteNumber(raw.cpuCount),
    recommendation: {
      totalRamBytes: toFiniteNumber(recommendation.totalRamBytes),
      cpuCount: toFiniteNumber(recommendation.cpuCount),
      recommendedSwapMiB: toFiniteNumber(recommendation.recommendedSwapMiB, 1024),
      recommendedZramMiB: toFiniteNumber(recommendation.recommendedZramMiB, 1024),
      recommendedSwappiness: toFiniteNumber(recommendation.recommendedSwappiness, 60),
      reason: typeof recommendation.reason === 'string' ? recommendation.reason : '',
    },
    zramInstall: {
      distribution: typeof zramInstall.distribution === 'string' ? zramInstall.distribution : '',
      version: typeof zramInstall.version === 'string' ? zramInstall.version : '',
      packageManager:
        typeof zramInstall.packageManager === 'string' ? zramInstall.packageManager : '',
      package: typeof zramInstall.package === 'string' ? zramInstall.package : '',
      installed: zramInstall.installed === true,
      supported: zramInstall.supported === true,
      usingGenerator: zramInstall.usingGenerator === true,
      installedPackages: Array.isArray(zramInstall.installedPackages)
        ? zramInstall.installedPackages
            .filter((item) => item && typeof item === 'object')
            .map((item) => {
              const row = item as Record<string, unknown>;
              return {
                name: typeof row.name === 'string' ? row.name : '',
                version: typeof row.version === 'string' ? row.version : '',
                installed: row.installed === true,
              };
            })
        : [],
      recommendedPackage:
        typeof zramInstall.recommendedPackage === 'string'
          ? zramInstall.recommendedPackage
          : typeof zramInstall.package === 'string'
            ? zramInstall.package
            : '',
      recommendedVersion:
        typeof zramInstall.recommendedVersion === 'string' ? zramInstall.recommendedVersion : '',
      recommendedInstalled:
        zramInstall.recommendedInstalled === true || zramInstall.installed === true,
      activeBackend: typeof zramInstall.activeBackend === 'string' ? zramInstall.activeBackend : '',
      installCommand:
        typeof zramInstall.installCommand === 'string' ? zramInstall.installCommand : '',
      reinstallCommand:
        typeof zramInstall.reinstallCommand === 'string' ? zramInstall.reinstallCommand : '',
      configPath: typeof zramInstall.configPath === 'string' ? zramInstall.configPath : '',
    },
  };
}

function mib(bytes: number) {
  return (bytes / 1024 / 1024).toFixed(1) + ' MiB';
}

function gib(bytes: number) {
  return (bytes / 1024 / 1024 / 1024).toFixed(2) + ' GiB';
}

export default function SwapSettingsTab() {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [messageApi, contextHolder] = message.useMessage();
  const [status, setStatus] = useState<SwapStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [busy, setBusy] = useState(false);
  const [swapSize, setSwapSize] = useState(1024);
  const [swapPriority, setSwapPriority] = useState(100);
  const [zramSize, setZramSize] = useState(1024);
  const [zramPriority, setZramPriority] = useState(200);
  const [zramStreams, setZramStreams] = useState(4);
  const [zramLimit, setZramLimit] = useState(0);
  const [zramAlgorithm, setZramAlgorithm] = useState('');
  const [swappiness, setSwappiness] = useState(60);
  const [systemUpdateOpen, setSystemUpdateOpen] = useState(false);

  const refresh = useCallback(async () => {
    try {
      setLoadError('');
      const msg = (await HttpUtil.get('/panel/api/setting/swap/status')) as ApiMsg<unknown>;
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load swap status');
      const next = normalizeSwapStatus(msg.obj);
      setStatus(next);
      setSwappiness(next.swappiness);
      if (next.zramConfig.sizeBytes) {
        setZramSize(Math.round(next.zramConfig.sizeBytes / 1024 / 1024));
      }
      if (next.zramConfig.priority >= 0) setZramPriority(next.zramConfig.priority);
      if (next.zramConfig.streams > 0) setZramStreams(next.zramConfig.streams);
      setZramLimit(Math.round(next.zramConfig.memoryLimitBytes / 1024 / 1024));
      if (next.zramConfig.algorithm) setZramAlgorithm(next.zramConfig.algorithm);
      if (next.swapFileConfig.sizeBytes) {
        setSwapSize(Math.round(next.swapFileConfig.sizeBytes / 1024 / 1024));
      }
      if (next.swapFileConfig.priority >= 0) setSwapPriority(next.swapFileConfig.priority);
      if (!next.zramConfig.algorithm && next.zram.length > 0 && next.zram[0].algorithm) {
        setZramAlgorithm(next.zram[0].algorithm);
      }
      setLoading(false);
    } catch (error) {
      const text = error instanceof Error ? error.message : String(error);
      setLoadError(text);
      messageApi.error(text);
      setLoading(false);
    }
  }, [messageApi]);

  useEffect(() => {
    const initial = window.setTimeout(() => void refresh(), 0);
    const timer = window.setInterval(() => void refresh(), 4000);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(timer);
    };
  }, [refresh]);

  const action = async (fn: () => Promise<ApiMsg>) => {
    setBusy(true);
    try {
      const msg = await fn();
      if (!msg?.success) throw new Error(msg?.msg || 'Operation failed');
      await refresh();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  };

  const installZram = () =>
    action(() => HttpUtil.post('/panel/api/setting/swap/zram/install') as Promise<ApiMsg>);

  const reinstallZram = () =>
    action(() => HttpUtil.post('/panel/api/setting/swap/zram/reinstall') as Promise<ApiMsg>);

  const algorithmOptions = (status?.zramAlgorithms ?? []).map((value) => ({ label: value, value }));

  const zramColumns = [
    { title: t('pages.settings.swap.device'), dataIndex: 'device', key: 'device' },
    { title: t('pages.settings.swap.algorithm'), dataIndex: 'algorithm', key: 'algorithm' },
    {
      title: t('pages.settings.swap.size'),
      key: 'size',
      render: (_: unknown, row: ZramArea) => mib(row.sizeBytes),
    },
    {
      title: t('pages.settings.swap.used'),
      key: 'used',
      render: (_: unknown, row: ZramArea) => mib(row.usedBytes),
    },
    {
      title: t('pages.settings.swap.compressed'),
      key: 'compressed',
      render: (_: unknown, row: ZramArea) => mib(row.compressedBytes),
    },
    {
      title: t('pages.settings.swap.memoryUsed'),
      key: 'memoryUsed',
      render: (_: unknown, row: ZramArea) => mib(row.memoryUsedBytes),
    },
    { title: t('pages.settings.swap.priority'), dataIndex: 'priority', key: 'priority' },
    {
      title: t('pages.settings.swap.status'),
      key: 'status',
      render: (_: unknown, row: ZramArea) => (
        <Space>
          <Tag color={row.active ? 'success' : 'default'}>
            {row.active ? t('pages.settings.swap.active') : t('pages.settings.swap.inactive')}
          </Tag>
          {row.managed && <Tag color="blue">{t('pages.settings.swap.managed')}</Tag>}
        </Space>
      ),
    },
    {
      title: t('pages.settings.swap.actions'),
      key: 'actions',
      render: (_: unknown, row: ZramArea) =>
        row.managed ? (
          <Popconfirm
            title={t('pages.settings.swap.deleteZramConfirm')}
            onConfirm={() =>
              void action(
                () => HttpUtil.post('/panel/api/setting/swap/zram/delete') as Promise<ApiMsg>,
              )
            }
          >
            <Button danger icon={<DeleteOutlined />} disabled={busy}>
              {t('pages.settings.swap.delete')}
            </Button>
          </Popconfirm>
        ) : null,
    },
  ];

  const zramMissing = (status?.zram?.length ?? 0) === 0;
  const installInfo = status?.zramInstall;

  const overviewTab = (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Row gutter={[12, 12]}>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small">
            <Statistic
              title={t('pages.settings.swap.ramTotal')}
              value={gib(status?.ramTotalBytes ?? 0)}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small">
            <Statistic
              title={t('pages.settings.swap.ramAvailable')}
              value={gib(status?.ramAvailableBytes ?? 0)}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small">
            <Statistic title={t('pages.settings.swap.cpu')} value={status?.cpuCount ?? 0} />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card size="small">
            <Statistic title={t('pages.settings.swap.used')} value={mib(status?.usedBytes ?? 0)} />
          </Card>
        </Col>
      </Row>

      <Card size="small" title={t('pages.settings.swap.systemTitle')}>
        <Space direction="vertical" size={10} style={{ width: '100%' }}>
          <Space wrap align="end" className="swap-action-row">
            <div>
              <div>{t('pages.settings.swap.swappiness')}</div>
              <InputNumber
                min={0}
                max={200}
                value={swappiness}
                onChange={onNumber(setSwappiness)}
              />
            </div>
            <Button
              icon={<SaveOutlined />}
              type="primary"
              onClick={() =>
                void action(
                  () =>
                    HttpUtil.post('/panel/api/setting/swap/swappiness', {
                      value: swappiness,
                    }) as Promise<ApiMsg>,
                )
              }
              loading={busy}
            >
              {t('pages.settings.swap.apply')}
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => setSystemUpdateOpen(true)}
              disabled={busy}
            >
              {t('pages.settings.swap.systemUpdates')}
            </Button>
          </Space>
          {status?.recommendation && (
            <Alert
              type="info"
              showIcon
              title={status.recommendation.reason}
              description={
                <Space wrap>
                  <Tag>
                    {t('pages.settings.swap.recommendedSwap')}:{' '}
                    {status.recommendation.recommendedSwapMiB} MiB
                  </Tag>
                  <Tag>
                    {t('pages.settings.swap.recommendedZram')}:{' '}
                    {status.recommendation.recommendedZramMiB} MiB
                  </Tag>
                  <Tag>
                    {t('pages.settings.swap.recommendedSwappiness')}:{' '}
                    {status.recommendation.recommendedSwappiness}
                  </Tag>
                </Space>
              }
            />
          )}
        </Space>
      </Card>

      {installInfo && (
        <Card size="small" title={t('pages.settings.swap.packageInfoTitle')}>
          <Space direction="vertical" size={10} style={{ width: '100%' }}>
            <Alert
              type={installInfo.supported ? 'info' : 'error'}
              showIcon
              title={
                zramMissing
                  ? installInfo.supported
                    ? t('pages.settings.swap.zramMissing')
                    : t('pages.settings.swap.zramUnsupported')
                  : t('pages.settings.swap.zramTitle')
              }
              description={
                <Space wrap>
                  <Tag>
                    {t('pages.settings.swap.recommendedPackage')}:{' '}
                    {installInfo.recommendedPackage || installInfo.package}
                    {installInfo.recommendedVersion ? ' @ ' + installInfo.recommendedVersion : ''}
                  </Tag>
                  {installInfo.activeBackend && (
                    <Tag color="processing">
                      {t('pages.settings.swap.activeBackend')}: {installInfo.activeBackend}
                    </Tag>
                  )}
                  {installInfo.installedPackages.filter((item) => item.installed).length > 0 ? (
                    installInfo.installedPackages
                      .filter((item) => item.installed)
                      .map((item) => (
                        <Tag key={item.name}>
                          {item.name}
                          {item.version ? ' @ ' + item.version : ''}
                        </Tag>
                      ))
                  ) : (
                    <Tag>{t('pages.settings.swap.notInstalled')}</Tag>
                  )}
                </Space>
              }
            />
            {installInfo.activeBackend &&
              !['systemd-zram-generator', 'zram-config', 'zram-tools', 'zram-init'].includes(
                installInfo.activeBackend,
              ) && (
                <Alert type="warning" showIcon title={t('pages.settings.swap.backendConflict')} />
              )}
            <Space wrap>
              <Button
                type="primary"
                icon={<DownloadOutlined />}
                onClick={() => void installZram()}
                loading={busy}
                disabled={!installInfo.supported}
              >
                {zramMissing
                  ? installInfo.installed
                    ? t('pages.settings.swap.configureZram')
                    : t('pages.settings.swap.installZram')
                  : t('pages.settings.swap.configureZram')}
              </Button>
              <Button
                icon={<ReloadOutlined />}
                onClick={() => void reinstallZram()}
                loading={busy}
                disabled={!installInfo.supported || !installInfo.reinstallCommand}
              >
                {t('pages.settings.swap.reinstallZram')}
              </Button>
              {installInfo.configPath && <Tag>{installInfo.configPath}</Tag>}
            </Space>
          </Space>
        </Card>
      )}
    </Space>
  );

  const swapFileTab = (
    <Card size="small" title={t('pages.settings.swap.fileTitle')}>
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        {status?.swapFileConfig.enabled && (
          <Alert
            type="success"
            showIcon
            title={[
              status.swapFileConfig.path,
              mib(status.swapFileConfig.sizeBytes),
              'priority ' + status.swapFileConfig.priority,
            ].join(' · ')}
          />
        )}
        <Space wrap align="end">
          <div>
            <div>{t('pages.settings.swap.sizeMiB')}</div>
            <InputNumber min={16} max={65536} value={swapSize} onChange={onNumber(setSwapSize)} />
          </div>
          <div>
            <div>{t('pages.settings.swap.priority')}</div>
            <InputNumber
              min={0}
              max={32767}
              value={swapPriority}
              onChange={onNumber(setSwapPriority)}
            />
          </div>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() =>
              void action(
                () =>
                  HttpUtil.post('/panel/api/setting/swap/create', {
                    sizeMiB: swapSize,
                    priority: swapPriority,
                  }) as Promise<ApiMsg>,
              )
            }
            loading={busy}
            disabled={status?.swapFileConfig.enabled}
          >
            {t('pages.settings.swap.createSwap')}
          </Button>
          {status?.swapFileConfig.enabled && (
            <Popconfirm
              title={t('pages.settings.swap.deleteConfirm')}
              onConfirm={() =>
                void action(
                  () => HttpUtil.post('/panel/api/setting/swap/delete') as Promise<ApiMsg>,
                )
              }
            >
              <Button danger icon={<DeleteOutlined />} disabled={busy}>
                {t('pages.settings.swap.delete')}
              </Button>
            </Popconfirm>
          )}
        </Space>
      </Space>
    </Card>
  );

  const zramTab = (
    <Card size="small" title={t('pages.settings.swap.zramTitle')}>
      <Space direction="vertical" size={12} style={{ width: '100%' }}>
        <Space wrap align="end">
          <div>
            <div>{t('pages.settings.swap.sizeMiB')}</div>
            <InputNumber min={16} max={65536} value={zramSize} onChange={onNumber(setZramSize)} />
          </div>
          <div>
            <div>{t('pages.settings.swap.algorithm')}</div>
            <Select
              style={{ minWidth: 180 }}
              value={zramAlgorithm || undefined}
              onChange={setZramAlgorithm}
              options={algorithmOptions}
              placeholder={t('pages.settings.swap.algorithmPlaceholder')}
            />
          </div>
          <div>
            <div>{t('pages.settings.swap.streams')}</div>
            <InputNumber min={1} max={64} value={zramStreams} onChange={onNumber(setZramStreams)} />
          </div>
          <div>
            <div>{t('pages.settings.swap.memoryLimitMiB')}</div>
            <InputNumber min={0} max={65536} value={zramLimit} onChange={onNumber(setZramLimit)} />
          </div>
          <div>
            <div>{t('pages.settings.swap.priority')}</div>
            <InputNumber
              min={0}
              max={32767}
              value={zramPriority}
              onChange={onNumber(setZramPriority)}
            />
          </div>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={() =>
              void action(
                () =>
                  HttpUtil.post('/panel/api/setting/swap/zram/create', {
                    sizeMiB: zramSize,
                    algorithm: zramAlgorithm,
                    streams: zramStreams,
                    memoryLimitMiB: zramLimit,
                    priority: zramPriority,
                  }) as Promise<ApiMsg>,
              )
            }
            loading={busy}
            disabled={status?.zramConfig.enabled || installInfo?.usingGenerator}
          >
            {t('pages.settings.swap.createZram')}
          </Button>
          {status?.zramConfig.enabled && (
            <Popconfirm
              title={t('pages.settings.swap.deleteZramConfirm')}
              onConfirm={() =>
                void action(
                  () => HttpUtil.post('/panel/api/setting/swap/zram/delete') as Promise<ApiMsg>,
                )
              }
            >
              <Button danger icon={<DeleteOutlined />} disabled={busy}>
                {t('pages.settings.swap.delete')}
              </Button>
            </Popconfirm>
          )}
        </Space>
        {installInfo?.usingGenerator && (
          <Alert
            type="info"
            showIcon
            title={t('pages.settings.swap.generatorManaged')}
            description={installInfo.configPath}
          />
        )}
        <Table
          size="small"
          rowKey="device"
          pagination={false}
          scroll={{ x: isMobile ? 760 : undefined, y: 360 }}
          columns={zramColumns}
          dataSource={status?.zram ?? []}
        />
      </Space>
    </Card>
  );

  const areasTab = (
    <Card size="small" title={t('pages.settings.swap.allAreas')}>
      <Table
        rowKey="path"
        pagination={false}
        size="small"
        scroll={{ x: isMobile ? 700 : undefined, y: 300 }}
        dataSource={status?.areas ?? []}
        columns={[
          { title: t('pages.settings.swap.device'), dataIndex: 'path', key: 'path' },
          { title: t('pages.settings.swap.type'), dataIndex: 'type', key: 'type' },
          {
            title: t('pages.settings.swap.size'),
            key: 'size',
            render: (_: unknown, row: SwapArea) => mib(row.sizeBytes),
          },
          {
            title: t('pages.settings.swap.used'),
            key: 'used',
            render: (_: unknown, row: SwapArea) => mib(row.usedBytes),
          },
          { title: t('pages.settings.swap.priority'), dataIndex: 'priority', key: 'priority' },
          {
            title: t('pages.settings.swap.status'),
            key: 'status',
            render: (_: unknown, row: SwapArea) => (
              <Space>
                <Tag color={row.active ? 'success' : 'default'}>
                  {row.active ? t('pages.settings.swap.active') : t('pages.settings.swap.inactive')}
                </Tag>
                {row.managed && <Tag color="blue">{t('pages.settings.swap.managed')}</Tag>}
              </Space>
            ),
          },
        ]}
      />
    </Card>
  );

  if (loadError && !status) {
    return (
      <>
        {contextHolder}
        <Result
          status="error"
          title={t('somethingWentWrong')}
          subTitle={loadError}
          extra={
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void refresh()}>
              {t('pages.settings.swap.refresh')}
            </Button>
          }
        />
      </>
    );
  }

  return (
    <>
      {contextHolder}
      <SystemUpdateModal open={systemUpdateOpen} onClose={() => setSystemUpdateOpen(false)} />
      {loadError && (
        <Alert type="warning" showIcon closable style={{ marginBottom: 12 }} title={loadError} />
      )}
      {isMobile && (
        <div className="swap-mobile-toolbar">
          <Tag color="processing">
            {t('pages.settings.swap.areasCount', { count: status?.areas.length ?? 0 })}
          </Tag>
          {installInfo?.distribution && (
            <Tag>
              {installInfo.distribution} {installInfo.version}
            </Tag>
          )}
          <Button
            size="small"
            icon={<ReloadOutlined />}
            onClick={() => void refresh()}
            loading={loading}
          >
            {t('pages.settings.swap.refresh')}
          </Button>
        </div>
      )}
      <Tabs
        className="swap-settings-tabs"
        items={[
          { key: 'overview', label: t('pages.settings.swap.overview'), children: overviewTab },
          { key: 'swap-file', label: t('pages.settings.swap.fileTitle'), children: swapFileTab },
          { key: 'zram', label: 'ZRAM', children: zramTab },
          { key: 'areas', label: t('pages.settings.swap.allAreas'), children: areasTab },
        ]}
        tabBarExtraContent={
          !isMobile
            ? {
                right: (
                  <Space wrap>
                    <Tag color="processing">
                      {t('pages.settings.swap.areasCount', { count: status?.areas.length ?? 0 })}
                    </Tag>
                    {installInfo?.distribution && (
                      <Tag>
                        {installInfo.distribution} {installInfo.version}
                      </Tag>
                    )}
                    <Button
                      size="small"
                      icon={<ReloadOutlined />}
                      onClick={() => void refresh()}
                      loading={loading}
                    >
                      {t('pages.settings.swap.refresh')}
                    </Button>
                  </Space>
                ),
              }
            : undefined
        }
      />
    </>
  );
}
