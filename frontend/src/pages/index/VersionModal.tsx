import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Collapse, Modal, Radio, Segmented, Space, Spin, Tag, Tooltip } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { useQueryClient } from '@tanstack/react-query';
import { activateOnKey } from '@/utils/a11y';
import type { Status } from '@/models/status';
import GeodataSection from './GeodataSection';
import './VersionModal.css';

interface BusyEvent {
  busy: boolean;
  tip?: string;
}

interface VersionModalProps {
  open: boolean;
  status: Status;
  onClose: () => void;
  onBusy: (e: BusyEvent) => void;
}

const GEOFILES = [
  'geosite.dat',
  'geoip.dat',
  'geosite_IR.dat',
  'geoip_IR.dat',
  'geosite_RU.dat',
  'geoip_RU.dat',
];

export default function VersionModal({ open, onClose, onBusy }: VersionModalProps) {
  const { t } = useTranslation();
  const { allSetting } = useAllSettings();
  const queryClient = useQueryClient();
  const coreType = allSetting.coreType;
  const coreName =
    coreType === 'xray'
      ? 'Xray'
      : coreType === 'sing-box'
        ? 'sing-box (ядро панели)'
        : 'sing-box (официальное)';
  const [versions, setVersions] = useState<{ tag: string; prerelease: boolean }[]>([]);
  const [releaseChannel, setReleaseChannel] = useState<'stable' | 'beta'>('stable');
  const [versionsError, setVersionsError] = useState('');
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState('');
  const [modal, modalContextHolder] = Modal.useModal();
  const [activeKey, setActiveKey] = useState<string | string[]>('1');
  const [singBoxVersion, setSingBoxVersion] = useState('');
  const [loading, setLoading] = useState(false);

  const normalizeVersion = (value: string) =>
    value.match(/v?([0-9]+(?:[.-][a-z0-9]+)*)/i)?.[1]?.toLowerCase() || '';

  const visibleVersions = versions.filter((version) =>
    releaseChannel === 'beta' ? version.prerelease : !version.prerelease,
  );

  const fetchVersions = useCallback(async () => {
    setLoading(true);
    setVersionsError('');
    setVersions([]);
    try {
      const [status, releases] = await Promise.all([
        HttpUtil.get<{ cores: { type: string; version: string }[] }>('/panel/api/setting/cores'),
        coreType === 'sing-box'
          ? Promise.resolve(null)
          : HttpUtil.get<{ tag: string; prerelease: boolean }[]>(
              `/panel/api/setting/cores/${coreType}/versions`,
              undefined,
              { silent: true },
            ),
      ]);
      setSingBoxVersion(status.obj?.cores.find((core) => core.type === coreType)?.version || '');
      if (releases) {
        if (!releases.success) setVersionsError(releases.msg || 'Не удалось загрузить версии');
        else setVersions(releases.obj || []);
      }
    } catch (error) {
      setVersionsError(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  }, [coreType]);

  const [wasOpen, setWasOpen] = useState(false);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setLoading(true);
  }

  useEffect(() => {
    if (open) void Promise.resolve().then(fetchVersions);
  }, [open, fetchVersions]);

  function switchCoreVersion(version: string) {
    modal.confirm({
      title: t('pages.coreSelection.updateConfirm', { core: `${coreName} · ${version}` }),
      content: t('pages.coreSelection.restartConnections'),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: async () => {
        setUpdating(true);
        setUpdateError('');
        try {
          const result = await HttpUtil.post(
            `/panel/api/setting/cores/${coreType}/update`,
            { version },
            { timeout: 960000 },
          );
          if (!result.success) throw new Error(result.msg || t('pages.coreSelection.updateFailed'));
          await fetchVersions();
          void queryClient.invalidateQueries({ queryKey: ['server'] });
          window.dispatchEvent(new Event('xui-core-settings-saved'));
        } catch (error) {
          setUpdateError(error instanceof Error ? error.message : String(error));
        } finally {
          setUpdating(false);
        }
      },
    });
  }

  function updateGeofile(fileName: string) {
    const isSingle = !!fileName;
    modal.confirm({
      title: t('pages.index.geofileUpdateDialog'),
      content: isSingle
        ? t('pages.index.geofileUpdateDialogDesc').replace('#filename#', fileName)
        : t('pages.index.geofilesUpdateDialogDesc'),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: async () => {
        onClose();
        onBusy({ busy: true, tip: t('pages.index.dontRefresh') });
        const url = isSingle
          ? `/panel/api/server/updateGeofile/${fileName}`
          : '/panel/api/server/updateGeofile';
        try {
          await HttpUtil.post(url);
        } finally {
          onBusy({ busy: false });
        }
      },
    });
  }

  const activeKeyStr = Array.isArray(activeKey) ? activeKey[0] : activeKey;
  return (
    <Modal open={open} title={`${coreName} · ${t('update')}`} footer={null} onCancel={onClose}>
      {modalContextHolder}
      <Spin spinning={loading}>
        <Collapse
          accordion
          activeKey={activeKey}
          onChange={setActiveKey}
          items={[
            {
              key: '1',
              label: coreName,
              children: (
                <>
                  {updateError && (
                    <Alert type="error" title={updateError} showIcon style={{ marginBottom: 12 }} />
                  )}
                  {coreType === 'sing-box' ? (
                    <div className="version-list">
                      <div className="version-list-item">
                        <span>{t('pages.index.currentPanelVersion')}</span>
                        <Tag color="blue">{singBoxVersion || coreName}</Tag>
                      </div>
                    </div>
                  ) : (
                    <>
                      {versionsError && (
                        <Alert
                          type="warning"
                          className="mb-12"
                          title={versionsError}
                          action={<Button onClick={() => void fetchVersions()}>Повторить</Button>}
                        />
                      )}
                      <div className="version-filter">
                        <Segmented
                          block
                          aria-label="Тип выпусков ядра"
                          value={releaseChannel}
                          onChange={(value) => setReleaseChannel(value as 'stable' | 'beta')}
                          options={[
                            { label: 'Стабильные', value: 'stable' },
                            { label: 'Бета', value: 'beta' },
                          ]}
                        />
                      </div>
                      <div className="version-list">
                        {visibleVersions.length === 0 ? (
                          <div className="version-list-empty">
                            {loading
                              ? t('loading')
                              : versionsError
                                ? 'Не удалось загрузить версии'
                                : releaseChannel === 'beta'
                                  ? 'Нет доступных предварительных выпусков'
                                  : 'Нет доступных стабильных выпусков'}
                          </div>
                        ) : (
                          visibleVersions.map((version) => {
                            const installed =
                              normalizeVersion(version.tag) === normalizeVersion(singBoxVersion);
                            return (
                              <div className="version-list-item" key={version.tag}>
                                <Space size={8} wrap>
                                  <Tag color={installed ? 'green' : 'blue'}>{version.tag}</Tag>
                                  {installed && <Tag color="success">Установлена</Tag>}
                                </Space>
                                <Radio
                                  aria-label={`Выбрать ${version.tag}`}
                                  checked={installed}
                                  disabled={updating || installed}
                                  onClick={() => switchCoreVersion(version.tag)}
                                />
                              </div>
                            );
                          })
                        )}
                      </div>
                    </>
                  )}
                </>
              ),
            },
            {
              key: '2',
              label: 'Geofiles',
              children: (
                <>
                  <div className="version-list">
                    {GEOFILES.map((file, index) => (
                      <div key={file} className="version-list-item">
                        <Tag color={index % 2 === 0 ? 'purple' : 'green'}>{file}</Tag>
                        <Tooltip title={t('update')}>
                          <ReloadOutlined
                            className="reload-icon"
                            role="button"
                            tabIndex={0}
                            aria-label={t('update')}
                            onClick={() => updateGeofile(file)}
                            onKeyDown={activateOnKey(() => updateGeofile(file))}
                          />
                        </Tooltip>
                      </div>
                    ))}
                  </div>
                  <div className="actions-row">
                    <Button onClick={() => updateGeofile('')}>
                      {t('pages.index.geofilesUpdateAll')}
                    </Button>
                  </div>
                </>
              ),
            },
            {
              key: '3',
              label: t('pages.index.geodataTitle'),
              children: (
                <GeodataSection active={activeKeyStr === '3'} onBusy={onBusy} onClose={onClose} />
              ),
            },
          ]}
        />
      </Spin>
    </Modal>
  );
}
