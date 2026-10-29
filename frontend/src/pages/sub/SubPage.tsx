import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, ConfigProvider, Layout, Tabs, message } from 'antd';
import type { TabsProps } from 'antd';
import {
  ApiOutlined,
  AppstoreOutlined,
  ClockCircleOutlined,
  CustomerServiceOutlined,
  LinkOutlined,
  UnorderedListOutlined,
} from '@ant-design/icons';

import { ClipboardManager, LanguageManager } from '@/utils';
import { setMessageInstance } from '@/utils/messageBus';
import { useTheme } from '@/hooks/useTheme';
import SubHeader from './SubHeader';
import SubHero from './SubHero';
import SubLinksTab from './SubLinksTab';
import { telegramProfiles } from './telegramProfiles';
import type { TelegramSubscription } from '@/generated/types';
import {
  buildSubApps,
  buildTelemtEndpoint,
  daysUntil,
  detectPlatform,
  resolveSubStatus,
} from './subPageModel';
import './SubPage.css';
import './SubLuna.css';
import LunaBackdrop from './LunaBackdrop';
import { lunaTheme } from './lunaTheme';
import UndergroundBackdrop from './UndergroundBackdrop';
import { undergroundTheme } from './undergroundTheme';
import './SubUnderground.css';

// Keep the first (links) tab eager; load larger, less-used tabs on demand.
const SubAppsTab = lazy(() => import('./SubAppsTab'));
const SubConfigsTab = lazy(() => import('./SubConfigsTab'));
const SubTelegramTab = lazy(() => import('./SubTelegramTab'));

const subData = window.__SUB_PAGE_DATA__ || {};

const sId = subData.sId || '';
const subUrl = subData.subUrl || '';
const subJsonUrl = subData.subJsonUrl || '';
const subClashUrl = subData.subClashUrl || '';
const appearance = subData.appearance?.enabled ? subData.appearance : undefined;
const luna = ['luna', 'whitemax'].includes(subData.appearance?.theme ?? '');
const underground = subData.appearance?.theme === 'underground';
const customTheme = luna || underground;
const subTitle = appearance?.title || subData.subTitle || '';
const subSupportUrl = subData.subSupportUrl || '';
const updateHours = Number(subData.subUpdates || 0);
const announce = subData.announce || '';
const links: string[] = Array.isArray(subData.links) ? subData.links : [];
const linkEmails: string[] = Array.isArray(subData.emails) ? subData.emails : [];
const totalByte = Number(subData.totalByte || 0);
const usedByte =
  Number(subData.usedByte || 0) ||
  Number(subData.downloadByte || 0) + Number(subData.uploadByte || 0);
const expireMs = Number(subData.expire || 0) * 1000;
const clientEmail = [...new Set(linkEmails.filter(Boolean))].join(', ');
const loadedAt = Date.now();

const heroData = {
  status: resolveSubStatus({ enabled: !!subData.enabled, usedByte, totalByte, expireMs }, loadedAt),
  daysLeft: daysUntil(expireMs, loadedAt),
  usedByte,
  totalByte,
  expireMs,
  lastOnlineMs: Number(subData.lastOnline || 0),
  download: subData.download || '0',
  upload: subData.upload || '0',
  used: subData.used || '0',
  total: subData.total || '∞',
  remained: subData.remained || '',
  datepicker: subData.datepicker || 'gregorian',
};

const builtApps = buildSubApps({ subUrl, sId, subTitle });
const apps =
  subData.subFormatMode === 'xray'
    ? {
        android: builtApps.android.filter((app) => app.name !== 'Sing-box'),
        ios: builtApps.ios.filter((app) => app.name !== 'Sing-box'),
      }
    : builtApps;
const initialPlatform = detectPlatform(navigator.userAgent);
const RTL_LANGUAGES = new Set(['fa-IR', 'ar-EG']);

export default function SubPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage('subscription'));
  const telemtEndpoint = buildTelemtEndpoint(window.location.href, sId);
  const [telemtData, setTelemtData] = useState<Partial<TelegramSubscription>>({});
  const [telemtLoading, setTelemtLoading] = useState(() => Boolean(telemtEndpoint));
  const [telemtError, setTelemtError] = useState(() => !telemtEndpoint);

  useEffect(() => {
    if (!telemtEndpoint) return;

    const controller = new AbortController();
    const timeout = window.setTimeout(() => {
      setTelemtLoading(false);
      setTelemtError(true);
      controller.abort();
    }, 8000);

    void fetch(telemtEndpoint, {
      method: 'GET',
      cache: 'no-store',
      credentials: 'same-origin',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(`Telemt subscription request failed with ${response.status}`);
        }
        return (await response.json()) as TelegramSubscription;
      })
      .then((data) => {
        setTelemtData(data);
        setTelemtError(false);
      })
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === 'AbortError')) {
          setTelemtData({});
          setTelemtError(true);
        }
      })
      .finally(() => {
        window.clearTimeout(timeout);
        if (!controller.signal.aborted) {
          setTelemtLoading(false);
        }
      });

    return () => {
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [telemtEndpoint]);

  const onLangChange = useCallback((next: string) => {
    setLang(next);
    LanguageManager.setLanguage(next, 'subscription');
  }, []);

  const copy = useCallback(
    async (value: string, toast?: string) => {
      if (!value) return;
      const ok = await ClipboardManager.copyText(value);
      if (ok) messageApi.success(toast ?? t('copied'));
    },
    [t, messageApi],
  );

  const open = useCallback((url: string) => {
    if (url) window.open(url, '_blank');
  }, []);

  const tabs = useMemo(() => {
    const items: NonNullable<TabsProps['items']> = [];
    if (subUrl || subJsonUrl || subClashUrl) {
      items.push({
        key: 'subscription',
        icon: <LinkOutlined aria-hidden />,
        label: t('subscription.tabLinks'),
        children: (
          <SubLinksTab
            subUrl={subUrl}
            subJsonUrl={subJsonUrl}
            subFormatMode={subData.subFormatMode}
            subClashUrl={subClashUrl}
            onCopy={copy}
          />
        ),
      });
    }
    if (subUrl) {
      items.push({
        key: 'apps',
        icon: <AppstoreOutlined aria-hidden />,
        label: t('subscription.tabApps'),
        children: (
          <Suspense fallback={<div className="sub-muted">{t('loading')}</div>}>
            <SubAppsTab apps={apps} initialPlatform={initialPlatform} onOpen={open} />
          </Suspense>
        ),
      });
    }
    const profiles = telegramProfiles(telemtData, links);
    if (profiles.length > 0 || telemtLoading || telemtError) {
      items.push({
        key: 'telemt',
        icon: <ApiOutlined />,
        label: 'Telegram Proxy',
        children: (
          <Suspense fallback={<div className="sub-muted">{t('loading')}</div>}>
            <SubTelegramTab
              profiles={profiles}
              loading={telemtLoading}
              error={telemtError}
              onCopy={copy}
              onOpen={open}
            />
          </Suspense>
        ),
      });
    }
    if (links.length > 0) {
      items.push({
        key: 'configs',
        icon: <UnorderedListOutlined aria-hidden />,
        label: (
          <>
            {t('subscription.tabConfigs')}
            <span className="sub-tab-count">{links.length}</span>
          </>
        ),
        children: (
          <Suspense fallback={<div className="sub-muted">{t('loading')}</div>}>
            <SubConfigsTab links={links} onCopy={copy} />
          </Suspense>
        ),
      });
    }
    return items;
  }, [t, copy, open, telemtData, telemtLoading, telemtError]);

  const direction = RTL_LANGUAGES.has(lang) ? 'rtl' : 'ltr';
  const pageClass = [
    'subscription-page',
    (customTheme || isDark) && 'is-dark',
    !customTheme && isUltra && 'is-ultra',
    luna && 'sub-luna',
    underground && 'sub-underground',
  ]
    .filter(Boolean)
    .join(' ');

  const subscriptionStyle = {
    ...(appearance && !customTheme ? { '--sub-accent': appearance.accent } : {}),
    colorScheme: customTheme || isDark ? 'dark' : 'light',
  } as React.CSSProperties;

  return (
    <ConfigProvider
      theme={underground ? undergroundTheme() : luna ? lunaTheme() : antdThemeConfig}
      direction={direction}
    >
      {messageContextHolder}
      <Layout className={pageClass} dir={direction} style={subscriptionStyle}>
        {underground ? (
          <UndergroundBackdrop />
        ) : luna ? (
          <LunaBackdrop />
        ) : (
          <div className="sub-aurora" aria-hidden="true">
            <span className="sub-aurora-grid" />
          </div>
        )}
        <Layout.Content className="sub-content">
          <Card className="sub-card">
            <SubHeader
              title={subTitle}
              isDark={customTheme || isDark}
              isLuna={luna}
              logoURL={appearance?.logoURL}
              sId={sId}
              email={clientEmail}
              lang={lang}
              onLangChange={onLangChange}
            />
            {appearance?.description && (
              <p className="sub-description sub-luna-description" dir="auto">
                {appearance.description}
              </p>
            )}
            {(appearance || customTheme) && subUrl && (
              <Button
                type="primary"
                size="large"
                className="sub-connect sub-luna-connect"
                icon={<LinkOutlined />}
                onClick={() => void copy(subUrl)}
              >
                {t('pages.inbounds.copyLink')}
              </Button>
            )}
            {announce && <Alert type="info" showIcon title={announce} className="sub-announce" />}
            <SubHero {...heroData} lang={lang} />
            {tabs.length > 0 && <Tabs className="sub-tabs" tabBarGutter={24} items={tabs} />}
            {(updateHours > 0 || subSupportUrl) && (
              <footer className="sub-footer">
                {updateHours > 0 && (
                  <span>
                    <ClockCircleOutlined aria-hidden />
                    {t('subscription.updateInterval', { hours: updateHours })}
                  </span>
                )}
                {subSupportUrl && (
                  <a href={subSupportUrl} target="_blank" rel="noopener noreferrer">
                    <CustomerServiceOutlined aria-hidden />
                    {t('subscription.support')}
                  </a>
                )}
              </footer>
            )}
          </Card>
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
