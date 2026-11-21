import { DownOutlined, InfoCircleOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

export default function SubAppleRegionHelp({ platform }: { platform: 'ios' | 'macos' }) {
  const { t, i18n } = useTranslation();
  const supportLocale = i18n.language === 'ru-RU' ? 'ru-ru' : 'en-us';

  return (
    <details className="sub-apple-help">
      <summary>
        <InfoCircleOutlined aria-hidden />
        <span>{t('subscription.appleRegion.title')}</span>
        <DownOutlined aria-hidden className="sub-apple-help-chevron" />
      </summary>
      <div className="sub-apple-help-body">
        {platform === 'macos' && <p>{t('subscription.appleRegion.macInstaller')}</p>}
        <p>{t('subscription.appleRegion.before')}</p>
        <ol>
          <li>{t(`subscription.appleRegion.${platform}Open`)}</li>
          <li>{t(`subscription.appleRegion.${platform}Account`)}</li>
          <li>{t('subscription.appleRegion.country')}</li>
          <li>{t('subscription.appleRegion.payment')}</li>
          <li>{t('subscription.appleRegion.reopen')}</li>
        </ol>
        <a
          href={`https://support.apple.com/${supportLocale}/118283`}
          target="_blank"
          rel="noopener noreferrer"
        >
          {t('subscription.appleRegion.appleSupport')}
        </a>
      </div>
    </details>
  );
}
