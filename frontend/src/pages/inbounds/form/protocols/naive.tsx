import { useTranslation } from 'react-i18next';
import { Input, Select, Switch } from 'antd';
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

      <FormField
        name={['settings', 'shareLinkFormat']}
        label={t('pages.inbounds.form.naiveShareLinkFormat')}
        tooltip={t('pages.inbounds.form.naiveShareLinkFormatHint')}
        valueProp="checked"
        transform={{
          input: (value) => value === 'hiddify',
          output: (value) => (value ? 'hiddify' : 'standard'),
        }}
      >
        <Switch
          checkedChildren={t('pages.inbounds.form.naiveShareLinkFormatHiddify')}
          unCheckedChildren={t('pages.inbounds.form.naiveShareLinkFormatStandard')}
        />
      </FormField>

      <FormField name={['settings', 'tls', 'serverName']} label="SNI">
        <Input placeholder="example.com" />
      </FormField>

      <div style={{ color: 'var(--ant-color-text-secondary)', fontSize: 12, marginTop: 4 }}>
        {t('pages.inbounds.form.naiveHint')}
      </div>
    </>
  );
}
