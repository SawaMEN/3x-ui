import { Alert, Button, Input, Switch } from 'antd';
import { useTranslation } from 'react-i18next';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useNavigate } from 'react-router';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import { subscriptionCaddyConfig } from './subscription-domains';

export default function SubscriptionDomains({
  allSetting,
  updateSetting,
}: {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: status } = useDomainFrontStatus();
  const managed =
    status?.config.enabled && status.config.subscriptionDomain ? status.config : undefined;
  const domain = managed?.domains.find((item) => item.name === managed.subscriptionDomain);
  const automaticURL = managed
    ? `https://${managed.subscriptionDomain}${managed.publicPort === 443 ? '' : `:${managed.publicPort}`}`
    : '';
  const overridden = !!allSetting.subPublicURL.trim();
  const publicURL = allSetting.subPublicURL;
  const usingManaged = !!managed && !overridden;
  const config = usingManaged ? '' : subscriptionCaddyConfig(allSetting);
  return (
    <>
      {managed && (
        <Alert
          type="info"
          showIcon
          title={t('pages.settings.domains.managedSubscription')}
          description={automaticURL}
          action={
            <Button onClick={() => navigate('/settings?subscriptionTab=tls#subscription')}>
              {t('pages.settings.domains.subscriptionTitle')}
            </Button>
          }
        />
      )}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subPublicURL')}
        description={
          <>
            {t('pages.settings.subPublicURLDesc')} {t('pages.settings.domains.publicOverrideHelp')}
          </>
        }
      >
        <Input
          aria-label={t('pages.settings.subPublicURL')}
          placeholder={automaticURL || 'https://sub.example.com'}
          value={publicURL}
          onChange={(e) =>
            updateSetting({
              subPublicURL: e.target.value,
              ...(managed && !e.target.value.trim() ? { subCDNEnable: false } : {}),
            })
          }
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
          checked={usingManaged ? domain?.mode === 'cdn' : allSetting.subCDNEnable}
          disabled={usingManaged}
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
