import type { ReactNode } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Input,
  InputNumber,
  Select,
  Switch,
  Tabs,
  Typography,
} from 'antd';
import {
  ApartmentOutlined,
  BranchesOutlined,
  CompassOutlined,
  FileTextOutlined,
  IdcardOutlined,
  InfoCircleOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate, useSearchParams } from 'react-router';
import type { AllSetting } from '@/models/setting';
import type { SubProfileMode, SubHostSortMode } from '@/schemas/setting';
import { onNumber } from '@/utils/onNumber';
import { DefaultSettingTag, SettingListItem } from '@/components/ui';
import { RemarkTemplateField } from '@/components/form';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { sanitizePath, normalizePath } from './uriPath';
import HappSettingsContent from './HappSettingsContent';
import SubscriptionDomains from './SubscriptionDomains';
import SubscriptionFormatsTab from './SubscriptionFormatsTab';
import SubscriptionBalancersTab from './SubscriptionBalancersTab';
import { remoteSourceBadge } from './subscriptionShared';
import './domain-workspace.css';
import './subscription-workspace.css';

interface SubscriptionGeneralTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  actions?: ReactNode;
}

function uriWithSubscriptionPort(uri: string, previousPort: number, nextPort: number): string {
  if (!uri) return uri;
  try {
    const parsed = new URL(uri);
    if (parsed.port !== String(previousPort)) return uri;
    parsed.port =
      (parsed.protocol === 'https:' && nextPort === 443) ||
      (parsed.protocol === 'http:' && nextPort === 80)
        ? ''
        : String(nextPort);
    return parsed.toString();
  } catch {
    return uri;
  }
}

function usesOldDefaultPort(uri: string): boolean {
  try {
    return new URL(uri).port === '2096';
  } catch {
    return false;
  }
}

export default function SubscriptionGeneralTab({
  allSetting,
  updateSetting,
  actions,
}: SubscriptionGeneralTabProps) {
  const { t } = useTranslation();
  const l = (key: string) => t('pages.settings.subscriptionWorkspace.' + key);
  const navigate = useNavigate();
  const { hash } = useLocation();
  const [searchParams] = useSearchParams();
  const { isMobile } = useMediaQuery();
  const panels = [
    { key: 'access', label: l('access'), icon: <LinkOutlined /> },
    { key: 'formats', label: l('formats'), icon: <FileTextOutlined /> },
    { key: 'profile', label: l('profile'), icon: <IdcardOutlined /> },
    { key: 'information', label: l('information'), icon: <InfoCircleOutlined /> },
    { key: 'apps', label: l('apps'), icon: <BranchesOutlined /> },
    { key: 'balancers', label: t('pages.settings.subBalancers.menu'), icon: <ApartmentOutlined /> },
  ] as const;
  const requestedPanel =
    hash === '#subscription-formats'
      ? 'formats'
      : hash === '#subscription-balancers'
        ? 'balancers'
        : searchParams.get('subscriptionTab') === 'happ'
          ? 'apps'
          : searchParams.get('subscriptionTab');
  const activePanel = panels.find(({ key }) => key === requestedPanel) ?? panels[0];

  function selectPanel(key: (typeof panels)[number]['key']) {
    const params = new URLSearchParams(searchParams);
    if (params.get('subscriptionTab') === 'happ' && key !== 'apps') params.delete('happTab');
    if (key === 'access') params.delete('subscriptionTab');
    else params.set('subscriptionTab', key);
    navigate({ search: params.toString(), hash: '#subscription' });
  }

  const changeSubscriptionPort = (nextPort: number) => {
    updateSetting({
      subPort: nextPort,
      subURI: uriWithSubscriptionPort(allSetting.subURI, allSetting.subPort, nextPort),
      subJsonURI: uriWithSubscriptionPort(allSetting.subJsonURI, allSetting.subPort, nextPort),
      subClashURI: uriWithSubscriptionPort(allSetting.subClashURI, allSetting.subPort, nextPort),
    });
  };
  const hasOldSubscriptionURL =
    allSetting.subPort !== 2096 &&
    [allSetting.subURI, allSetting.subJsonURI, allSetting.subClashURI].some(usesOldDefaultPort);

  const repairOldSubscriptionURLs = () => {
    updateSetting({
      subURI: uriWithSubscriptionPort(allSetting.subURI, 2096, allSetting.subPort),
      subJsonURI: uriWithSubscriptionPort(allSetting.subJsonURI, 2096, allSetting.subPort),
      subClashURI: uriWithSubscriptionPort(allSetting.subClashURI, 2096, allSetting.subPort),
    });
  };

  const connection = (
    <div className="subscription-settings-stack">
      <Card className="subscription-settings-card">
        <SettingListItem
          paddings="small"
          title={t('pages.settings.subEnable')}
          description={t('pages.settings.subEnableDesc')}
        >
          <Switch
            checked={allSetting.subEnable}
            onChange={(v) => updateSetting({ subEnable: v })}
          />
        </SettingListItem>

        <SubscriptionDomains allSetting={allSetting} updateSetting={updateSetting} />
        <SettingListItem
          paddings="small"
          title={t('pages.settings.subPath')}
          description={t('pages.settings.subPathDesc')}
        >
          <Input
            value={allSetting.subPath}
            placeholder="/sub/"
            onChange={(e) => updateSetting({ subPath: sanitizePath(e.target.value) })}
            onBlur={() => updateSetting({ subPath: normalizePath(allSetting.subPath) })}
          />
        </SettingListItem>
      </Card>
      <Collapse
        items={[
          {
            key: 'listener',
            label: l('listener'),
            children: (
              <>
                {' '}
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.subListen')}
                  description={t('pages.settings.subListenDesc')}
                >
                  <Input
                    value={allSetting.subListen}
                    onChange={(e) => updateSetting({ subListen: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.subDomain')}
                  description={l('listenerDomainHelp')}
                >
                  <Input
                    value={allSetting.subDomain}
                    onChange={(e) => updateSetting({ subDomain: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title={t('pages.settings.subPort')}
                  badge={<DefaultSettingTag settingKey="subPort" value={allSetting.subPort} />}
                  description={l('listenerPortHelp')}
                >
                  <InputNumber
                    value={allSetting.subPort}
                    min={1}
                    max={65535}
                    style={{ width: '100%' }}
                    onChange={onNumber(changeSubscriptionPort)}
                  />
                </SettingListItem>
                {hasOldSubscriptionURL && (
                  <Alert
                    type="warning"
                    showIcon
                    style={{ margin: '12px 20px' }}
                    title={t('pages.settings.subOldPortWarning')}
                    action={
                      <Button size="small" onClick={repairOldSubscriptionURLs}>
                        {t('pages.settings.subOldPortRepair')}
                      </Button>
                    }
                  />
                )}
              </>
            ),
          },
          {
            key: 'legacy',
            label: l('formatOverrides'),
            children: (
              <>
                {' '}
                <SettingListItem
                  paddings="small"
                  title={l('primaryURL')}
                  description={l('formatOverrideHelp')}
                >
                  <Input
                    value={allSetting.subURI}
                    placeholder="(http|https)://domain[:port]/path/"
                    onChange={(e) => updateSetting({ subURI: e.target.value })}
                  />
                </SettingListItem>
                {(['subJsonURI', 'subClashURI'] as const).map((key) => (
                  <SettingListItem
                    key={key}
                    paddings="small"
                    title={l(key)}
                    description={l('formatOverrideHelp')}
                  >
                    <Input
                      value={allSetting[key]}
                      placeholder="https://sub.example.com/path/"
                      onChange={(e) => updateSetting({ [key]: e.target.value })}
                    />
                  </SettingListItem>
                ))}
              </>
            ),
          },
        ]}
      />
    </div>
  );

  const profile = (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subTitle')}
        description={t('pages.settings.subTitleDesc')}
      >
        <RemarkTemplateField
          value={allSetting.subTitle}
          onChange={(v) => updateSetting({ subTitle: v })}
          metadataOnly
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subSupportUrl')}
        description={t('pages.settings.subSupportUrlDesc')}
      >
        <RemarkTemplateField
          value={allSetting.subSupportUrl}
          placeholder="https://example.com"
          onChange={(v) => updateSetting({ subSupportUrl: v })}
          metadataOnly
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subProfileMode')}
        description={t('pages.settings.subProfileModeDesc')}
      >
        <Select<SubProfileMode>
          id="sub-profile-mode"
          aria-label={t('pages.settings.subProfileMode')}
          value={allSetting.subProfileMode}
          style={{ width: '100%' }}
          onChange={(value) => updateSetting({ subProfileMode: value })}
          options={[
            { value: 'none', label: t('pages.settings.subProfileModeNone') },
            { value: 'builtin', label: t('pages.settings.subProfileModeBuiltin') },
            { value: 'custom', label: t('pages.settings.subProfileModeCustom') },
          ]}
        />
      </SettingListItem>
      {allSetting.subProfileMode === 'builtin' ? (
        <Alert
          type="warning"
          showIcon
          style={{ margin: '12px 20px' }}
          title={t('pages.settings.subProfileBuiltinWarning')}
        />
      ) : null}
      {allSetting.subProfileMode === 'custom' ? (
        <SettingListItem
          paddings="small"
          title={t('pages.settings.subProfileUrl')}
          description={t('pages.settings.subProfileUrlDesc')}
        >
          <RemarkTemplateField
            value={allSetting.subProfileUrl}
            placeholder="https://example.com"
            onChange={(v) => updateSetting({ subProfileUrl: v })}
            metadataOnly
          />
        </SettingListItem>
      ) : null}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subAnnounce')}
        description={t('pages.settings.subAnnounceDesc')}
      >
        <RemarkTemplateField
          value={allSetting.subAnnounce}
          onChange={(v) => updateSetting({ subAnnounce: v })}
          multiline
          rows={3}
          metadataOnly
        />
      </SettingListItem>

      {(['subTelegramLocalEnable', 'subTelegramRemoteEnable', 'subTelegramWebEnable'] as const).map(
        (key) => (
          <SettingListItem
            key={key}
            paddings="small"
            title={t(`pages.settings.${key}`)}
            description={t(`pages.settings.${key}Desc`)}
          >
            <Switch
              aria-label={t(`pages.settings.${key}`)}
              checked={allSetting[key]}
              onChange={(value) => updateSetting({ [key]: value })}
            />
          </SettingListItem>
        ),
      )}
      <SettingListItem paddings="small" title={t('pages.settings.subAppearanceTheme')}>
        <Select
          id="sub-appearance-theme"
          aria-label={t('pages.settings.subAppearanceTheme')}
          value={allSetting.subAppearanceTheme}
          style={{ width: '100%' }}
          options={[
            { value: 'classic', label: t('pages.settings.subAppearanceClassic') },
            { value: 'luna', label: t('subscription.themes.luna') },
            { value: 'underground', label: t('subscription.themes.underground') },
            ...(['light', 'dark', 'ultra-dark', 'colorful', 'blue-gray', 'cyberpunk'] as const).map(
              (value) => ({ value, label: t(`subscription.themes.${value}`) }),
            ),
          ]}
          onChange={(value) => updateSetting({ subAppearanceTheme: value })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subAppearanceEnable')}
        description={t('pages.settings.subAppearanceDesc')}
      >
        <Switch
          checked={allSetting.subAppearanceEnable}
          onChange={(v) => updateSetting({ subAppearanceEnable: v })}
        />
      </SettingListItem>
      {allSetting.subAppearanceEnable && (
        <>
          <SettingListItem paddings="small" title={t('pages.settings.subAppearanceTitle')}>
            <Input
              value={allSetting.subAppearanceTitle}
              maxLength={160}
              onChange={(e) => updateSetting({ subAppearanceTitle: e.target.value })}
            />
          </SettingListItem>
          <SettingListItem paddings="small" title={t('pages.settings.subAppearanceDescription')}>
            <Input.TextArea
              rows={3}
              maxLength={2000}
              value={allSetting.subAppearanceDescription}
              onChange={(e) => updateSetting({ subAppearanceDescription: e.target.value })}
            />
          </SettingListItem>
          <SettingListItem paddings="small" title={t('pages.settings.subAppearanceLogoURL')}>
            <Input
              placeholder="https://example.com/logo.png"
              value={allSetting.subAppearanceLogoURL}
              onChange={(e) => updateSetting({ subAppearanceLogoURL: e.target.value })}
            />
          </SettingListItem>
          <SettingListItem paddings="small" title={t('pages.settings.subAppearanceAccent')}>
            <Input
              type="color"
              aria-label={t('pages.settings.subAppearanceAccent')}
              value={allSetting.subAppearanceAccent}
              onChange={(e) => updateSetting({ subAppearanceAccent: e.target.value })}
            />
          </SettingListItem>
          <Alert
            showIcon
            type="info"
            style={{ margin: '12px 20px' }}
            title={t('pages.settings.subAppearancePreviewHint')}
          />
        </>
      )}
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subThemeDir')}
        description={
          <>
            {t('pages.settings.subThemeDirDesc')}{' '}
            <a
              href="https://github.com/MHSanaei/3x-ui/blob/main/docs/custom-subscription-templates.md"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('pages.settings.subThemeDirDocs')}
            </a>
          </>
        }
      >
        <Input
          value={allSetting.subThemeDir}
          placeholder="/etc/3x-ui/sub_templates/my-theme/"
          onChange={(e) => updateSetting({ subThemeDir: e.target.value })}
        />
      </SettingListItem>
    </>
  );
  const information = (
    <>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subHostSortMode')}
        description={t('pages.settings.subHostSortModeDesc')}
      >
        <Select
          id="sub-host-sort-mode"
          aria-label={t('pages.settings.subHostSortMode')}
          value={allSetting.subHostSortMode}
          onChange={(value: SubHostSortMode) => updateSetting({ subHostSortMode: value })}
          options={[
            { value: 'inbound-first', label: t('pages.settings.subHostSortInbound') },
            { value: 'hosts-first', label: t('pages.settings.subHostSortHosts') },
          ]}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subEncrypt')}
        description={t('pages.settings.subEncryptDesc')}
      >
        <Switch
          checked={allSetting.subEncrypt}
          onChange={(v) => updateSetting({ subEncrypt: v })}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.remarkTemplate')}
        description={t('pages.settings.remarkTemplateDesc')}
      >
        <RemarkTemplateField
          value={allSetting.remarkTemplate}
          onChange={(v) => updateSetting({ remarkTemplate: v })}
          maxLength={256}
        />
      </SettingListItem>
      <SettingListItem
        paddings="small"
        title={t('pages.settings.subShowIdentityOnAllLinks')}
        description={t('pages.settings.subShowIdentityOnAllLinksDesc')}
      >
        <Switch
          checked={allSetting.subShowIdentityOnAllLinks}
          onChange={(v) => updateSetting({ subShowIdentityOnAllLinks: v })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subInfoNodeEnable')}
        description={t('pages.settings.subInfoNodeEnableDesc')}
      >
        <Switch
          checked={allSetting.subInfoNodeEnable}
          onChange={(v) => updateSetting({ subInfoNodeEnable: v })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subCalendarExpireInclusive')}
        description={t('pages.settings.subCalendarExpireInclusiveDesc')}
      >
        <Switch
          checked={allSetting.subCalendarExpireInclusive}
          onChange={(v) => updateSetting({ subCalendarExpireInclusive: v })}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subExpiredTemplate')}
        description={t('pages.settings.subExpiredTemplateDesc')}
      >
        <RemarkTemplateField
          value={allSetting.subExpiredTemplate}
          onChange={(v) => updateSetting({ subExpiredTemplate: v })}
          maxLength={256}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subTrafficDepletedTemplate')}
        description={t('pages.settings.subTrafficDepletedTemplateDesc')}
      >
        <RemarkTemplateField
          value={allSetting.subTrafficDepletedTemplate}
          onChange={(v) => updateSetting({ subTrafficDepletedTemplate: v })}
          maxLength={256}
        />
      </SettingListItem>

      <SettingListItem
        paddings="small"
        title={t('pages.settings.subUpdates')}
        badge={<DefaultSettingTag settingKey="subUpdates" value={allSetting.subUpdates} />}
        description={t('pages.settings.subUpdatesDesc')}
      >
        <InputNumber
          value={allSetting.subUpdates}
          min={0}
          max={525600}
          style={{ width: '100%' }}
          onChange={onNumber((v) => updateSetting({ subUpdates: v }))}
        />
      </SettingListItem>
    </>
  );
  const apps = (
    <Tabs
      key={searchParams.get('subscriptionTab') === 'happ' ? 'happ-link' : 'apps'}
      items={[
        {
          key: 'happ',
          label: 'Happ',
          children: (
            <HappSettingsContent
              allSetting={allSetting}
              updateSetting={updateSetting}
              isMobile={isMobile}
              remoteSourceBadge={remoteSourceBadge}
              // QR settings links select the link control; ordinary Happ visits still start on routing.
              defaultActiveTab={searchParams.get('happTab') === 'links' ? 'links' : 'routing'}
            />
          ),
        },
        {
          key: 'incy',
          label: (
            <>
              <CompassOutlined /> Incy
            </>
          ),
          children: (
            <>
              <SettingListItem
                paddings="small"
                title={t('pages.settings.subIncyEnableRouting')}
                description={t('pages.settings.subIncyEnableRoutingDesc')}
              >
                <Switch
                  checked={allSetting.subIncyEnableRouting}
                  onChange={(v) => updateSetting({ subIncyEnableRouting: v })}
                />
              </SettingListItem>
              <SettingListItem
                paddings="small"
                title={t('pages.settings.subIncyRoutingRules')}
                badge={remoteSourceBadge(allSetting.subIncyRoutingRules)}
                description={t('pages.settings.subIncyRoutingRulesDesc')}
              >
                <Input.TextArea
                  value={allSetting.subIncyRoutingRules}
                  placeholder="incy://routing/onadd/... or https://.../DEFAULT.JSON"
                  onChange={(e) => updateSetting({ subIncyRoutingRules: e.target.value })}
                />
              </SettingListItem>
            </>
          ),
        },
      ]}
    />
  );

  const content = {
    access: connection,
    formats: <SubscriptionFormatsTab allSetting={allSetting} updateSetting={updateSetting} />,
    profile: <Card className="subscription-settings-card">{profile}</Card>,
    information: <Card className="subscription-settings-card">{information}</Card>,
    apps: <Card className="subscription-settings-card subscription-settings-nested">{apps}</Card>,
    balancers: (
      <Card className="subscription-settings-card subscription-settings-nested">
        <SubscriptionBalancersTab allSetting={allSetting} updateSetting={updateSetting} />
      </Card>
    ),
  };

  return (
    <div className="domain-hub subscription-workspace">
      <header className="domain-hub-header">
        <div className="domain-hub-brand">
          <span className="domain-hub-logo" aria-hidden>
            <LinkOutlined />
          </span>
          <Typography.Title level={3}>{t('pages.settings.subSettings')}</Typography.Title>
        </div>
        {actions}
      </header>
      <nav
        className="domain-service-map subscription-workspace-nav"
        aria-label={t('pages.settings.subSettings')}
      >
        {panels.map(({ key, label, icon }) => (
          <button
            key={key}
            type="button"
            className="domain-service-tile"
            aria-current={activePanel.key === key ? 'page' : undefined}
            onClick={() => selectPanel(key)}
          >
            <span className="domain-service-icon" aria-hidden>
              {icon}
            </span>
            <span>{label}</span>
          </button>
        ))}
      </nav>
      <section className="subscription-workspace-body" aria-label={activePanel.label}>
        {content[activePanel.key]}
      </section>
    </div>
  );
}
