import { Alert, Form, Input, InputNumber } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';

export function PingtunnelFields() {
  const { t } = useTranslation();
  const { control } = useFormContext();
  const key = useWatch({ control, name: 'settings.key' }) as number | undefined;
  const secret = useWatch({ control, name: 'settings.encryptKey' }) as string | undefined;
  const address = useWatch({ control, name: 'shareAddr' }) as string | undefined;
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.pingtunnelHint')} />
      <FormField name={['settings', 'key']} label={t('pages.inbounds.form.pingtunnelKey')}>
        <InputNumber min={0} max={2147483647} />
      </FormField>
      <FormField
        name={['settings', 'encryptKey']}
        label={t('pages.inbounds.form.pingtunnelSecret')}
      >
        <Input.Password autoComplete="new-password" />
      </FormField>
      {key && secret && (
        <Form.Item label={t('pages.inbounds.form.pingtunnelClientConfig')}>
          <Input.TextArea
            readOnly
            autoSize
            value={JSON.stringify(
              {
                type: 'client',
                listen: '127.0.0.1:1080',
                server: address || 'PUBLIC_SERVER_IP',
                sock5: 1,
                key,
                encrypt: 'chacha20',
                encrypt_key: secret,
              },
              null,
              2,
            )}
          />
        </Form.Item>
      )}
    </>
  );
}

export function TrustTunnelFields() {
  const { t } = useTranslation();
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.trusttunnelHint')} />
      <FormField
        name={['settings', 'hostname']}
        label={t('pages.inbounds.form.trusttunnelHostname')}
      >
        <Input placeholder="vpn.example.com" />
      </FormField>
      <FormField
        name={['settings', 'certificate']}
        label={t('pages.inbounds.form.trusttunnelCert')}
      >
        <Input />
      </FormField>
      <FormField name={['settings', 'privateKey']} label={t('pages.inbounds.form.trusttunnelKey')}>
        <Input.Password />
      </FormField>
    </>
  );
}
