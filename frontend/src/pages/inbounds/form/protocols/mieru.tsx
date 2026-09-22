import { useTranslation } from 'react-i18next';

export default function MieruFields() {
  const { t } = useTranslation();

  return (
    <div style={{ color: 'var(--ant-color-text-secondary)', fontSize: 12, marginTop: 4 }}>
      {t('pages.inbounds.form.mieruAutoHint')}
    </div>
  );
}
