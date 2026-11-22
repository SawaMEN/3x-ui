import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Menu, Popover, Space } from 'antd';
import { TranslationOutlined } from '@ant-design/icons';

import { LanguageManager } from '@/utils';

interface SubHeaderProps {
  isDark: boolean;
  isLuna?: boolean;
  title: string;
  description?: string;
  logoURL?: string;
  sId: string;
  email: string;
  lang: string;
  onLangChange: (lang: string) => void;
}

export default function SubHeader({
  title,
  description,
  isDark,
  isLuna = false,
  logoURL,
  sId,
  email,
  lang,
  onLangChange,
}: SubHeaderProps) {
  const { t } = useTranslation();
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

  const [failedLogo, setFailedLogo] = useState('');

  return (
    <header className="sub-header">
      <div className="sub-brand">
        {logoURL && failedLogo !== logoURL && (
          <img
            className="sub-brand-logo"
            src={logoURL}
            alt=""
            referrerPolicy="no-referrer"
            onError={() => setFailedLogo(logoURL)}
          />
        )}
        <div className="sub-brand-text">
          <h1 className="sub-brand-title" dir="auto">
            {title || t('subscription.title')}
          </h1>
          {(sId || email) && (
            <div className="sub-brand-id">
              <bdi>{[sId, email].filter(Boolean).join(' · ')}</bdi>
            </div>
          )}
          {description && (
            <p className="sub-description" dir="auto">
              {description}
            </p>
          )}
        </div>
      </div>
      <div className="sub-toolbar">
        <Popover
          rootClassName={isLuna ? 'sub-luna-popover dark' : isDark ? 'dark' : 'light'}
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
            className="toolbar-btn"
            aria-label={t('pages.settings.language')}
            icon={<TranslationOutlined aria-hidden />}
          />
        </Popover>
      </div>
    </header>
  );
}
