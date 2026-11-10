import { Form, Input, InputNumber, Select } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';

// Masquerade and UDP association timeouts are server settings. Keep them out
// of the outbound editor: a native Hysteria client cannot apply either option.
export default function HysteriaForm() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const version = useWatch({ control, name: 'streamSettings.hysteriaSettings.version' }) ?? 2;
  const obfs = useWatch({ control, name: 'streamSettings.hysteriaSettings.obfs' });
  const set = (name: string, value: unknown) =>
    setValue(name, value, { shouldDirty: true, shouldValidate: true });
  return (
    <>
      <FormField
        name={['streamSettings', 'hysteriaSettings', 'version']}
        label={t('pages.inbounds.form.version')}
        onAfterChange={(value) => {
          set('settings.version', value);
          set('streamSettings.hysteriaSettings.obfs', undefined);
        }}
      >
        <Select
          options={[
            { value: 2, label: 'Hysteria 2' },
            { value: 1, label: 'Hysteria 1' },
          ]}
        />
      </FormField>
      <FormField
        name={['streamSettings', 'hysteriaSettings', 'auth']}
        label={t('pages.xray.outboundForm.authPassword')}
      >
        <Input.Password />
      </FormField>
      {(['up', 'down'] as const).map((name) => (
        <FormField
          key={name}
          name={['streamSettings', 'hysteriaSettings', name]}
          label={t(`pages.inbounds.form.hy${name === 'up' ? 'Up' : 'Down'}`)}
          tooltip={t('pages.inbounds.form.hyBandwidthHint')}
        >
          <InputNumber min={0} precision={0} style={{ width: '100%' }} />
        </FormField>
      ))}
      {version === 2 ? (
        <>
          <FormField
            name={['streamSettings', 'hysteriaSettings', 'bbrProfile']}
            label={t('pages.inbounds.form.hyBbrProfile')}
          >
            <Select
              allowClear
              options={['conservative', 'standard', 'aggressive'].map((value) => ({
                value,
                label: value,
              }))}
            />
          </FormField>
          <Form.Item label={t('pages.inbounds.form.hyObfs')}>
            <Select
              value={typeof obfs === 'object' ? obfs?.type : ''}
              options={[
                { value: '', label: t('none') },
                { value: 'salamander', label: 'Salamander' },
                { value: 'gecko', label: 'Gecko' },
              ]}
              onChange={(type) =>
                set(
                  'streamSettings.hysteriaSettings.obfs',
                  type
                    ? { type, password: typeof obfs === 'object' ? (obfs?.password ?? '') : '' }
                    : undefined,
                )
              }
            />
          </Form.Item>
          {typeof obfs === 'object' && obfs?.type && (
            <FormField
              name={['streamSettings', 'hysteriaSettings', 'obfs', 'password']}
              label={t('pages.inbounds.form.hyObfsPassword')}
            >
              <Input.Password />
            </FormField>
          )}
          {typeof obfs === 'object' &&
            obfs?.type === 'gecko' &&
            (['minPacketSize', 'maxPacketSize'] as const).map((name) => (
              <FormField
                key={name}
                name={['streamSettings', 'hysteriaSettings', 'obfs', name]}
                label={t(`pages.inbounds.form.hyPacket${name === 'minPacketSize' ? 'Min' : 'Max'}`)}
              >
                <InputNumber min={1} max={2048} precision={0} style={{ width: '100%' }} />
              </FormField>
            ))}
        </>
      ) : (
        <FormField
          name={['streamSettings', 'hysteriaSettings', 'obfs']}
          label={t('pages.inbounds.form.hyObfsPassword')}
        >
          <Input.Password />
        </FormField>
      )}
    </>
  );
}
