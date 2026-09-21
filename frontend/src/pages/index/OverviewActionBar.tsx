import { Fragment } from 'react';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Tag, Tooltip } from 'antd';
import {
  ApiOutlined,
  ArrowUpOutlined,
  AreaChartOutlined,
  BarsOutlined,
  CloudDownloadOutlined,
  CloudServerOutlined,
  ControlOutlined,
  PoweroffOutlined,
  ReloadOutlined,
} from '@ant-design/icons';

import { formatPanelVersion } from '@/lib/panel-version';
import type { Status } from '@/models/status';

interface OverviewActionBarProps {
  status: Status;
  coreType: 'xray' | 'sing-box';
  coreVersion: string;
  coreRunning: boolean;
  coreError: string;
  coreColor: string;
  isMobile: boolean;
  accessLogEnable: boolean;
  panelVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  onStopXray: () => void;
  onRestartXray: () => void;
  onOpenLogs: () => void;
  onOpenXrayLogs: () => void;
  onOpenAmneziaWGLogs: () => void;
  onOpenConfig: () => void;
  onOpenBackup: () => void;
  onOpenSystemHistory: () => void;
  onOpenXrayMetrics: () => void;
  onOpenPanelUpdate: () => void;
  onOpenVersionSwitch: () => void;
  lowPower: boolean;
  onRefreshHistory: () => void;
}

interface BarAction {
  key: string;
  icon: ReactNode;
  text: string;
  onClick: () => void;
  primary?: boolean;
}

const XRAY_STATE_KEYS: Record<string, string> = {
  running: 'pages.index.xrayStatusRunning',
  stop: 'pages.index.xrayStatusStop',
  error: 'pages.index.xrayStatusError',
};

export default function OverviewActionBar({
  status,
  coreType,
  coreVersion,
  coreRunning,
  coreError,
  coreColor,
  isMobile,
  accessLogEnable,
  panelVersion,
  latestVersion,
  updateAvailable,
  onStopXray,
  onRestartXray,
  onOpenLogs,
  onOpenXrayLogs,
  onOpenAmneziaWGLogs,
  onOpenConfig,
  onOpenBackup,
  onOpenSystemHistory,
  onOpenXrayMetrics,
  onOpenPanelUpdate,
  onOpenVersionSwitch,
  lowPower,
  onRefreshHistory,
}: OverviewActionBarProps) {
  const { t } = useTranslation();
  const effectiveState =
    coreType === 'sing-box' ? (coreRunning ? 'running' : 'stop') : status.xray.state;
  const stateText =
    coreType === 'sing-box'
      ? coreRunning
        ? 'Работает'
        : coreError
          ? 'Ошибка'
          : 'Остановлен'
      : t(XRAY_STATE_KEYS[status.xray.state] ?? 'pages.index.xrayStatusUnknown');
  const coreName = coreType === 'sing-box' ? 'sing-box' : 'Xray';
  const displayedVersion = coreType === 'sing-box' ? coreVersion : status.xray.version;
  const hasVersion = !!displayedVersion && displayedVersion !== 'Unknown';
  const size = isMobile ? ('small' as const) : ('middle' as const);

  const actionGroups: BarAction[][] = [
    [
      {
        key: 'restart',
        icon: <ReloadOutlined />,
        text: coreType === 'sing-box' ? 'Перезапустить sing-box' : t('pages.index.restartXray'),
        onClick: onRestartXray,
        primary: true,
      },
      {
        key: 'stop',
        icon: <PoweroffOutlined />,
        text: coreType === 'sing-box' ? 'Остановить sing-box' : t('pages.index.stopXray'),
        onClick: onStopXray,
      },
    ],
    [
      {
        key: 'logs',
        icon: <BarsOutlined />,
        text: t('pages.index.logs'),
        onClick: coreType === 'sing-box' ? onOpenXrayLogs : onOpenLogs,
      },
      ...(coreType === 'xray' && accessLogEnable
        ? [
            {
              key: 'accessLogs',
              icon: <BarsOutlined />,
              text: t('pages.index.accessLogs'),
              onClick: onOpenXrayLogs,
            },
          ]
        : []),
      ...(status.amneziawg.configured
        ? [
            {
              key: 'amneziawgLogs',
              icon: <ApiOutlined />,
              text: t('pages.index.amneziawgLogs'),
              onClick: onOpenAmneziaWGLogs,
            },
          ]
        : []),
      {
        key: 'config',
        icon: <ControlOutlined />,
        text: t('pages.index.config'),
        onClick: onOpenConfig,
      },
      {
        key: 'backup',
        icon: <CloudServerOutlined />,
        text: t('pages.index.backupTitle'),
        onClick: onOpenBackup,
      },
    ],
    [
      {
        key: 'history',
        icon: <AreaChartOutlined />,
        text: t('pages.index.systemHistoryTitle'),
        onClick: onOpenSystemHistory,
      },
      ...(coreType === 'xray'
        ? [
            {
              key: 'metrics',
              icon: <ArrowUpOutlined />,
              text: t('pages.index.xrayMetricsTitle'),
              onClick: onOpenXrayMetrics,
            },
          ]
        : []),
      ...(lowPower
        ? [
            {
              key: 'refreshHistory',
              icon: <ReloadOutlined />,
              text: 'Обновить графики',
              onClick: onRefreshHistory,
            },
          ]
        : []),
    ],
  ];

  const statePill = (
    <span className="ov-state" data-state={effectiveState}>
      <span className="ov-state-dot" style={{ color: coreColor || status.xray.color }} />
      <span>{`${coreName} · ${stateText}`}</span>
      {hasVersion && (
        <Tooltip title={t('pages.index.xraySwitch')}>
          <button type="button" className="ov-state-version" onClick={onOpenVersionSwitch}>
            {displayedVersion}
          </button>
        </Tooltip>
      )}
    </span>
  );

  return (
    <div className="ov-bar">
      {coreError ? (
        <Tooltip title={<span className="ov-error-detail">{coreError}</span>}>{statePill}</Tooltip>
      ) : (
        statePill
      )}

      {effectiveState === 'running' && coreError ? (
        <Tooltip title={<span className="ov-error-detail">{coreError}</span>}>
          <Tag color="error">{t('pages.index.xrayStatusError')}</Tag>
        </Tooltip>
      ) : null}

      {updateAvailable ? (
        <Tag
          className="ov-update-tag"
          color="warning"
          icon={<CloudDownloadOutlined />}
          onClick={onOpenPanelUpdate}
        >
          {`${t('update')} ${formatPanelVersion(latestVersion)}`}
        </Tag>
      ) : (
        <Tooltip title={t('pages.index.updatePanel')}>
          <button type="button" className="ov-panel-version ov-mono" onClick={onOpenPanelUpdate}>
            {formatPanelVersion(panelVersion)}
          </button>
        </Tooltip>
      )}

      <div className="ov-bar-actions">
        {actionGroups.map((group, groupIndex) => (
          <Fragment key={group[0].key}>
            {groupIndex > 0 && <span className="ov-bar-sep" />}
            {group.map((action) => (
              <Button
                key={action.key}
                type={action.primary ? undefined : 'text'}
                color={action.primary ? 'primary' : undefined}
                variant={action.primary ? 'outlined' : undefined}
                size={size}
                icon={action.icon}
                aria-label={action.text}
                onClick={action.onClick}
              >
                {isMobile ? undefined : action.text}
              </Button>
            ))}
          </Fragment>
        ))}
      </div>
    </div>
  );
}
