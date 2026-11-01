import { useTranslation } from 'react-i18next';
import { InputNumber } from 'antd';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { KcpStreamSettingsSchema } from '@/schemas/protocols/stream/kcp';

export default function KcpForm() {
  const { t } = useTranslation();
  return (
    <>
      {(
        [
          'mtu',
          'tti',
          'uplinkCapacity',
          'downlinkCapacity',
          'cwndMultiplier',
          'maxSendingWindow',
        ] as const
      ).map((field) => (
        <FormField
          key={field}
          name={['streamSettings', 'kcpSettings', field]}
          label={t(`pages.inbounds.form.${field}`)}
          rules={{ validate: rhfZodValidate(KcpStreamSettingsSchema.shape[field]) }}
        >
          <InputNumber
            min={field === 'uplinkCapacity' || field === 'downlinkCapacity' ? 0 : 1}
            style={{ width: '100%' }}
          />
        </FormField>
      ))}
    </>
  );
}
