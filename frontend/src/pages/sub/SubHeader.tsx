import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Dropdown, Menu, Popover, Space } from 'antd';
import { MoonOutlined, SunOutlined, TranslationOutlined, WifiOutlined } from '@ant-design/icons';

import { LanguageManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface SubHeaderProps {
  whiteMax: boolean;
  whiteMaxDark?: boolean;
  onWhiteMaxDarkChange?: () => void;
  onWhiteMaxChange: (value: boolean) => void;
  title: string;
  logoURL?: string;
  sId: string;
  email: string;
  lang: string;
  onLangChange: (lang: string) => void;
}

export default function SubHeader({
  title,
  whiteMax,
  whiteMaxDark = true,
  onWhiteMaxDarkChange,
  onWhiteMaxChange,
  logoURL,
  sId,
  email,
  lang,
  onLangChange,
}: SubHeaderProps) {
  const { t } = useTranslation();
  const { mode, isDark, setThemeMode, lowPower, toggleLowPower } = useTheme();
  const langMenuItems = useMemo(
    () =>
      (LanguageManager.supportedLanguages as { value: string; name: string; icon: string }[]).map(
        (l) => ({
          key: l.value,
          label: (
            <Space size={8}>
              <span aria-hidden="true">{l.icon}</span>
              <span>{l.name}</span>
            </Space>
          ),
        }),
      ),
    [],
  );

  const effectiveDark = whiteMax ? whiteMaxDark : isDark;
  const themeIcon = !effectiveDark ? <SunOutlined /> : <MoonOutlined />;
  const [failedLogo, setFailedLogo] = useState('');
  const initial = Array.from(title)[0]?.toUpperCase();

  return (
    <header className="sub-header">
      <div className="sub-brand">
        <span className="sub-brand-mark" aria-hidden="true">
          {logoURL && failedLogo !== logoURL ? (
            <img
              src={logoURL}
              alt=""
              referrerPolicy="no-referrer"
              onError={() => setFailedLogo(logoURL)}
            />
          ) : (
            (initial ?? <WifiOutlined />)
          )}
        </span>
        <div className="sub-brand-text">
          <div className="sub-brand-title" dir="auto">
            {title || t('subscription.title')}
          </div>
          <div className="sub-brand-id">
            <bdi>{email ? `${sId} - ${email}` : sId}</bdi>
          </div>
        </div>
      </div>
      <div className="sub-toolbar">
        {whiteMax && (
          <Button
            shape="circle"
            size="large"
            className="toolbar-btn"
            aria-label={t('subscription.whiteMaxToggle')}
            title={t('subscription.whiteMaxToggle')}
            onClick={onWhiteMaxDarkChange}
            icon={whiteMaxDark ? <SunOutlined /> : <MoonOutlined />}
          />
        )}
        <Dropdown
          menu={{
            selectedKeys: [whiteMax ? 'whitemax' : mode],
            items: ['light', 'dark', 'ultra-dark', 'colorful', 'blue-gray', 'cyberpunk']
              .map((key) => ({
                key,
                label: t(`subscription.themes.${key}`),
              }))
              .concat([{ key: 'whitemax', label: 'WhiteMax' }]),
            onClick: ({ key }) => {
              if (lowPower && key !== 'dark' && key !== 'whitemax') toggleLowPower();
              onWhiteMaxChange(key === 'whitemax');
              if (key !== 'whitemax') setThemeMode(key as typeof mode);
            },
          }}
          trigger={['click']}
        >
          <Button
            shape="circle"
            size="large"
            className="toolbar-btn"
            aria-label={t('menu.theme')}
            title={t('menu.theme')}
            icon={themeIcon}
          />
        </Dropdown>
        <Popover
          rootClassName={effectiveDark ? 'dark' : 'light'}
          placement="bottomRight"
          trigger="click"
          styles={{ content: { padding: 4 } }}
          content={
            <Menu
              mode="vertical"
              selectable
              selectedKeys={[lang]}
              items={langMenuItems}
              onClick={({ key }) => onLangChange(key)}
              style={{ border: 'none', minWidth: 160 }}
            />
          }
        >
          <Button
            shape="circle"
            size="large"
            className="toolbar-btn"
            aria-label={t('pages.settings.language')}
            icon={<TranslationOutlined aria-hidden />}
          />
        </Popover>
      </div>
    </header>
  );
}
