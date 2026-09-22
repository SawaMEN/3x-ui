import { useTranslation } from 'react-i18next';
import { InputNumber, Select, Switch } from 'antd';

import { FormField } from '@/components/form/rhf';

const multiplexingOptions = [
  'MULTIPLEXING_OFF',
  'MULTIPLEXING_LOW',
  'MULTIPLEXING_MIDDLE',
  'MULTIPLEXING_HIGH',
].map((value) => ({ value, label: value }));

const handshakeOptions = ['HANDSHAKE_STANDARD', 'HANDSHAKE_NO_WAIT'].map((value) => ({
  value,
  label: value,
}));

export default function MieruFields() {
  const { t } = useTranslation();

  return (
    <>
      <FormField
        name={['settings', 'tcpPorts']}
        label={t('pages.inbounds.form.mieruTcpPorts')}
        tooltip={t('pages.inbounds.form.mieruPortRangesHint')}
      >
        <Select
          mode="tags"
          tokenSeparators={[',', ';']}
          placeholder="2012-2022"
          style={{ width: '100%' }}
        />
      </FormField>

      <FormField
        name={['settings', 'udpPorts']}
        label={t('pages.inbounds.form.mieruUdpPorts')}
        tooltip={t('pages.inbounds.form.mieruPortRangesHint')}
      >
        <Select
          mode="tags"
          tokenSeparators={[',', ';']}
          placeholder="2023-2033"
          style={{ width: '100%' }}
        />
      </FormField>

      <FormField name={['settings', 'multiplexing']} label={t('pages.inbounds.form.mieruMultiplexing')}>
        <Select options={multiplexingOptions} />
      </FormField>

      <FormField name={['settings', 'handshakeMode']} label={t('pages.inbounds.form.mieruHandshakeMode')}>
        <Select options={handshakeOptions} />
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
