import { Tabs, Typography } from 'antd';
import {
  CloudDownloadOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  ReloadOutlined,
  SafetyOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router';
import type { AllSetting } from '@/models/setting';
import { FirewallManager } from '@/pages/firewall/FirewallManager';
import DomainFrontTab from './DomainFrontTab';
import ProxyCoreTab from './ProxyCoreTab';
import SwapSettingsTab from './SwapSettingsTab';
import SystemUpdatePanel from './SystemUpdatePanel';
import { getAdditionalSettingsTab } from './additional-settings';
import './AdditionalSettingsTab.css';

export default function AdditionalSettingsTab(props: {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  saveSettings: () => Promise<boolean | undefined>;
  settingsDirty: boolean;
}) {
  const { t } = useTranslation();
  const { hash } = useLocation();
  const navigate = useNavigate();
  const activeKey = getAdditionalSettingsTab(hash) ?? 'firewall';
  const tabs = [
    {
      key: 'firewall',
      label: t('pages.settings.advanced.firewall'),
      icon: <SafetyOutlined />,
      children: <FirewallManager />,
    },
    {
      key: 'core',
      label: t('pages.settings.coreType'),
      icon: <SettingOutlined />,
      children: <ProxyCoreTab active={activeKey === 'core'} />,
    },
    {
      key: 'components',
      label: t('pages.settings.advanced.components'),
      icon: <CloudDownloadOutlined />,
      children: <SystemUpdatePanel section="components" />,
    },
    {
      key: 'system',
      label: t('pages.settings.advanced.system'),
      icon: <ReloadOutlined />,
      children: <SystemUpdatePanel section="system" />,
    },
    {
      key: 'swap',
      label: t('pages.settings.swap.menu'),
      icon: <DatabaseOutlined />,
      children: <SwapSettingsTab />,
    },
    {
      key: 'domains',
      label: t('pages.settings.domains.title'),
      icon: <CloudServerOutlined />,
      children: <DomainFrontTab {...props} />,
    },
  ];
  return (
    <section className="additional-settings">
      <Typography.Title level={3} className="additional-settings-title">
        {t('pages.settings.advanced.title')}
      </Typography.Title>
      <Tabs
        activeKey={activeKey}
        onChange={(key) => navigate(`/settings#advanced/${key}`)}
        items={tabs}
      />
    </section>
  );
}
