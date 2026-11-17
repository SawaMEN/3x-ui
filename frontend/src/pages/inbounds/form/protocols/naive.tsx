import { useTranslation } from 'react-i18next';
import { Input, Select, Switch } from 'antd';
import { FormField } from '@/components/form/rhf';
import { useFormContext, useWatch } from 'react-hook-form';
import SniSearchInput from '../security/SniSearchInput';

export default function NaiveFields() {
  const { t } = useTranslation();
  const { control } = useFormContext();
  const network = useWatch({ control, name: 'settings.network' });

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

      {network === 'udp' && (
        <FormField name={['settings', 'quicCongestionControl']} label="QUIC congestion control">
          <Select
            options={[
              { value: 'bbr', label: 'BBR' },
              { value: 'cubic', label: 'CUBIC' },
              { value: 'reno', label: 'Reno' },
            ]}
          />
        </FormField>
      )}

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
        <SniSearchInput placeholder="example.com" />
      </FormField>
      <FormField
        name={['settings', 'tls', 'certificatePath']}
        label="TLS: путь к сертификату (fullchain.pem)"
      >
        <Input placeholder="/etc/letsencrypt/live/example.com/fullchain.pem" />
      </FormField>
      <FormField
        name={['settings', 'tls', 'keyPath']}
        label="TLS: путь к приватному ключу (privkey.pem)"
      >
        <Input placeholder="/etc/letsencrypt/live/example.com/privkey.pem" />
      </FormField>

      <div style={{ color: 'var(--ant-color-text-secondary)', fontSize: 12, marginTop: 4 }}>
        {t('pages.inbounds.form.naiveHint')}
      </div>
    </>
  );
}
