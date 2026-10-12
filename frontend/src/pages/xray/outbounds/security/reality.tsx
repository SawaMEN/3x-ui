import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Select } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';

import { FormField } from '@/components/form/rhf';

import { UTLS_OPTIONS } from '../outbound-form-constants';

export default function RealityForm() {
  const { t } = useTranslation();
  const { setValue } = useFormContext();
  const legacyVerify = useWatch({ name: 'streamSettings.realitySettings.mldsa65Verify' });
  return (
    <>
      <FormField label="SNI" name={['streamSettings', 'realitySettings', 'serverName']}>
        <Input />
      </FormField>
      <FormField label="uTLS" name={['streamSettings', 'realitySettings', 'fingerprint']}>
        <Select options={UTLS_OPTIONS} />
      </FormField>
      <FormField
        label={t('pages.xray.outboundForm.shortId')}
        name={['streamSettings', 'realitySettings', 'shortId']}
      >
        <Input />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.spiderX')}
        name={['streamSettings', 'realitySettings', 'spiderX']}
      >
        <Input />
      </FormField>
      <FormField
        label={t('pages.inbounds.publicKey')}
        name={['streamSettings', 'realitySettings', 'publicKey']}
      >
        <Input.TextArea autoSize={{ minRows: 2 }} />
      </FormField>
      {typeof legacyVerify === 'string' && legacyVerify.trim() && (
        <>
          <Alert
            showIcon
            type="warning"
            title={t('pages.inbounds.form.legacyRealityUnsupported')}
          />
          <FormField
            label={t('pages.inbounds.form.mldsa65Verify')}
            name={['streamSettings', 'realitySettings', 'mldsa65Verify']}
          >
            <Input.TextArea readOnly autoSize={{ minRows: 2 }} />
          </FormField>{' '}
          <Button
            danger
            onClick={() =>
              setValue('streamSettings.realitySettings.mldsa65Verify', '', { shouldDirty: true })
            }
          >
            {t('clear')}
          </Button>
        </>
      )}
    </>
  );
}
