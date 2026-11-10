import { useTranslation } from 'react-i18next';
import { Alert, Button, Skeleton } from 'antd';
import { ApiOutlined, CopyOutlined } from '@ant-design/icons';

import type { TelegramProxyProfile } from '@/generated/types';
import SubQrButton from './SubQrButton';

interface SubTelegramTabProps {
  profiles: TelegramProxyProfile[];
  loading: boolean;
  error: boolean;
  onCopy: (value: string) => void;
  onOpen: (value: string) => void;
}

export default function SubTelegramTab({
  profiles,
  loading,
  error,
  onCopy,
  onOpen,
}: SubTelegramTabProps) {
  const { t } = useTranslation();
  return (
    <div className="sub-rows">
      {error && <Alert type="warning" showIcon title={t('subscription.telegramUnavailable')} />}
      {loading && profiles.length === 0 && (
        <Skeleton active paragraph={{ rows: 2 }} title={false} />
      )}
      {profiles.map((profile) => {
        const title = profile.name || profile.host;
        const endpoint = `${profile.host}${profile.port ? `:${profile.port}` : ''}`;
        return (
          <section className="sub-telegram-profile" key={profile.link}>
            <div className="sub-row-main">
              <div className="sub-row-title" dir="auto">
                {title}
              </div>
              <div className="sub-muted">
                <bdi>{endpoint}</bdi>
              </div>
            </div>
            <div className="sub-telegram-actions">
              <Button
                type="primary"
                icon={<ApiOutlined aria-hidden />}
                onClick={() => onOpen(profile.link)}
              >
                {t('subscription.openTelegram')}
              </Button>
              <Button
                icon={<CopyOutlined />}
                onClick={() => onCopy(profile.link)}
                aria-label={t('copy')}
              />
              <SubQrButton value={profile.link} label={title} onCopy={onCopy} />
            </div>
          </section>
        );
      })}
    </div>
  );
}
