import { useTranslation } from 'react-i18next';
import { Input, Select } from 'antd';
import { FormField } from '@/components/form/rhf';

export default function NaiveFields() {
  const { t } = useTranslation();

  return (
    <>
      <FormField name={['settings', 'network']} label={t('pages.inbounds.form.naiveNetwork')}>
        <Select
          options={[
            { value: 'tcp', label: t('pages.inbounds.form.naiveNetworkTcp') },
            { value: 'udp', label: t('pages.inbounds.form.naiveNetworkUdp') },
          ]}
        />
      </FormField>

      <FormField name={['settings', 'tls', 'serverName']} label="SNI">
        <Input placeholder="example.com" />
      </FormField>

      <FormField
        name={['settings', 'tls', 'certificatePath']}
        label={t('pages.inbounds.form.naiveCertificatePath')}
      >
        <Input placeholder="/root/cert/example.com/fullchain.pem" />
      </FormField>

      <FormField
        name={['settings', 'tls', 'keyPath']}
        label={t('pages.inbounds.form.naiveKeyPath')}
      >
        <Input placeholder="/root/cert/example.com/privkey.pem" />
      </FormField>

      <div style={{ color: 'var(--ant-color-text-secondary)', fontSize: 12, marginTop: 4 }}>
        {t('pages.inbounds.form.naiveHint')}
      </div>
    </>
  );
}
