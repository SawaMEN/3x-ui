import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  InputNumber,
  Result,
  Modal,
  Descriptions,
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

type SystemUpdatePackage = {
  name: string;
  installedVersion: string;
  availableVersion: string;
  installed: boolean;
  required: boolean;
  kernel: boolean;
  updateAvailable: boolean;
};

type SystemUpdateStatus = {
  distribution: string;
  version: string;
  packageManager: string;
  supported: boolean;
  runningAsRoot: boolean;
  packages: SystemUpdatePackage[];
  kernel: {
    runningVersion: string;
    updateAvailable: boolean;
    availableVersion: string;
    packageNames: string[];
    rebootRequired: boolean;
  };
  updatesAvailable: boolean;
  missingPackages: boolean;
  canUpdate: boolean;
  notes: string[];
};

type SystemUpdateResult = {
  updated: boolean;
  rebootRequired: boolean;
  output: string;
  error: string;
};

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };

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

function normalizeSystemUpdate(value: unknown): SystemUpdateStatus {
  const raw =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  const kernel =
    raw.kernel && typeof raw.kernel === 'object' ? (raw.kernel as Record<string, unknown>) : {};
  const packages = Array.isArray(raw.packages) ? raw.packages : [];
  return {
    distribution: typeof raw.distribution === 'string' ? raw.distribution : '',
    version: typeof raw.version === 'string' ? raw.version : '',
    packageManager: typeof raw.packageManager === 'string' ? raw.packageManager : '',
    supported: raw.supported !== false,
    runningAsRoot: raw.runningAsRoot === true,
    packages: packages
      .filter((item) => item && typeof item === 'object')
      .map((item) => {
        const row = item as Record<string, unknown>;
        return {
          name: typeof row.name === 'string' ? row.name : '',
          installedVersion: typeof row.installedVersion === 'string' ? row.installedVersion : '',
          availableVersion: typeof row.availableVersion === 'string' ? row.availableVersion : '',
          installed: row.installed === true,
          required: row.required === true,
          kernel: row.kernel === true,
          updateAvailable: row.updateAvailable === true,
        };
      }),
    kernel: {
      runningVersion: typeof kernel.runningVersion === 'string' ? kernel.runningVersion : '',
      updateAvailable: kernel.updateAvailable === true,
      availableVersion:
        typeof kernel.availableVersion === 'string' ? kernel.availableVersion : '',
      packageNames: Array.isArray(kernel.packageNames)
        ? kernel.packageNames.filter((item): item is string => typeof item === 'string')
        : [],
      rebootRequired: kernel.rebootRequired === true,
    },
    updatesAvailable: raw.updatesAvailable === true,
    missingPackages: raw.missingPackages === true,
    canUpdate: raw.canUpdate === true,
    notes: Array.isArray(raw.notes)
      ? raw.notes.filter((item): item is string => typeof item === 'string')
      : [],
  };
}

function normalizeSystemUpdateResult(value: unknown): SystemUpdateResult {
  const raw =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : {};
  return {
    updated: raw.updated === true,
    rebootRequired: raw.rebootRequired === true,
    output: typeof raw.output === 'string' ? raw.output : '',
    error: typeof raw.error === 'string' ? raw.error : '',
  };
}

export default function SwapSettingsTab() {
  const { t } = useTranslation();
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
  const [systemUpdateBusy, setSystemUpdateBusy] = useState(false);
  const [systemUpdate, setSystemUpdate] = useState<SystemUpdateStatus | null>(null);
  const [systemUpdateResult, setSystemUpdateResult] = useState<SystemUpdateResult | null>(null);

  const refreshSystemUpdateCapability = useCallback(async () => {
    try {
      const msg = (await HttpUtil.get(
        '/panel/api/setting/system/update/status',
      )) as ApiMsg<unknown>;
      if (!msg?.success) {
        setSystemUpdate(null);
        return;
      }
      setSystemUpdate(normalizeSystemUpdate(msg.obj));
    } catch {
      setSystemUpdate(null);
    }
  }, []);

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
    const initial = window.setTimeout(() => {
      void refresh();
      void refreshSystemUpdateCapability();
    }, 0);
    const timer = window.setInterval(() => void refresh(), 4000);
    const systemTimer = window.setInterval(() => void refreshSystemUpdateCapability(), 30000);
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(timer);
      window.clearInterval(systemTimer);
    };
  }, [refresh, refreshSystemUpdateCapability]);

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

  const checkSystemUpdates = async () => {
    setSystemUpdateBusy(true);
    try {
      const msg = (await HttpUtil.post(
        '/panel/api/setting/system/update/check',
      )) as ApiMsg<unknown>;
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to check system updates');
      const next = normalizeSystemUpdate(msg.obj);
      setSystemUpdate(next);
      setSystemUpdateResult(null);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
    }
  };

  const openSystemUpdate = () => {
    if (systemUpdate?.supported !== true) return;
    setSystemUpdateOpen(true);
    void checkSystemUpdates();
  };

  const applySystemUpdates = async () => {
    setSystemUpdateBusy(true);
    try {
      const msg = (await HttpUtil.post(
        '/panel/api/setting/system/update/apply',
      )) as ApiMsg<unknown>;
      if (!msg?.success) {
        const result = normalizeSystemUpdateResult(msg.obj);
        setSystemUpdateResult(result);
        throw new Error(msg?.msg || result.error || 'System update failed');
      }
      const result = normalizeSystemUpdateResult(msg.obj);
      setSystemUpdateResult(result);
      await checkSystemUpdates();
      await refresh();
      messageApi.success(t('pages.settings.swap.updateDone'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
    }
  };

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
          <Space wrap align="end">
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
            <Button icon={<ReloadOutlined />} onClick={openSystemUpdate} disabled={busy}>
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
              installInfo.activeBackend !== 'systemd-zram-generator' &&
              installInfo.activeBackend !== 'zram-init' && (
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
                  ? installInfo.recommendedInstalled
                    ? t('pages.settings.swap.enableZram')
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
          scroll={{ y: 360 }}
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
        scroll={{ y: 300 }}
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

  const systemUpdatePackages = systemUpdate?.packages ?? [];
  const systemUpdateRows = systemUpdatePackages.map((item) => ({
    ...item,
    key: item.kernel ? 'kernel:' + item.name : 'package:' + item.name,
  }));

  const systemUpdateModal = (
    <Modal
      open={systemUpdateOpen}
      title={t('pages.settings.swap.systemUpdatesTitle')}
      width={900}
      onCancel={() => {
        if (!systemUpdateBusy) setSystemUpdateOpen(false);
      }}
      footer={
        <Space>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => void checkSystemUpdates()}
            loading={systemUpdateBusy}
          >
            {t('pages.settings.swap.checkUpdates')}
          </Button>
          <Popconfirm
            title={t('pages.settings.swap.updateConfirm')}
            onConfirm={() => void applySystemUpdates()}
            disabled={
              !systemUpdate?.canUpdate ||
              systemUpdateBusy ||
              !systemUpdate?.supported ||
              !systemUpdate?.runningAsRoot
            }
          >
            <Button
              type="primary"
              icon={<DownloadOutlined />}
              loading={systemUpdateBusy}
              disabled={!systemUpdate?.canUpdate || !systemUpdate?.supported || !systemUpdate?.runningAsRoot}
            >
              {t('pages.settings.swap.updateNow')}
            </Button>
          </Popconfirm>
        </Space>
      }
    >
      {!systemUpdate ? (
        <Alert type="info" showIcon title={t('pages.settings.swap.checkUpdates')} />
      ) : (
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Descriptions size="small" bordered column={{ xs: 1, sm: 2, md: 3 }}>
            <Descriptions.Item label={t('pages.settings.swap.distro')}>
              {systemUpdate.distribution} {systemUpdate.version}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.settings.swap.packageManager')}>
              {systemUpdate.packageManager}
            </Descriptions.Item>
            <Descriptions.Item label={t('pages.settings.swap.currentKernel')}>
              {systemUpdate.kernel.runningVersion || '—'}
            </Descriptions.Item>
          </Descriptions>

          {!systemUpdate.runningAsRoot && (
            <Alert type="error" showIcon title={t('pages.settings.swap.notRoot')} />
          )}
          {systemUpdate.missingPackages && (
            <Alert type="warning" showIcon title={t('pages.settings.swap.missingPackages')} />
          )}
          {systemUpdate.updatesAvailable ? (
            <Alert
              type="warning"
              showIcon
              title={t('pages.settings.swap.updatesAvailable')}
              description={
                systemUpdate.kernel.updateAvailable
                  ? t('pages.settings.swap.rebootAfterKernel')
                  : undefined
              }
            />
          ) : (
            !systemUpdate.missingPackages && (
              <Alert type="success" showIcon title={t('pages.settings.swap.noUpdates')} />
            )
          )}
          {systemUpdate.kernel.updateAvailable && (
            <Alert
              type="warning"
              showIcon
              title={
                t('pages.settings.swap.availableKernel') +
                ': ' +
                (systemUpdate.kernel.availableVersion ||
                  systemUpdate.kernel.packageNames.join(', '))
              }
            />
          )}
          {systemUpdate.kernel.rebootRequired && (
            <Alert type="warning" showIcon title={t('pages.settings.swap.rebootRequired')} />
          )}

          <Table
            size="small"
            pagination={false}
            scroll={{ y: 360 }}
            rowKey="key"
            dataSource={systemUpdateRows}
            columns={[
              {
                title: t('pages.settings.swap.device'),
                dataIndex: 'name',
                key: 'name',
              },
              {
                title: t('pages.settings.swap.installedVersion'),
                key: 'installedVersion',
                render: (_: unknown, row: SystemUpdatePackage) =>
                  row.installed
                    ? row.installedVersion || '—'
                    : t('pages.settings.swap.notInstalled'),
              },
              {
                title: t('pages.settings.swap.availableVersion'),
                key: 'availableVersion',
                render: (_: unknown, row: SystemUpdatePackage) =>
                  row.updateAvailable ? row.availableVersion || '—' : '—',
              },
              {
                title: t('pages.settings.swap.status'),
                key: 'status',
                render: (_: unknown, row: SystemUpdatePackage) => (
                  <Space wrap>
                    <Tag color={row.kernel ? 'geekblue' : 'blue'}>
                      {row.kernel
                        ? t('pages.settings.swap.kernelPackage')
                        : t('pages.settings.swap.dependency')}
                    </Tag>
                    {row.updateAvailable && (
                      <Tag color="warning">{t('pages.settings.swap.updatesAvailable')}</Tag>
                    )}
                    {!row.installed && (
                      <Tag color="error">{t('pages.settings.swap.notInstalled')}</Tag>
                    )}
                  </Space>
                ),
              },
            ]}
          />

          {systemUpdate.notes.map((note) => (
            <Alert key={note} type="info" showIcon title={note} />
          ))}

          {systemUpdateResult?.output && (
            <Card size="small" title={t('pages.settings.swap.updateOutput')}>
              <pre style={{ maxHeight: 260, overflow: 'auto', margin: 0, whiteSpace: 'pre-wrap' }}>
                {systemUpdateResult.output}
              </pre>
            </Card>
          )}
          {systemUpdateResult?.rebootRequired && (
            <Alert type="warning" showIcon title={t('pages.settings.swap.rebootAfterKernel')} />
          )}
        </Space>
      )}
    </Modal>
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
      {systemUpdateModal}
      {loadError && (
        <Alert type="warning" showIcon closable style={{ marginBottom: 12 }} title={loadError} />
      )}
      <Tabs
        className="swap-settings-tabs"
        items={[
          { key: 'overview', label: t('pages.settings.swap.overview'), children: overviewTab },
          { key: 'swap-file', label: t('pages.settings.swap.fileTitle'), children: swapFileTab },
          { key: 'zram', label: t('pages.settings.swap.zramTitle'), children: zramTab },
          { key: 'areas', label: t('pages.settings.swap.allAreas'), children: areasTab },
        ]}
        tabBarExtraContent={{
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
        }}
      />
    </>
  );
}
