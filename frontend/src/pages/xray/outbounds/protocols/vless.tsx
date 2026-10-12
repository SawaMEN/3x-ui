import { useTranslation } from 'react-i18next';
import { Alert, Button, Input } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';

import { FormField, rhfZodValidate } from '@/components/form/rhf';
import {
  VlessOutboundFormSettingsSchema,
  VmessOutboundFormSettingsSchema,
} from '@/schemas/forms/outbound-form';

export default function VlessFields() {
  const { t } = useTranslation();
  const { setValue } = useFormContext();
  const reverseTag = useWatch({ name: 'settings.reverseTag' });
  return (
    <>
      <FormField
        label="ID"
        name={['settings', 'id']}
        rules={{ validate: rhfZodValidate(VmessOutboundFormSettingsSchema.shape.id) }}
      >
        <Input placeholder="UUID" />
      </FormField>
      <FormField
        label={t('encryption')}
        name={['settings', 'encryption']}
        rules={{ validate: rhfZodValidate(VlessOutboundFormSettingsSchema.shape.encryption) }}
      >
        <Input />
      </FormField>
      {typeof reverseTag === 'string' && reverseTag.trim() && (
        <>
          <Alert type="warning" showIcon title={t('pages.clients.reverseUnsupported')} />
          <FormField label={t('pages.clients.reverseTag')} name={['settings', 'reverseTag']}>
            <Input readOnly placeholder={t('pages.xray.outboundForm.optional')} />
          </FormField>{' '}
          <Button danger onClick={() => setValue('settings.reverseTag', '', { shouldDirty: true })}>
            {t('clear')}
          </Button>
        </>
      )}
    </>
  );
}
