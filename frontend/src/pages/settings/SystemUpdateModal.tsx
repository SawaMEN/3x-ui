import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Modal,
  Pagination,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
  Tooltip,
  message,
} from 'antd';
import {
  CloudDownloadOutlined,
  DownloadOutlined,
  DeleteOutlined,
  PoweroffOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import './SystemUpdateModal.css';
import OlcRTCClientsModal from './OlcRTCClientsModal';

type SystemUpdatePackage = {
  name: string;
  installedVersion: string;
  availableVersion: string;
  installed: boolean;
  required: boolean;
  kernel: boolean;
  updateAvailable: boolean;
};

type DependencyKey = 'naiveproxy' | 'hysteria2' | 'sudoku' | 'pingtunnel' | 'trusttunnel' | 'olcrtc';

type DependencyStatus = {
  key: DependencyKey;
  label: string;
  installed: boolean;
  installedVersion: string;
  availableVersion: string;
  updateAvailable: boolean;
  prerelease?: boolean;
  source?: 'sing-box' | 'standalone';
  fallbackReason?: string;
  mixed?: boolean;
  error?: string;
  supported?: boolean;
  unsupportedReason?: string;
  busy?: boolean;
  operation?: string;
  running?: boolean;
};

type ExternalVPNBinaryStatus = {
  source?: string;
  fallbackReason?: string;
  installed?: boolean;
  version?: string;
  latestVersion?: string;
  updateAvailable?: boolean;
  error?: string;
  supported?: boolean;
  unsupportedReason?: string;
};

type ExternalVPNStatus = {
  hysteria2?: ExternalVPNBinaryStatus;
  pingtunnel?: ExternalVPNBinaryStatus;
  trusttunnel?: ExternalVPNBinaryStatus;
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
  allPackages: boolean;
  notes: string[];
};

type SystemUpdateResult = {
  updated: boolean;
  rebootRequired: boolean;
  output: string;
  error: string;
};

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };

const isDependencyInstallable = (dependency: DependencyStatus) =>
  dependency.source === 'standalone' ||
  dependency.key === 'sudoku' ||
  dependency.key === 'pingtunnel' ||
  dependency.key === 'trusttunnel' ||
  dependency.key === 'olcrtc';

const dependencyNeedsAction = (dependency: DependencyStatus) => {
  if (!dependency.availableVersion) return false;
  if (isDependencyInstallable(dependency)) {
    return !dependency.installed || dependency.updateAvailable;
  }
  return dependency.installed && dependency.updateAvailable;
};

function waitForUpdateRecovery(delayMs: number): Promise<void> {
  return new Promise((resolve) => {
    window.setTimeout(resolve, delayMs);
  });
}

function isTransientFetchFailure(message: string): boolean {
  return /failed to fetch|networkerror|network error|load failed/i.test(message);
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
      availableVersion: typeof kernel.availableVersion === 'string' ? kernel.availableVersion : '',
      packageNames: Array.isArray(kernel.packageNames)
        ? kernel.packageNames.filter((item): item is string => typeof item === 'string')
        : [],
      rebootRequired: kernel.rebootRequired === true,
    },
    updatesAvailable: raw.updatesAvailable === true,
    missingPackages: raw.missingPackages === true,
    canUpdate: raw.canUpdate === true,
    allPackages: raw.allPackages === true,
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

export default function SystemUpdateModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useMediaQuery();
  const [messageApi, contextHolder] = message.useMessage();
  const [systemUpdateBusy, setSystemUpdateBusy] = useState(false);
  const [systemUpdate, setSystemUpdate] = useState<SystemUpdateStatus | null>(null);
  const [systemUpdateResult, setSystemUpdateResult] = useState<SystemUpdateResult | null>(null);
  const [includeAllPackages, setIncludeAllPackages] = useState(false);
  const [packagePage, setPackagePage] = useState(1);
  const [bundledDependencies, setBundledDependencies] = useState<DependencyStatus[]>([]);
  const [standaloneDependencies, setStandaloneDependencies] = useState<DependencyStatus[]>([]);
  const dependencies = useMemo(
    () =>
      bundledDependencies.map((dependency) => {
        const standalone = standaloneDependencies.find((row) => row.key === dependency.key);
        if (standalone?.installed && standalone.source !== 'sing-box') return standalone;
        return standalone?.fallbackReason
          ? { ...dependency, fallbackReason: standalone.fallbackReason }
          : dependency;
      }),
    [bundledDependencies, standaloneDependencies],
  );
  const updatableDependencies = useMemo(
    () => [
      ...bundledDependencies.filter((row) => row.source !== 'sing-box'),
      ...standaloneDependencies,
    ],
    [bundledDependencies, standaloneDependencies],
  );
  const [dependencyBusy, setDependencyBusy] = useState<DependencyKey | null>(null);
  const [olcrtcSettingsOpen, setOlcrtcSettingsOpen] = useState(false);

  const loadDependencyUpdates = useCallback(async () => {
    const [core, sudoku, external, naive, olcrtc] = await Promise.all([
      HttpUtil.get('/panel/api/setting/singbox/status') as Promise<
        ApiMsg<{ installed?: boolean; version?: string }>
      >,
      HttpUtil.get('/panel/api/setting/sudoku/status') as Promise<ApiMsg<ExternalVPNBinaryStatus>>,
      HttpUtil.get('/panel/api/server/externalvpn/status') as Promise<ApiMsg<ExternalVPNStatus>>,
      HttpUtil.get('/panel/api/naiveproxy/status', undefined, { silent: true }) as Promise<
        ApiMsg<ExternalVPNBinaryStatus>
      >,
      HttpUtil.get('/panel/api/olcrtc/status', undefined, { silent: true }) as Promise<
        ApiMsg<ExternalVPNBinaryStatus & { busy?: boolean; operation?: string; running?: boolean }>
      >,
    ]);
    const bundled = ['naiveproxy', 'hysteria2'].map((key) => ({
      key: key as DependencyKey,
      label: key === 'naiveproxy' ? 'NaiveProxy' : 'Hysteria2',
      source: 'sing-box' as const,
      installed: core.success === true && core.obj?.installed === true,
      installedVersion: core.obj?.version || '',
      availableVersion: '',
      updateAvailable: false,
    }));
    const rows: DependencyStatus[] = [...bundled];
    for (const key of ['sudoku', 'pingtunnel', 'trusttunnel'] as const) {
      const data =
        key === 'sudoku'
          ? sudoku.success
            ? sudoku.obj
            : undefined
          : external.success
            ? external.obj?.[key]
            : undefined;
      rows.push({
        key,
        label: key === 'sudoku' ? 'Sudoku' : key === 'pingtunnel' ? 'Pingtunnel' : 'TrustTunnel',
        installed: data?.installed === true,
        installedVersion: data?.version || '',
        availableVersion: data?.latestVersion || '',
        updateAvailable: data?.updateAvailable === true,
        supported: data?.supported,
        unsupportedReason: data?.unsupportedReason,
        error:
          data?.error ||
          (key === 'sudoku'
            ? !sudoku.success
              ? sudoku.msg
              : undefined
            : !external.success
              ? external.msg
              : undefined),
      });
    }
    rows.push({
      key: 'olcrtc',
      label: 'OlcRTC',
      source: 'standalone',
      installed: olcrtc.obj?.installed === true,
      installedVersion: olcrtc.obj?.installedVersion || '',
      availableVersion: olcrtc.obj?.availableVersion || 'upstream',
      updateAvailable: olcrtc.obj?.updateAvailable === true,
      supported: olcrtc.obj?.supported,
      unsupportedReason: olcrtc.obj?.unsupportedReason,
      busy: olcrtc.obj?.busy === true,
      running: olcrtc.obj?.running === true,
      operation: olcrtc.obj?.operation,
      error: olcrtc.obj?.error || (olcrtc.success ? undefined : olcrtc.msg),
    });
    setBundledDependencies(rows);
    setStandaloneDependencies(
      (['naiveproxy', 'hysteria2'] as const).map((key) => {
        const response = key === 'naiveproxy' ? naive : external;
        const data = key === 'naiveproxy' ? naive.obj : external.obj?.hysteria2;
        return {
          key,
          label: key === 'naiveproxy' ? 'NaiveProxy' : 'Hysteria2',
          source:
            data?.installed === true && data.source === 'sing-box' ? 'sing-box' : 'standalone',
          fallbackReason: data?.fallbackReason,
          mixed: data?.source === 'mixed',
          supported: data?.supported,
          unsupportedReason: data?.unsupportedReason,
          installed: data?.installed === true,
          installedVersion: data?.version || '',
          availableVersion: data?.latestVersion || '',
          updateAvailable: data?.updateAvailable === true,
          error: data?.error || (response.success ? undefined : response.msg),
        };
      }),
    );
  }, []);

  const checkSystemUpdates = useCallback(async () => {
    setSystemUpdateBusy(true);
    try {
      const [msg] = await Promise.all([
        HttpUtil.post(
          `/panel/api/setting/system/update/check?allPackages=${includeAllPackages}`,
        ) as Promise<ApiMsg<unknown>>,
        loadDependencyUpdates(),
      ]);
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to check system updates');
      setSystemUpdate(normalizeSystemUpdate(msg.obj));
      setSystemUpdateResult(null);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
    }
  }, [includeAllPackages, loadDependencyUpdates, messageApi]);

  const requestDependencyUpdate = useCallback(async (dependency: DependencyStatus) => {
    switch (dependency.key) {
      case 'olcrtc':
        return (await HttpUtil.post(
          dependency.installed ? '/panel/api/olcrtc/update' : '/panel/api/olcrtc/install',
        )) as ApiMsg<unknown>;
      case 'naiveproxy':
        return (await HttpUtil.post('/panel/api/naiveproxy/update')) as ApiMsg<unknown>;
      case 'hysteria2':
        return (await HttpUtil.post(
          '/panel/api/server/externalvpn/update/hysteria2',
        )) as ApiMsg<unknown>;
      case 'sudoku':
        return (await HttpUtil.post('/panel/api/setting/sudoku/update')) as ApiMsg<unknown>;
      case 'pingtunnel':
      case 'trusttunnel':
        return (await HttpUtil.post(
          `/panel/api/server/externalvpn/update/${dependency.key}`,
        )) as ApiMsg<unknown>;
    }
  }, []);

  const updateDependency = async (dependency: DependencyStatus) => {
    if (!dependencyNeedsAction(dependency)) return;

    setDependencyBusy(dependency.key);
    try {
      const response = await requestDependencyUpdate(dependency);
      if (!response?.success) {
        throw new Error(response?.msg || `Не удалось обновить ${dependency.label}`);
      }

      messageApi.success(
        dependency.key === 'olcrtc'
          ? 'OlcRTC: операция запущена. Статус будет обновляться автоматически.'
          : `${dependency.label}: ${t('pages.settings.swap.updateDone')}`,
      );
      await loadDependencyUpdates();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setDependencyBusy(null);
    }
  };

  const removeDependency = async (dependency: DependencyStatus) => {
    setDependencyBusy(dependency.key);
    try {
      const response = (await HttpUtil.post(
        dependency.key === 'olcrtc'
          ? '/panel/api/olcrtc/remove'
          : `/panel/api/server/components/remove/${dependency.key}`,
      )) as ApiMsg<unknown>;
      if (!response?.success)
        throw new Error(response?.msg || `Не удалось удалить ${dependency.label}`);
      messageApi.success(
        dependency.key === 'olcrtc'
          ? 'OlcRTC: удаление запущено. Настройки клиентов сохранятся для повторной установки.'
          : `${dependency.label}: компонент удалён.`,
      );
      await loadDependencyUpdates();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setDependencyBusy(null);
    }
  };

  useEffect(() => {
    if (!open || !dependencies.some((dependency) => dependency.key === 'olcrtc' && dependency.busy)) return;
    const interval = window.setInterval(() => { void loadDependencyUpdates(); }, 3000);
    return () => window.clearInterval(interval);
  }, [dependencies, loadDependencyUpdates, open]);

  const recoverSystemUpdateStatus = useCallback(async (): Promise<SystemUpdateStatus | null> => {
    const delays = [0, 1000, 2000, 4000, 6000];

    for (const delay of delays) {
      if (delay > 0) {
        await waitForUpdateRecovery(delay);
      }

      const status = await HttpUtil.get(
        `/panel/api/setting/system/update/status?allPackages=${includeAllPackages}`,
        undefined,
        {
          silent: true,
          timeout: 15000,
        },
      );
      if (status.success) {
        const normalized = normalizeSystemUpdate(status.obj);
        setSystemUpdate(normalized);
        return normalized;
      }
    }

    return null;
  }, [includeAllPackages]);

  const applyAllUpdates = async () => {
    setSystemUpdateBusy(true);
    try {
      const systemMsg = (await HttpUtil.post(
        `/panel/api/setting/system/update/apply?allPackages=${includeAllPackages}`,
      )) as ApiMsg<unknown>;
      if (!systemMsg?.success) {
        const result = normalizeSystemUpdateResult(systemMsg?.obj);
        setSystemUpdateResult(result);

        const errorMessage =
          systemMsg?.msg || result.error || t('pages.settings.swap.updateFailed');
        if (isTransientFetchFailure(errorMessage)) {
          const recoveredStatus = await recoverSystemUpdateStatus();
          if (recoveredStatus !== null) {
            messageApi.warning(
              'Соединение с панелью восстановлено. Завершение системного обновления не подтверждено; повторите проверку состояния.',
            );
            await loadDependencyUpdates();
            return;
          }
        }

        throw new Error(errorMessage);
      }

      if (systemMsg?.success) {
        setSystemUpdateResult(normalizeSystemUpdateResult(systemMsg?.obj));
      }

      const pending = updatableDependencies.filter(
        (dependency) => dependency.installed && dependencyNeedsAction(dependency),
      );

      const failed: string[] = [];
      const updatedTargets = new Set<string>();
      for (const dependency of pending) {
        const target = dependency.key;
        if (updatedTargets.has(target)) {
          continue;
        }
        try {
          updatedTargets.add(target);
          setDependencyBusy(dependency.key);
          const response = await requestDependencyUpdate(dependency);
          if (!response?.success) {
            failed.push(dependency.label);
          }
        } catch {
          failed.push(dependency.label);
        } finally {
          setDependencyBusy(null);
        }
      }

      await recoverSystemUpdateStatus();
      await loadDependencyUpdates();

      if (failed.length) {
        messageApi.warning(
          t('pages.settings.swap.partialUpdate', { components: failed.join(', ') }),
        );
      } else {
        messageApi.success(t('pages.settings.swap.updateDone'));
      }
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
      setDependencyBusy(null);
    }
  };

  const rebootSystem = async () => {
    setSystemUpdateBusy(true);
    try {
      const response = (await HttpUtil.post('/panel/api/setting/system/update/reboot')) as ApiMsg<{
        rebooting?: boolean;
      }>;
      if (!response?.success) {
        throw new Error(response?.msg || t('pages.settings.swap.rebootFailed'));
      }

      messageApi.info(t('pages.settings.swap.rebootStarted'));
      onClose();
      window.setTimeout(() => window.location.reload(), 8000);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
    }
  };

  useEffect(() => {
    if (!open) return;
    const timer = window.setTimeout(() => {
      void checkSystemUpdates();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [checkSystemUpdates, open]);

  const systemUpdateRows = useMemo(
    () =>
      (systemUpdate?.packages ?? [])
        .filter((item) => systemUpdate?.allPackages || item.required || item.kernel)
        .map((item) => ({
          ...item,
          key: item.kernel ? 'kernel:' + item.name : 'package:' + item.name,
        }))
        .sort((a, b) => {
          if (a.updateAvailable !== b.updateAvailable) return a.updateAvailable ? -1 : 1;
          if (a.kernel !== b.kernel) return a.kernel ? -1 : 1;
          return a.name.localeCompare(b.name);
        }),
    [systemUpdate],
  );

  const rebootRequired =
    Boolean(systemUpdate?.kernel.rebootRequired) || Boolean(systemUpdateResult?.rebootRequired);
  const allPackagesMode = systemUpdate?.allPackages ?? includeAllPackages;
  const isArch = systemUpdate?.packageManager === 'pacman';
  const componentUpdatesAvailable = updatableDependencies.some(
    (dependency) => dependency.installed && dependencyNeedsAction(dependency),
  );
  const unstableComponentUpdateAvailable = dependencies.some(
    (dependency) => dependency.prerelease && dependency.updateAvailable,
  );

  const renderPackageStatus = (row: SystemUpdatePackage) => (
    <Space size={4} wrap>
      <Tag color={row.kernel ? 'geekblue' : 'blue'}>
        {row.kernel
          ? t('pages.settings.swap.kernelPackage')
          : row.required
            ? t('pages.settings.swap.dependency')
            : t('pages.settings.swap.otherPackage')}
      </Tag>
      {row.updateAvailable && (
        <Tag color="warning">{t('pages.settings.swap.updatesAvailable')}</Tag>
      )}
      {!row.installed && <Tag color="error">{t('pages.settings.swap.notInstalled')}</Tag>}
    </Space>
  );

  return (
    <>
      {contextHolder}
      <OlcRTCClientsModal open={olcrtcSettingsOpen} onClose={() => setOlcrtcSettingsOpen(false)} />
      <Modal
        rootClassName="system-update-modal"
        open={open}
        title={t('pages.settings.swap.systemUpdatesTitle')}
        width={isMobile ? 'calc(100vw - 16px)' : 980}
        centered={!isMobile}
        onCancel={() => {
          if (!systemUpdateBusy && !dependencyBusy) onClose();
        }}
        footer={
          <div className="system-update-actions">
            <Button
              block={isMobile}
              icon={<ReloadOutlined />}
              onClick={() => void checkSystemUpdates()}
              loading={systemUpdateBusy}
              disabled={dependencyBusy !== null}
            >
              {t('pages.settings.swap.checkUpdates')}
            </Button>
            {rebootRequired && (
              <Button
                danger
                block={isMobile}
                icon={<PoweroffOutlined />}
                onClick={() => void rebootSystem()}
                loading={systemUpdateBusy}
                disabled={!systemUpdate?.runningAsRoot || dependencyBusy !== null}
              >
                {t('pages.settings.swap.rebootSystem')}
              </Button>
            )}
            <Popconfirm
              title={t(
                allPackagesMode
                  ? 'pages.settings.swap.updateAllConfirm'
                  : 'pages.settings.swap.updateConfirm',
              )}
              description={
                unstableComponentUpdateAvailable
                  ? t('pages.settings.swap.unstableUpdateConfirm')
                  : undefined
              }
              onConfirm={() => void applyAllUpdates()}
              disabled={
                !systemUpdate?.supported ||
                !systemUpdate?.runningAsRoot ||
                systemUpdateBusy ||
                dependencyBusy !== null ||
                (!systemUpdate.canUpdate && !componentUpdatesAvailable)
              }
            >
              <Button
                type="primary"
                block={isMobile}
                icon={<CloudDownloadOutlined />}
                loading={systemUpdateBusy}
                disabled={
                  !systemUpdate?.supported ||
                  !systemUpdate?.runningAsRoot ||
                  dependencyBusy !== null ||
                  (!systemUpdate.canUpdate && !componentUpdatesAvailable)
                }
              >
                {t(
                  allPackagesMode
                    ? 'pages.settings.swap.updateAllNow'
                    : 'pages.settings.swap.updateNow',
                )}
              </Button>
            </Popconfirm>
          </div>
        }
      >
        {!systemUpdate ? (
          <Alert type="info" showIcon title={t('pages.settings.swap.checkUpdates')} />
        ) : (
          <div className="system-update-content">
            <Space wrap>
              <Switch
                checked={allPackagesMode}
                disabled={isArch || systemUpdateBusy || dependencyBusy !== null}
                onChange={(checked) => {
                  setIncludeAllPackages(checked);
                  setPackagePage(1);
                  setSystemUpdate(null);
                  setSystemUpdateResult(null);
                }}
              />
              <Typography.Text>{t('pages.settings.swap.allPackagesOption')}</Typography.Text>
            </Space>
            {isArch && (
              <Alert type="info" showIcon title={t('pages.settings.swap.archFullUpgradeNote')} />
            )}
            <Descriptions
              size="small"
              bordered
              column={{ xs: 1, sm: 2, lg: 3 }}
              className="system-update-overview"
            >
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

            {!systemUpdate.supported && (
              <Alert
                type="error"
                showIcon
                title={t('pages.settings.swap.unsupportedSystemUpdate')}
              />
            )}
            {!systemUpdate.runningAsRoot && (
              <Alert type="error" showIcon title={t('pages.settings.swap.notRoot')} />
            )}
            {!isArch && (
              <Alert type="info" showIcon title={t('pages.settings.swap.systemUpdateNote')} />
            )}
            {systemUpdate.missingPackages && (
              <Alert type="warning" showIcon title={t('pages.settings.swap.missingPackages')} />
            )}
            {systemUpdate.updatesAvailable || componentUpdatesAvailable ? (
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

            {rebootRequired && (
              <Alert
                type="warning"
                showIcon
                message={t('pages.settings.swap.rebootRequired')}
                description={t('pages.settings.swap.rebootAfterKernel')}
              />
            )}

            <Card
              size="small"
              title={t('pages.settings.swap.componentUpdatesTitle')}
              className="system-update-section"
            >
              <Typography.Paragraph type="secondary">
                Дополнительные компоненты устанавливаются по кнопке «Установить». Общее обновление
                обновляет только уже установленные компоненты.
              </Typography.Paragraph>
              <div className="system-component-grid">
                {dependencies.map((dependency) => {
                  const standalone = standaloneDependencies.find(
                    (row) => row.key === dependency.key,
                  );
                  const action = standalone || dependency;
                  const actionLabel =
                    !action.installed && isDependencyInstallable(action)
                      ? standalone
                        ? 'Установить отдельное ядро'
                        : 'Установить'
                      : action.updateAvailable
                        ? t('pages.settings.swap.updateComponent')
                        : t('pages.settings.swap.upToDate');
                  return (
                    <Card
                      key={dependency.key}
                      size="small"
                      className="system-component-card"
                      title={dependency.label}
                      extra={
                        dependency.updateAvailable ? (
                          <Tag color="warning">{t('pages.settings.swap.updatesAvailable')}</Tag>
                        ) : dependency.installed ? (
                          <Tag color="success">{t('pages.settings.swap.upToDate')}</Tag>
                        ) : (
                          <Tag>{t('pages.settings.swap.notInstalled')}</Tag>
                        )
                      }
                    >
                      {(dependency.key === 'naiveproxy' || dependency.key === 'hysteria2') && (
                        <>
                          <Typography.Text
                            type="secondary"
                            style={{ display: 'block', marginBottom: 10 }}
                          >
                            {dependency.mixed
                              ? 'Совместимые входящие подключения используют отдельное ядро; остальные — sing-box.'
                              : dependency.source === 'sing-box'
                                ? 'Используется встроенный sing-box. Установка отдельного ядра автоматически переключит входящие подключения на него.'
                                : 'Используется отдельное ядро. Панель управляет запуском и клиентами, маршрутизация проходит через sing-box.'}
                          </Typography.Text>
                        </>
                      )}
                      {dependency.fallbackReason && (
                        <Alert
                          type="warning"
                          showIcon
                          title="Для совместимости используется sing-box"
                          description={dependency.fallbackReason}
                          style={{ marginBottom: 10 }}
                        />
                      )}
                      {action.error && (
                        <Alert
                          type="warning"
                          showIcon
                          title={action.error}
                          style={{ marginBottom: 10 }}
                        />
                      )}
                      {dependency.key === 'olcrtc' && (
                        <Typography.Paragraph type="secondary">
                          Отдельный WebRTC-сервер через Jitsi, Телемост или WB Stream. Не заменяет sing-box.
                          {dependency.busy
                            ? ` Выполняется: ${dependency.operation === 'remove' ? 'удаление' : 'установка или обновление'}.`
                            : dependency.installed
                              ? dependency.running ? ' Служба работает.' : ' Служба остановлена.'
                              : ' Нажмите «Установить», чтобы добавить компонент.'}
                        </Typography.Paragraph>
                      )}
                      {dependency.key === 'pingtunnel' && (
                        <Typography.Text
                          type="secondary"
                          style={{ display: 'block', marginBottom: 10 }}
                        >
                          Pingtunnel обновляется отдельным официальным Linux-бинарником из GitHub
                          Releases.
                        </Typography.Text>
                      )}
                      {dependency.key === 'trusttunnel' && (
                        <Typography.Text
                          type="secondary"
                          style={{ display: 'block', marginBottom: 10 }}
                        >
                          TrustTunnel обновляется отдельным официальным endpoint-бинарником из
                          GitHub Releases.
                        </Typography.Text>
                      )}
                      <div className="system-component-meta">
                        <div>
                          <Typography.Text type="secondary">
                            {t('pages.settings.swap.installedVersion')}
                          </Typography.Text>
                          <Typography.Text strong>
                            {dependency.installed ? dependency.installedVersion || '—' : '—'}
                          </Typography.Text>
                        </div>
                        <div>
                          <Typography.Text type="secondary">
                            {t('pages.settings.swap.availableVersion')}
                          </Typography.Text>
                          <Typography.Text strong>
                            {dependency.availableVersion || '—'}
                          </Typography.Text>
                        </div>
                      </div>
                      {standalone && !standalone.installed && (
                        <Typography.Paragraph type="secondary">
                          Отдельное ядро: {standalone.availableVersion || '—'}
                        </Typography.Paragraph>
                      )}
                      <Space wrap>
                        {dependency.key === 'olcrtc' && dependency.installed && (
                          <Button size="small" onClick={() => setOlcrtcSettingsOpen(true)} disabled={!dependency.running || dependency.busy}>
                            Настроить клиентов
                          </Button>
                        )}
                        {action.supported === false ? (
                          <Alert
                            type="info"
                            showIcon
                            title="Установка недоступна"
                            description={
                              action.unsupportedReason ||
                              'Для этой архитектуры нет официального бинарника.'
                            }
                          />
                        ) : (
                          <Tooltip title={actionLabel}>
                            <Button
                              size="small"
                              aria-label={actionLabel}
                              icon={action.installed ? <ReloadOutlined /> : <DownloadOutlined />}
                              loading={dependencyBusy === dependency.key || action.busy === true}
                              disabled={
                                (!action.updateAvailable &&
                                  !(isDependencyInstallable(action) && !action.installed)) ||
                                !action.availableVersion ||
                                dependencyBusy !== null ||
                                action.busy === true ||
                                systemUpdateBusy
                              }
                              onClick={() => void updateDependency(action)}
                            />
                          </Tooltip>
                        )}
                        {action.installed && (
                          <Popconfirm
                            title={`Удалить ${action.label}?`}
                            description={
                              dependency.key === 'olcrtc'
                                ? 'Служба OlcRTC остановится, бинарники удалятся. Данные клиентов сохранятся.'
                                : standalone
                                  ? 'Входящие подключения переключатся на встроенный sing-box.'
                                  : 'Активные подключения этого протокола необходимо сначала отключить.'
                            }
                            okText={t('delete')}
                            cancelText={t('cancel')}
                            onConfirm={() => removeDependency(action)}
                          >
                            <Tooltip title={`Удалить ${action.label}`}>
                              <Button
                                size="small"
                                danger
                                aria-label={`Удалить ${action.label}`}
                                icon={<DeleteOutlined />}
                                disabled={dependencyBusy !== null || systemUpdateBusy || action.busy === true}
                              />
                            </Tooltip>
                          </Popconfirm>
                        )}
                      </Space>
                    </Card>
                  );
                })}
              </div>
            </Card>

            <Card
              size="small"
              title={t('pages.settings.swap.systemPackagesTitle')}
              className="system-update-section"
            >
              {isMobile ? (
                <div className="system-package-list">
                  {systemUpdateRows.slice((packagePage - 1) * 20, packagePage * 20).map((row) => (
                    <Card key={row.key} size="small" className="system-package-card">
                      <Typography.Text strong>{row.name}</Typography.Text>
                      <div className="system-package-meta">
                        <span>
                          {t('pages.settings.swap.installedVersion')}:&nbsp;
                          {row.installed
                            ? row.installedVersion || '—'
                            : t('pages.settings.swap.notInstalled')}
                        </span>
                        <span>
                          {t('pages.settings.swap.availableVersion')}:&nbsp;
                          {row.availableVersion || '—'}
                        </span>
                      </div>
                      {renderPackageStatus(row)}
                    </Card>
                  ))}
                  {systemUpdateRows.length > 20 && (
                    <Pagination
                      simple
                      current={packagePage}
                      total={systemUpdateRows.length}
                      pageSize={20}
                      onChange={setPackagePage}
                    />
                  )}
                </div>
              ) : (
                <Table
                  size="small"
                  pagination={systemUpdateRows.length > 20 ? { pageSize: 20 } : false}
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
                      render: (_: unknown, row: SystemUpdatePackage) => row.availableVersion || '—',
                    },
                    {
                      title: t('pages.settings.swap.status'),
                      key: 'status',
                      render: (_: unknown, row: SystemUpdatePackage) => renderPackageStatus(row),
                    },
                  ]}
                />
              )}
            </Card>

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

            {systemUpdate.notes.map((note) => (
              <Alert key={note} type="info" showIcon title={note} />
            ))}

            {systemUpdateResult?.output && (
              <Card size="small" title={t('pages.settings.swap.updateOutput')}>
                <Typography.Text code>
                  <pre className="system-update-output">{systemUpdateResult.output}</pre>
                </Typography.Text>
              </Card>
            )}
          </div>
        )}
      </Modal>
    </>
  );
}
