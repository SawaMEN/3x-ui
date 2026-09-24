import { Button, Input, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useFormContext } from 'react-hook-form';

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
  const { t } = useTranslation();
  const { setValue } = useFormContext();

  return (
    <>
      <FormField
        name={['settings', 'tls', 'serverName']}
        label={t('pages.inbounds.form.anyTlsSni')}
        tooltip={t('pages.inbounds.form.anyTlsSniHint')}
      >
        <Input placeholder="example.com" />
      </FormField>

      <FormField
        name={['settings', 'tls', 'certificatePath']}
        label={t('pages.inbounds.form.anyTlsCertificatePath')}
        tooltip={t('pages.inbounds.form.anyTlsCertificatePathHint')}
      >
        <Input placeholder="/root/cert/example.com/fullchain.pem" />
      </FormField>

      <FormField
        name={['settings', 'tls', 'keyPath']}
        label={t('pages.inbounds.form.anyTlsKeyPath')}
        tooltip={t('pages.inbounds.form.anyTlsKeyPathHint')}
      >
        <Input placeholder="/root/cert/example.com/privkey.pem" />
      </FormField>

      <FormField
        name={['settings', 'paddingScheme']}
        label={t('pages.inbounds.form.anyTlsPaddingScheme')}
        tooltip={t('pages.inbounds.form.anyTlsPaddingSchemeHint')}
        extra={
          <Space direction="vertical" size={2} style={{ width: '100%' }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {t('pages.inbounds.form.anyTlsPaddingSchemeLineHint')}
            </Typography.Text>
            <Button
              type="link"
              size="small"
              style={{ paddingInline: 0 }}
              onClick={() => setValue('settings.paddingScheme', DEFAULT_PADDING_SCHEME)}
            >
              {t('pages.inbounds.form.anyTlsRestorePadding')}
            </Button>
          </Space>
        }
        transform={{
          input: (value) =>
            Array.isArray(value) && value.length > 0
              ? value.join('\n')
              : DEFAULT_PADDING_SCHEME.join('\n'),
          output: (value) =>
            String(value ?? '')
              .split(/\r?\n/)
              .map((line) => line.trim())
              .filter(Boolean),
        }}
      >
        <Input.TextArea rows={8} spellCheck={false} />
      </FormField>
    </>
  );
}
