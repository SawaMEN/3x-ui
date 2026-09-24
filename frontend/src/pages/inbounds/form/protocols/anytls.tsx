import { Input, Space, Typography } from 'antd';
import { FormField } from '@/components/form/rhf';

const DEFAULT_PADDING_SCHEME = [
  'stop=8',
  '0=30-30',
  '1=100-400',
  '2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000',
  '3=9-9,500-1000',
  '4=500-1000',
  '5=500-1000',
  '6=500-1000',
  '7=500-1000',
];

export default function AnyTlsFields() {
  return (
    <>
      <FormField name={['settings', 'tls', 'serverName']} label="SNI">
        <Input placeholder="example.com" />
      </FormField>

      <FormField
        name={['settings', 'tls', 'certificatePath']}
        label="Certificate path"
      >
        <Input placeholder="/root/cert/example.com/fullchain.pem" />
      </FormField>

      <FormField name={['settings', 'tls', 'keyPath']} label="Private key path">
        <Input placeholder="/root/cert/example.com/privkey.pem" />
      </FormField>

      <FormField
        name={['settings', 'paddingScheme']}
        label="Padding scheme"
        transform={{
          input: (value) =>
            Array.isArray(value) && value.length > 0 ? value.join('\n') : DEFAULT_PADDING_SCHEME.join('\n'),
          output: (value) =>
            String(value ?? '')
              .split(/\r?\n/)
              .map((line) => line.trim())
              .filter(Boolean),
        }}
      >
        <Input.TextArea rows={8} spellCheck={false} />
      </FormField>

      <Space direction="vertical" size={2} style={{ width: '100%' }}>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          AnyTLS требует TLS. Если пути сертификата оставить пустыми, панель
          использует сертификат HTTPS панели при генерации sing-box.
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Строки padding scheme вводятся по одной на строку.
        </Typography.Text>
      </Space>
    </>
  );
}
