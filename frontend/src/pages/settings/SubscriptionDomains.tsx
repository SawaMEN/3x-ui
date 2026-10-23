import { Input, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { subscriptionCaddyConfig } from './subscription-domains';

export default function SubscriptionDomains({
  allSetting,
  updateSetting,
}: {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}) {
  const { t } = useTranslation();
  const config = subscriptionCaddyConfig(allSetting);
  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subPublicURL')}
        description={t('pages.settings.subPublicURLDesc')}
      >
        <Input
          aria-label={t('pages.settings.subPublicURL')}
          placeholder="https://sub.example.com"
          value={allSetting.subPublicURL}
          onChange={(e) => updateSetting({ subPublicURL: e.target.value })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subAllowedDomains')}
        description={t('pages.settings.subAllowedDomainsDesc')}
      >
        <Input.TextArea
          aria-label={t('pages.settings.subAllowedDomains')}
          placeholder="sub2.example.com, cdn.example.com"
          rows={2}
          value={allSetting.subAllowedDomains}
          onChange={(e) => updateSetting({ subAllowedDomains: e.target.value })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subCDNEnable')}
        description={t('pages.settings.subCDNDesc')}
      >
        <Switch
          aria-label={t('pages.settings.subCDNEnable')}
          checked={allSetting.subCDNEnable}
          onChange={(value) => updateSetting({ subCDNEnable: value })}
        />
      </SettingListItem>
      {config && (
        <SettingListItem
          paddings="small"
          title={t('pages.settings.subReverseProxy')}
          description={t('pages.settings.subReverseProxyDesc')}
        >
          <Input.TextArea
            aria-label={t('pages.settings.subReverseProxy')}
            readOnly
            rows={12}
            value={config}
            style={{ fontFamily: 'monospace' }}
          />
        </SettingListItem>
      )}
    </>
  );
}
