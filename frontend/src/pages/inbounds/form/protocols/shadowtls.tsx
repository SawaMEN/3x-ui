import { Input, InputNumber, Select, Switch } from 'antd';
import { FormField } from '@/components/form/rhf';

export default function ShadowTlsFields() {
  return (
    <>
      <FormField name={['settings', 'handshake', 'server']} label="Handshake server">
        <Input placeholder="cloudflare.com" />
      </FormField>

      <FormField name={['settings', 'handshake', 'serverPort']} label="Handshake port">
        <InputNumber min={1} max={65535} style={{ width: '100%' }} />
      </FormField>

      <FormField name={['settings', 'strictMode']} label="Strict mode" valueProp="checked">
        <Switch />
      </FormField>

      <FormField name={['settings', 'wildcardSni']} label="Wildcard SNI">
        <Select
          options={[
            { label: 'Off', value: 'off' },
            { label: 'Authenticated only', value: 'authed' },
            { label: 'All', value: 'all' },
          ]}
        />
      </FormField>
    </>
  );
}
