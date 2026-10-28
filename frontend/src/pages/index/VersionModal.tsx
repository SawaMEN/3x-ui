import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Collapse, Modal, Spin, Tag, Tooltip } from 'antd';
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
  const coreName = coreType === 'xray' ? 'Xray' : 'sing-box';
  const [updating, setUpdating] = useState(false);
  const [updateError, setUpdateError] = useState('');
  const [modal, modalContextHolder] = Modal.useModal();
  const [activeKey, setActiveKey] = useState<string | string[]>('1');
  const [singBoxVersion, setSingBoxVersion] = useState('');
  const [loading, setLoading] = useState(false);

  const fetchVersions = useCallback(async () => {
    try {
      const msg = await HttpUtil.get<{ version?: string }>('/panel/api/setting/singbox/status');
      setSingBoxVersion(msg.obj?.version || '');
    } finally {
      setLoading(false);
    }
  }, []);

  const [wasOpen, setWasOpen] = useState(false);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setLoading(true);
  }

  useEffect(() => {
    if (open) void fetchVersions();
  }, [open, fetchVersions]);

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
                  <Alert
                    type="info"
                    showIcon
                    title={singBoxVersion || coreName}
                    description={
                      coreType === 'sing-box'
                        ? t('pages.coreSelection.bundledUpdateNote')
                        : t('pages.coreSelection.standaloneUpdateNote')
                    }
                  />
                  {updateError && (
                    <Alert type="error" title={updateError} showIcon style={{ marginTop: 12 }} />
                  )}
                  {coreType !== 'sing-box' && (
                    <Button
                      type="primary"
                      loading={updating}
                      style={{ marginTop: 12 }}
                      onClick={() =>
                        modal.confirm({
                          title: t('pages.coreSelection.updateConfirm', { core: coreName }),
                          content: t('pages.coreSelection.restartConnections'),
                          onOk: async () => {
                            setUpdating(true);
                            setUpdateError('');
                            try {
                              const result = await HttpUtil.post(
                                `/panel/api/setting/cores/${coreType}/update`,
                                {},
                                { timeout: 330000 },
                              );
                              if (!result.success)
                                throw new Error(
                                  result.msg || t('pages.coreSelection.updateFailed'),
                                );
                              await fetchVersions();
                              void queryClient.invalidateQueries({ queryKey: ['server'] });
                              window.dispatchEvent(new Event('xui-core-settings-saved'));
                            } catch (error) {
                              setUpdateError(
                                error instanceof Error ? error.message : String(error),
                              );
                            } finally {
                              setUpdating(false);
                            }
                          },
                        })
                      }
                    >
                      {t('update')}
                    </Button>
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
