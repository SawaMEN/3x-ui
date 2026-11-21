import { Alert, Collapse, Input, Select, Switch } from 'antd';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import {
  automaticSubscriptionOrigin,
  subscriptionCaddyConfig,
  supportsManagedSubscriptionListener,
} from './subscription-domains';

function normalizeAlias(value: string): string {
  return value.trim().toLowerCase().replace(/\.$/, '');
}

export default function SubscriptionDomains({
  allSetting,
  updateSetting,
}: {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}) {
  const { t } = useTranslation();
  const l = (key: string) => t('pages.settings.subscriptionWorkspace.' + key);
  const { data: status } = useDomainFrontStatus();
  const gateway = status?.config;
  const automaticURL = automaticSubscriptionOrigin(gateway, allSetting);
  const publicURL = allSetting.subPublicURL;
  const followsGateway = Boolean(automaticURL && !publicURL.trim());
  const domains = gateway?.domains.filter(
    (domain) => domain.enabled && ['direct', 'cdn', 'relay', 'sub_link_only'].includes(domain.mode),
  );
  const selectedDomain = domains?.find((domain) => domain.name === gateway?.subscriptionDomain);
  let origin: URL | undefined;
  try {
    const parsed = new URL(publicURL.trim() || automaticURL);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') origin = parsed;
  } catch {
    // The address can be incomplete while editing.
  }

  const port = origin ? Number(origin.port || (origin.protocol === 'http:' ? 80 : 443)) : undefined;
  const originDomain = origin?.hostname.replace(/\.$/, '');
  const aliases = [
    ...new Set(allSetting.subAllowedDomains.split(/[,;\s]+/).map(normalizeAlias)),
  ].filter((name) => name && name !== originDomain);
  const usingSubscriptionListener =
    supportsManagedSubscriptionListener(gateway, allSetting) &&
    origin?.protocol === 'https:' &&
    originDomain === gateway?.subscriptionDomain &&
    port === allSetting.subPort;
  const managedHost = gateway?.enabled && domains?.some((domain) => domain.name === originDomain);
  const proxyConfig =
    followsGateway || usingSubscriptionListener ? '' : subscriptionCaddyConfig(allSetting);

  return (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subPublicURL')}
        description={
          <>
            {l('publicURLHelp')} <Link to="/settings#domains">{l('domainsAndTLS')}</Link>
          </>
        }
      >
        <Input
          aria-label={t('pages.settings.subPublicURL')}
          placeholder={automaticURL || 'https://sub.example.com:8443'}
          value={publicURL}
          onChange={(event) => {
            const subPublicURL = event.target.value;
            updateSetting({
              subPublicURL,
              ...(!subPublicURL.trim() ? { subCDNEnable: false } : {}),
            });
          }}
        />
      </SettingListItem>
      {managedHost && port !== gateway.publicPort && !usingSubscriptionListener && (
        <Alert type="warning" showIcon title={t('pages.settings.subGatewayPortMismatch')} />
      )}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subAllowedDomains')}
        description={l('aliasesHelp')}
      >
        <Select
          mode="tags"
          allowClear
          aria-label={t('pages.settings.subAllowedDomains')}
          style={{ width: '100%' }}
          tokenSeparators={[',', ';', ' ']}
          value={aliases}
          options={domains
            ?.filter((domain) => domain.name !== originDomain)
            .map((domain) => ({ value: domain.name, label: domain.name }))}
          onChange={(values: string[]) =>
            updateSetting({
              subAllowedDomains: [...new Set(values.map(normalizeAlias))]
                .filter((value) => value && value !== originDomain)
                .join(','),
            })
          }
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subRestrictDomains')}
        description={t('pages.settings.subRestrictDomainsDesc')}
      >
        <Switch
          aria-label={t('pages.settings.subRestrictDomains')}
          checked={allSetting.subRestrictDomains}
          disabled={!origin && !allSetting.subRestrictDomains}
          onChange={(subRestrictDomains) => updateSetting({ subRestrictDomains })}
        />
      </SettingListItem>
      <Collapse
        ghost
        items={[
          {
            key: 'proxy',
            label: l('proxy'),
            children: (
              <>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.subCDNEnable')}
                  description={t('pages.settings.subCDNDesc')}
                >
                  <Switch
                    aria-label={t('pages.settings.subCDNEnable')}
                    checked={
                      followsGateway ? selectedDomain?.mode === 'cdn' : allSetting.subCDNEnable
                    }
                    disabled={
                      followsGateway || (origin?.protocol !== 'https:' && !allSetting.subCDNEnable)
                    }
                    onChange={(subCDNEnable) => updateSetting({ subCDNEnable })}
                  />
                </SettingListItem>
                {proxyConfig && (
                  <SettingListItem
                    paddings="small"
                    title={t('pages.settings.subReverseProxy')}
                    description={t('pages.settings.subReverseProxyDesc')}
                  >
                    <Input.TextArea
                      aria-label={t('pages.settings.subReverseProxy')}
                      readOnly
                      autoSize={{ minRows: 5, maxRows: 12 }}
                      value={proxyConfig}
                      className="subscription-proxy-config"
                    />
                  </SettingListItem>
                )}
              </>
            ),
          },
        ]}
      />
    </>
  );
}
