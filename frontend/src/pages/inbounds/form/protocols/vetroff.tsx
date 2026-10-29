import { Alert, Input } from 'antd';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';

export function VetroffFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.vetroffHint')} />
      <FormField name={['settings', 'hostname']} label="SNI">
        <Input placeholder="vpn.example.com" />
      </FormField>
      <FormField
        name={['settings', 'certificate']}
        label={t('pages.inbounds.form.trusttunnelCert')}
      >
        <Input placeholder="/etc/ssl/vetroff/fullchain.pem" />
      </FormField>
      <FormField name={['settings', 'privateKey']} label={t('pages.inbounds.form.trusttunnelKey')}>
        <Input placeholder="/etc/ssl/vetroff/privkey.pem" />
      </FormField>
    </>
  );
}
