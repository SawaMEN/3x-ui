import { useCallback, useEffect, useState } from 'react';
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
  message,
} from 'antd';
import { DownloadOutlined, ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useMediaQuery } from '@/hooks/useMediaQuery';

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

  const checkSystemUpdates = useCallback(async () => {
    setSystemUpdateBusy(true);
    try {
      const msg = (await HttpUtil.post(
        '/panel/api/setting/system/update/check',
      )) as ApiMsg<unknown>;
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to check system updates');
      setSystemUpdate(normalizeSystemUpdate(msg.obj));
      setSystemUpdateResult(null);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSystemUpdateBusy(false);
    }
  }, [messageApi]);

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
      setSystemUpdateResult(normalizeSystemUpdateResult(msg.obj));
      await checkSystemUpdates();
      messageApi.success(t('pages.settings.swap.updateDone'));
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

  const systemUpdateRows = (systemUpdate?.packages ?? []).map((item) => ({
    ...item,
    key: item.kernel ? 'kernel:' + item.name : 'package:' + item.name,
  }));

  return (
    <>
      {contextHolder}
      <Modal
        open={open}
        title={t('pages.settings.swap.systemUpdatesTitle')}
        width={isMobile ? 'calc(100vw - 24px)' : 900}
        onCancel={() => {
          if (!systemUpdateBusy) onClose();
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
                disabled={
                  !systemUpdate?.canUpdate ||
                  !systemUpdate?.supported ||
                  !systemUpdate?.runningAsRoot
                }
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

            {!systemUpdate.supported && (
              <Alert
                type="error"
                showIcon
                title="Обновление системы не поддерживается для этой ОС."
              />
            )}
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
              scroll={{ x: isMobile ? 760 : undefined, y: 360 }}
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
    </>
  );
}
