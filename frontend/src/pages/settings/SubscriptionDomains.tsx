import { Alert, Input, InputNumber, Select, Switch, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
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
  let origin: URL | undefined;
  try {
    origin = new URL(publicURL || automaticURL);
  } catch {
    /* The manual origin may be incomplete while editing. */
  }
  const selected = [
    ...new Set([
      ...(origin ? [origin.hostname] : []),
      ...allSetting.subAllowedDomains.split(/[,;\s]+/).filter(Boolean),
    ]),
  ];
  const port = origin ? Number(origin.port || (origin.protocol === 'http:' ? 80 : 443)) : 443;
  function selectDomains(domains: string[], nextPort = port) {
    const names = [...new Set(domains.map((name) => name.trim().toLowerCase()).filter(Boolean))];
    if (!names.length) return;
    const scheme = origin?.protocol || 'https:';
    const defaultPort = scheme === 'https:' ? 443 : 80;
    updateSetting({
      subRestrictDomains: true,
      subPublicURL: `${scheme}//${names[0]}${nextPort === defaultPort ? '' : `:${nextPort}`}`,
      subAllowedDomains: names.slice(1).join(','),
      subCDNEnable:
        status?.config.domains.find((item) => item.name === names[0])?.mode === 'cdn' ||
        (!status?.config.domains.some((item) => item.name === names[0]) && allSetting.subCDNEnable),
    });
  }
  return (
    <>
      <Typography.Title level={5}>{t('pages.settings.subscriptionAccess')}</Typography.Title>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subRestrictDomains')}
        description={t('pages.settings.subRestrictDomainsDesc')}
      >
        <Switch
          aria-label={t('pages.settings.subRestrictDomains')}
          checked={allSetting.subRestrictDomains}
          disabled={!selected.length}
          onChange={(enabled) =>
            enabled ? selectDomains(selected) : updateSetting({ subRestrictDomains: false })
          }
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subSelectedDomains')}
        description={t('pages.settings.subSelectedDomainsDesc')}
      >
        <Select
          mode="tags"
          aria-label={t('pages.settings.subSelectedDomains')}
          style={{ width: '100%' }}
          value={selected}
          tokenSeparators={[',', ';', ' ']}
          options={status?.config.domains
            .filter((item) => item.enabled)
            .map((item) => ({
              value: item.name,
              label: item.name,
            }))}
          onChange={(names: string[]) => {
            if (names.length) selectDomains(names);
          }}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subPublicPort')}
        description={t('pages.settings.subPublicPortDesc')}
      >
        <InputNumber
          min={1}
          max={65535}
          precision={0}
          aria-label={t('pages.settings.subPublicPort')}
          value={port}
          disabled={!selected.length}
          onChange={(value) => {
            if (value != null) selectDomains(selected, value);
          }}
        />
      </SettingListItem>
      {status?.config.enabled &&
        selected.some((name) =>
          status.config.domains.some((item) => item.enabled && item.name === name),
        ) &&
        port !== status.config.publicPort && (
          <Alert type="warning" showIcon title={t('pages.settings.subGatewayPortMismatch')} />
        )}
      {managed && (
        <Alert
          type="info"
          showIcon
          title={t('pages.settings.domains.managedSubscription')}
          description={automaticURL}
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
