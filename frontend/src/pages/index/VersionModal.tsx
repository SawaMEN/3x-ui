import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Collapse, Modal, Radio, Spin, Tag, Tooltip } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
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

type SingBoxReleaseVersion = {
  version?: string;
  prerelease?: boolean;
};

export default function VersionModal({ open, status, onClose, onBusy }: VersionModalProps) {
  const { t } = useTranslation();
  const [modal, modalContextHolder] = Modal.useModal();
  const [activeKey, setActiveKey] = useState<string | string[]>('1');
  const [versions, setVersions] = useState<string[]>([]);
  const [coreType, setCoreType] = useState<'xray' | 'sing-box'>('xray');
  const [singBoxVersion, setSingBoxVersion] = useState('');
  const [loading, setLoading] = useState(false);

  const fetchVersions = useCallback(async () => {
    try {
      const settingsMsg = await HttpUtil.post<{ coreType?: 'xray' | 'sing-box' }>(
        '/panel/api/setting/all',
      );
      const selectedCore =
        settingsMsg?.success && settingsMsg.obj?.coreType === 'sing-box' ? 'sing-box' : 'xray';
      setCoreType(selectedCore);

      if (selectedCore === 'sing-box') {
        const [versionMsg, statusMsg] = await Promise.all([
          HttpUtil.get<Array<SingBoxReleaseVersion | string>>('/panel/api/setting/singbox/versions'),
          HttpUtil.get<{ version?: string }>('/panel/api/setting/singbox/status'),
        ]);
        if (versionMsg?.success) {
          setVersions(
            (versionMsg.obj || [])
              .map((item) => (typeof item === 'string' ? item : item?.version || ''))
              .filter(Boolean),
          );
        }
        if (statusMsg?.success) setSingBoxVersion(statusMsg.obj?.version || '');
      } else {
        const msg = await HttpUtil.get<string[]>('/panel/api/server/getXrayVersion');
        if (msg?.success) setVersions(msg.obj || []);
      }
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

  function switchCoreVersion(version: string) {
    const isSingBox = coreType === 'sing-box';
    modal.confirm({
      title: isSingBox ? 'Переключить версию sing-box?' : t('pages.index.xraySwitchVersionDialog'),
      content: isSingBox
        ? `Установить sing-box ${version} и заменить текущую версию?`
        : t('pages.index.xraySwitchVersionDialogDesc').replace('#version#', version),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: async () => {
        onClose();
        onBusy({ busy: true, tip: t('pages.index.dontRefresh') });
        try {
          if (isSingBox) {
            await HttpUtil.post(`/panel/api/setting/singbox/install/${encodeURIComponent(version)}`);
          } else {
            await HttpUtil.post(`/panel/api/server/installXray/${encodeURIComponent(version)}`);
          }
        } finally {
          onBusy({ busy: false });
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
    <Modal
      open={open}
      title={coreType === 'sing-box' ? 'Обновления sing-box' : t('pages.index.xrayUpdates')}
      footer={null}
      onCancel={onClose}
    >
      {modalContextHolder}
      <Spin spinning={loading}>
        <Collapse
          accordion
          activeKey={activeKey}
          onChange={setActiveKey}
          items={[
            {
              key: '1',
              label: coreType === 'sing-box' ? 'sing-box' : 'Xray',
              children: (
                <>
                  <Alert
                    type="warning"
                    className="mb-12"
                    title={
                      coreType === 'sing-box'
                        ? 'Выберите версию sing-box'
                        : t('pages.index.xraySwitchClickDesk')
                    }
                    showIcon
                  />
                  <div className="version-list">
                    {versions.map((version, index) => (
                      <div key={version} className="version-list-item">
                        <Tag color={index % 2 === 0 ? 'purple' : 'green'}>{version}</Tag>
                        <Radio
                          checked={
                            coreType === 'sing-box'
                              ? singBoxVersion.includes(version.replace(/^v/, ''))
                              : version === `v${status?.xray?.version}`
                          }
                          onClick={() => switchCoreVersion(version)}
                        />
                      </div>
                    ))}
                  </div>
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
