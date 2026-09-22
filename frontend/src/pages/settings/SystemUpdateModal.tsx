import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CloudDownloadOutlined,
  DownloadOutlined,
  PoweroffOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import './SystemUpdateModal.css';

type SystemUpdatePackage = {
  name: string;
  installedVersion: string;
  availableVersion: string;
  installed: boolean;
  required: boolean;
  kernel: boolean;
  updateAvailable: boolean;
};

type DependencyKey = 'xray' | 'sing-box' | 'telemt';

type DependencyStatus = {
  key: DependencyKey;
  label: string;
  installed: boolean;
  installedVersion: string;
  availableVersion: string;
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

const normalizeVersion = (value: string) => value.trim().replace(/^v/i, '');

const versionsDiffer = (installed: string, available: string) =>
  normalizeVersion(installed) !== normalizeVersion(available);

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
  const [dependencies, setDependencies] = useState<DependencyStatus[]>([]);
  const [dependencyBusy, setDependencyBusy] = useState<DependencyKey | null>(null);

  const loadDependencyUpdates = useCallback(async () => {
    const safeGet = async <T,>(url: string): Promise<ApiMsg<T>> => {
      try {
        return (await HttpUtil.get<T>(url)) as ApiMsg<T>;
      } catch {
        return { success: false };
      }
    };

    const [serverStatus, xrayVersions, singBoxStatus, singBoxVersions, telemtStatus] =
      await Promise.all([
        safeGet<{ xray?: { version?: string } }>('/panel/api/server/status'),
        safeGet<string[]>('/panel/api/server/getXrayVersion'),
        safeGet<{ installed?: boolean; version?: string }>('/panel/api/setting/singbox/status'),
        safeGet<string[]>('/panel/api/setting/singbox/versions'),
        safeGet<{
          installed?: boolean;
          version?: string;
          latestVersion?: string;
          updateAvailable?: boolean;
        }>('/panel/api/telemt/status'),
      ]);

    const xrayCurrent = serverStatus.success ? serverStatus.obj?.xray?.version || '' : '';
    const xrayLatest =
      xrayVersions.success && Array.isArray(xrayVersions.obj) ? xrayVersions.obj[0] || '' : '';
    const singBoxCurrent = singBoxStatus.success ? singBoxStatus.obj?.version || '' : '';
    const singBoxInstalled = singBoxStatus.success && singBoxStatus.obj?.installed === true;
    const singBoxLatest =
      singBoxVersions.success && Array.isArray(singBoxVersions.obj)
        ? singBoxVersions.obj[0] || ''
        : '';
    const telemtCurrent = telemtStatus.success ? telemtStatus.obj?.version || '' : '';
    const telemtLatest = telemtStatus.success ? telemtStatus.obj?.latestVersion || '' : '';

    setDependencies([
      {
        key: 'xray',
        label: 'Xray',
        installed: Boolean(xrayCurrent),
        installedVersion: xrayCurrent,
        availableVersion: xrayLatest,
        updateAvailable: Boolean(xrayCurrent && xrayLatest && versionsDiffer(xrayCurrent, xrayLatest)),
      },
      {
        key: 'sing-box',
        label: 'sing-box',
        installed: singBoxInstalled,
        installedVersion: singBoxCurrent,
        availableVersion: singBoxLatest,
        updateAvailable: Boolean(
          singBoxInstalled && singBoxCurrent && singBoxLatest && versionsDiffer(singBoxCurrent, singBoxLatest),
        ),
      },
      {
        key: 'telemt',
        label: 'Telemt',
        installed: telemtStatus.success && telemtStatus.obj?.installed === true,
        installedVersion: telemtCurrent,
        availableVersion: telemtLatest,
        updateAvailable:
          telemtStatus.success && telemtStatus.obj?.updateAvailable === true
            ? true
            : Boolean(telemtCurrent && telemtLatest && versionsDiffer(telemtCurrent, telemtLatest)),
      },
    ]);
  }, []);

  const checkSystemUpdates = useCallback(async () => {
    setSystemUpdateBusy(true);
    try {
      const [msg] = await Promise.all([
        HttpUtil.post('/panel/api/setting/system/update/check') as Promise<ApiMsg<unknown>>,
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
  }, [loadDependencyUpdates, messageApi]);

  const updateDependency = async (dependency: DependencyStatus) => {
    if (!dependency.installed || !dependency.availableVersion) return;

    setDependencyBusy(dependency.key);
    try {
      let response: ApiMsg<unknown>;
      switch (dependency.key) {
        case 'xray':
          response = (await HttpUtil.post(
            `/panel/api/server/installXray/${encodeURIComponent(dependency.availableVersion)}`,
          )) as ApiMsg<unknown>;
          break;
        case 'sing-box':
          response = (await HttpUtil.post(
            `/panel/api/setting/singbox/install/${encodeURIComponent(dependency.availableVersion)}`,
          )) as ApiMsg<unknown>;
          break;
        case 'telemt':
          response = (await HttpUtil.post('/panel/api/telemt/action', {
            action: 'update',
          })) as ApiMsg<unknown>;
          break;
      }

      if (!response?.success) {
        throw new Error(response?.msg || `Не удалось обновить ${dependency.label}`);
      }

      messageApi.success(`${dependency.label}: ${t('pages.settings.swap.updateDone')}`);
      await loadDependencyUpdates();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setDependencyBusy(null);
    }
  };

  const applyAllUpdates = async () => {
    setSystemUpdateBusy(true);
    try {
      const systemMsg = (await HttpUtil.post(
        '/panel/api/setting/system/update/apply',
      )) as ApiMsg<unknown>;
      if (!systemMsg?.success) {
        const result = normalizeSystemUpdateResult(systemMsg.obj);
        setSystemUpdateResult(result);
        throw new Error(systemMsg?.msg || result.error || t('pages.settings.swap.updateFailed'));
      }

      setSystemUpdateResult(normalizeSystemUpdateResult(systemMsg.obj));

      const pending = dependencies.filter(
        (dependency) =>
          dependency.installed &&
          dependency.updateAvailable &&
          Boolean(dependency.availableVersion),
      );

      const failed: string[] = [];
      for (const dependency of pending) {
        try {
          setDependencyBusy(dependency.key);
          let response: ApiMsg<unknown>;
          if (dependency.key === 'xray') {
            response = (await HttpUtil.post(
              `/panel/api/server/installXray/${encodeURIComponent(dependency.availableVersion)}`,
            )) as ApiMsg<unknown>;
          } else if (dependency.key === 'sing-box') {
            response = (await HttpUtil.post(
              `/panel/api/setting/singbox/install/${encodeURIComponent(dependency.availableVersion)}`,
            )) as ApiMsg<unknown>;
          } else {
            response = (await HttpUtil.post('/panel/api/telemt/action', {
              action: 'update',
            })) as ApiMsg<unknown>;
          }
          if (!response?.success) {
            failed.push(dependency.label);
          }
        } catch {
          failed.push(dependency.label);
        } finally {
          setDependencyBusy(null);
        }
      }

      const refreshed = (await HttpUtil.post(
        '/panel/api/setting/system/update/check',
      )) as ApiMsg<unknown>;
      if (refreshed?.success) {
        setSystemUpdate(normalizeSystemUpdate(refreshed.obj));
      }
      await loadDependencyUpdates();

      if (failed.length) {
        messageApi.warning(t('pages.settings.swap.partialUpdate', { components: failed.join(', ') }));
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
      const response = (await HttpUtil.post(
        '/panel/api/setting/system/update/reboot',
      )) as ApiMsg<{ rebooting?: boolean }>;
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
    void checkSystemUpdates();
  }, [checkSystemUpdates, open]);

  const systemUpdateRows = useMemo(
    () =>
      (systemUpdate?.packages ?? []).map((item) => ({
        ...item,
        key: item.kernel ? 'kernel:' + item.name : 'package:' + item.name,
      })),
    [systemUpdate],
  );

  const rebootRequired =
    Boolean(systemUpdate?.kernel.rebootRequired) || Boolean(systemUpdateResult?.rebootRequired);
  const componentUpdatesAvailable = dependencies.some((dependency) => dependency.updateAvailable);

  const renderPackageStatus = (row: SystemUpdatePackage) => (
    <Space size={4} wrap>
      <Tag color={row.kernel ? 'geekblue' : 'blue'}>
        {row.kernel ? t('pages.settings.swap.kernelPackage') : t('pages.settings.swap.dependency')}
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
                disabled={
                  !systemUpdate?.runningAsRoot || dependencyBusy !== null
                }
              >
                {t('pages.settings.swap.rebootSystem')}
              </Button>
            )}
            <Popconfirm
              title={t('pages.settings.swap.updateConfirm')}
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
                {t('pages.settings.swap.updateNow')}
              </Button>
            </Popconfirm>
          </div>
        }
      >
        {!systemUpdate ? (
          <Alert type="info" showIcon title={t('pages.settings.swap.checkUpdates')} />
        ) : (
          <div className="system-update-content">
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
              <div className="system-component-grid">
                {dependencies.map((dependency) => (
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
                    <Button
                      size="small"
                      icon={<DownloadOutlined />}
                      loading={dependencyBusy === dependency.key}
                      disabled={!dependency.updateAvailable || dependencyBusy !== null || systemUpdateBusy}
                      onClick={() => void updateDependency(dependency)}
                    >
                      {dependency.updateAvailable
                        ? t('pages.settings.swap.updateComponent')
                        : t('pages.settings.swap.upToDate')}
                    </Button>
                  </Card>
                ))}
              </div>
            </Card>

            <Card
              size="small"
              title={t('pages.settings.swap.systemPackagesTitle')}
              className="system-update-section"
            >
              {isMobile ? (
                <div className="system-package-list">
                  {systemUpdateRows.map((row) => (
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
                          {row.updateAvailable ? row.availableVersion || '—' : '—'}
                        </span>
                      </div>
                      {renderPackageStatus(row)}
                    </Card>
                  ))}
                </div>
              ) : (
                <Table
                  size="small"
                  pagination={false}
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
