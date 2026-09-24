import { Input, InputNumber, Select, Switch, Divider } from 'antd';
import { FormField } from '@/components/form/rhf';

const aeadOptions = [
  { value: 'chacha20-poly1305', label: 'ChaCha20-Poly1305' },
  { value: 'aes-128-gcm', label: 'AES-128-GCM' },
  { value: 'none', label: 'None' },
];

const asciiOptions = [
  { value: 'prefer_entropy', label: 'Prefer entropy' },
  { value: 'prefer_ascii', label: 'Prefer ASCII' },
  { value: 'up_ascii_down_entropy', label: 'Up ASCII / Down entropy' },
  { value: 'up_entropy_down_ascii', label: 'Up entropy / Down ASCII' },
];

const httpMaskModes = [
  { value: 'legacy', label: 'Legacy' },
  { value: 'stream', label: 'Stream' },
  { value: 'poll', label: 'Poll' },
  { value: 'auto', label: 'Auto' },
  { value: 'ws', label: 'WebSocket' },
];

const multiplexModes = [
  { value: 'off', label: 'Off' },
  { value: 'auto', label: 'Auto' },
  { value: 'on', label: 'On' },
];

export default function SudokuFields() {
  return (
    <>
      <div
        style={{
          color: 'var(--ant-color-text-secondary)',
          fontSize: 12,
          marginBottom: 10,
        }}
      >
        The panel generates the Sudoku server key automatically. Client keys are generated per
        inbound and preserved with that inbound.
      </div>

      <FormField name={['settings', 'key']} label="Master public key">
        <Input readOnly placeholder="Generated automatically" />
      </FormField>

      <FormField name={['settings', 'fallbackAddress']} label="Fallback address">
        <Input placeholder="127.0.0.1:80" />
      </FormField>

      <Divider plain titlePlacement="start" style={{ margin: '8px 0 12px' }}>
        Transport
      </Divider>

      <FormField name={['settings', 'aead']} label="AEAD">
        <Select options={aeadOptions} />
      </FormField>

      <FormField name={['settings', 'suspiciousAction']} label="Suspicious action">
        <Select
          options={[
            { value: 'fallback', label: 'Fallback' },
            { value: 'silent', label: 'Silent' },
          ]}
        />
      </FormField>

      <FormField name={['settings', 'paddingMin']} label="Padding minimum">
        <InputNumber min={0} max={65535} style={{ width: '100%' }} />
      </FormField>

      <FormField name={['settings', 'paddingMax']} label="Padding maximum">
        <InputNumber min={0} max={65535} style={{ width: '100%' }} />
      </FormField>

      <FormField name={['settings', 'ascii']} label="ASCII mode">
        <Select options={asciiOptions} />
      </FormField>

      <FormField name={['settings', 'customTable']} label="Custom table">
        <Input placeholder="xpxvvpvv" />
      </FormField>

      <FormField name={['settings', 'customTables']} label="Custom table rotation">
        <Select mode="tags" tokenSeparators={[',', ';']} style={{ width: '100%' }} />
      </FormField>

      <FormField
        name={['settings', 'enablePureDownlink']}
        label="Pure Sudoku downlink"
        valueProp="checked"
      >
        <Switch />
      </FormField>

      <FormField name={['settings', 'multiplex']} label="Multiplex">
        <Select options={multiplexModes} />
      </FormField>

      <Divider plain titlePlacement="start" style={{ margin: '8px 0 12px' }}>
        HTTP mask
      </Divider>

      <FormField
        name={['settings', 'httpmask', 'disable']}
        label="Disable HTTP mask"
        valueProp="checked"
      >
        <Switch />
      </FormField>

      <FormField name={['settings', 'httpmask', 'mode']} label="Mode">
        <Select options={httpMaskModes} />
      </FormField>

      <FormField name={['settings', 'httpmask', 'tls']} label="HTTP mask TLS" valueProp="checked">
        <Switch />
      </FormField>

      <FormField name={['settings', 'httpmask', 'host']} label="HTTP mask Host">
        <Input placeholder="www.example.com" />
      </FormField>

      <FormField name={['settings', 'httpmask', 'pathRoot']} label="HTTP mask path root">
        <Input placeholder="abc123" />
      </FormField>

      <FormField name={['settings', 'httpmask', 'multiplex']} label="HTTP mask multiplex">
        <Select options={multiplexModes} />
      </FormField>
    </>
  );
}
