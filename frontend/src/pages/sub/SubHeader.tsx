import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Menu, Popover, Space } from 'antd';
import {
  MoonFilled,
  MoonOutlined,
  SunOutlined,
  TranslationOutlined,
  WifiOutlined,
} from '@ant-design/icons';

import { LanguageManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';

interface SubHeaderProps {
  title: string;
  logoURL?: string;
  sId: string;
  email: string;
  lang: string;
  onLangChange: (lang: string) => void;
}

export default function SubHeader({
  title,
  logoURL,
  sId,
  email,
  lang,
  onLangChange,
}: SubHeaderProps) {
  const { t } = useTranslation();
  const { isDark, isUltra, mode, setThemeMode, lowPower, toggleLowPower } = useTheme();
  const themes = [
    { key: 'light', label: 'Light / Светлая' },
    { key: 'dark', label: 'Dark / Тёмная' },
    { key: 'ultra-dark', label: 'Ultra Dark' },
    { key: 'blue-gray', label: 'Blue Gray' },
    { key: 'colorful', label: 'Colorful' },
    { key: 'cyberpunk', label: 'Cyberpunk' },
  ];

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

  const themeIcon = !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;
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
        <Popover
          placement="bottomRight"
          trigger="click"
          content={
            <Menu
              selectedKeys={[lowPower ? 'dark' : mode]}
              items={themes}
              onClick={({ key }) => {
                if (lowPower && key !== 'dark') toggleLowPower();
                setThemeMode(key as typeof mode);
              }}
            />
          }
        >
          <Button
            shape="circle"
            size="large"
            className="toolbar-btn"
            aria-label={t('menu.theme')}
            title={t('menu.theme')}
            icon={themeIcon}
          />
        </Popover>
        <Popover
          rootClassName={isDark ? 'dark' : 'light'}
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
            icon={<TranslationOutlined />}
          />
        </Popover>
      </div>
    </header>
  );
}
