import { useTranslation } from 'react-i18next';
import { InputNumber, Select, Switch } from 'antd';

import { FormField } from '@/components/form/rhf';

export default function MieruFields() {
  const { t } = useTranslation();

  return (
    <>
      <FormField name={['settings', 'protocols']} label={t('pages.inbounds.form.mieruProtocols')}>
        <Select
          mode="multiple"
          options={[
            { value: 'TCP', label: 'TCP' },
            { value: 'UDP', label: 'UDP' },
          ]}
          tokenSeparators={[',']}
        />
      </FormField>

      <FormField
        name={['settings', 'additionalPorts']}
        label={t('pages.inbounds.form.mieruAdditionalPorts')}
        tooltip={t('pages.inbounds.form.mieruAdditionalPortsHint')}
      >
        <Select mode="tags" tokenSeparators={[',']} />
      </FormField>

      <FormField name={['settings', 'mtu']} label={t('pages.inbounds.form.mieruMtu')}>
        <InputNumber min={1280} max={1400} style={{ width: '100%' }} />
      </FormField>

      <FormField name={['settings', 'loggingLevel']} label={t('pages.inbounds.form.mieruLoggingLevel')}>
        <Select
          options={['OFF', 'ERROR', 'WARN', 'INFO', 'DEBUG'].map((value) => ({
            value,
            label: value,
          }))}
        />
      </FormField>

      <FormField
        name={['settings', 'userHintIsMandatory']}
        label={t('pages.inbounds.form.mieruUserHintMandatory')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
    </>
  );
}
