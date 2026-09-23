import { useTranslation } from 'react-i18next';
import { Input } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function PsiphonFields() {
  const { t } = useTranslation();

  return (
    <>
      <FormField
        name={['settings', 'serverEntry']}
        label={t('pages.inbounds.form.psiphonServerEntry')}
        tooltip={t('pages.inbounds.form.psiphonServerEntryHint')}
      >
        <Input.TextArea autoSize={{ minRows: 6, maxRows: 10 }} placeholder="302030203020..." />
      </FormField>

      <div style={{ color: 'var(--ant-color-text-secondary)', fontSize: 12, marginTop: 4 }}>
        {t('pages.inbounds.form.psiphonAutoHint')}
      </div>
    </>
  );
}
