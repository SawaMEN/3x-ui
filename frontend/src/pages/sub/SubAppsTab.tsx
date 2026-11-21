import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Segmented } from 'antd';
import { DownloadOutlined, PlusOutlined } from '@ant-design/icons';

import happIcon from './app-icons/happ.svg';
import { APP_PLATFORMS, buildSubApps, type AppPlatform } from './subPageModel';

interface SubAppsTabProps {
  subUrl: string;
  profileName: string;
  initialPlatform: AppPlatform;
}

const PLATFORM_OPTIONS = (Object.keys(APP_PLATFORMS) as AppPlatform[]).map((value) => ({
  value,
  label: APP_PLATFORMS[value],
}));

export default function SubAppsTab({ subUrl, profileName, initialPlatform }: SubAppsTabProps) {
  const { t } = useTranslation();
  const [platform, setPlatform] = useState(initialPlatform);

  return (
    <div className="sub-apps">
      <Segmented<AppPlatform>
        block
        value={platform}
        onChange={setPlatform}
        options={PLATFORM_OPTIONS}
        aria-label={t('subscription.tabApps')}
      />
      <ul className="sub-app-list">
        {buildSubApps(platform, subUrl, profileName).map(({ name, url, downloadUrl }) => (
          <li key={name} className="sub-row sub-app-row">
            <div className="sub-app-identity">
              <span className="sub-app-mark" aria-hidden="true">
                {name === 'Happ' ? (
                  <span className="sub-app-glyph" style={{ maskImage: `url("${happIcon}")` }} />
                ) : (
                  'H'
                )}
              </span>
              <span className="sub-app-name">{name}</span>
            </div>
            <div className="sub-app-actions">
              <Button
                href={downloadUrl}
                target="_blank"
                rel="noopener noreferrer"
                icon={<DownloadOutlined aria-hidden />}
                aria-label={`${t('download')} · ${name}`}
              >
                {t('download')}
              </Button>
              <Button
                type="primary"
                href={url}
                icon={<PlusOutlined aria-hidden />}
                aria-label={`${t('add')} · ${name}`}
              >
                {t('add')}
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
