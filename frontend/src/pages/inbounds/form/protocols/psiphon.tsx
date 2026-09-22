import { useTranslation } from 'react-i18next';
import { Input, Select } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function PsiphonFields() {
  const { t } = useTranslation();

  return (
    <>
      <FormField
        name={['settings', 'serverAddress']}
        label={t('pages.inbounds.form.psiphonServerAddress')}
        tooltip={t('pages.inbounds.form.psiphonServerAddressHint')}
      >
        <Input placeholder="203.0.113.10" />
      </FormField>

      <FormField
        name={['settings', 'tunnelProtocol']}
        label={t('pages.inbounds.form.psiphonTunnelProtocol')}
      >
        <Select
          options={['OSSH', 'SSH', 'TLS-OSSH', 'QUIC-OSSH'].map((value) => ({
            value,
            label: value,
          }))}
        />
      </FormField>

      <FormField
        name={['settings', 'serverEntry']}
        label={t('pages.inbounds.form.psiphonServerEntry')}
        tooltip={t('pages.inbounds.form.psiphonServerEntryHint')}
      >
        <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} />
      </FormField>

      <FormField
        name={['settings', 'additionalArguments']}
        label={t('pages.inbounds.form.psiphonAdditionalArguments')}
      >
        <Select mode="tags" tokenSeparators={[' ', ',']} />
      </FormField>
    </>
  );
}
