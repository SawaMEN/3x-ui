import { Alert, Collapse, Input, Select } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';

export default function SnellFields() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const version = useWatch({ control, name: 'settings.version' });
  const obfsMode = useWatch({ control, name: 'settings.obfsMode' });

  function setVersion(nextVersion: 5 | 6) {
    setValue('settings.version', nextVersion, { shouldDirty: true, shouldValidate: true });
    setValue('settings.mode', 'default', { shouldDirty: true });
    setValue('settings.obfsMode', 'none', { shouldDirty: true });
    setValue('settings.obfsHost', 'bing.com', { shouldDirty: true });
  }

  const advanced = (
    <>
      <FormField
        name={['settings', 'psk']}
        label="PSK"
        tooltip={t('pages.inbounds.form.snellPskTooltip')}
      >
        <Input.Password
          placeholder={t('pages.inbounds.form.additionalSecretAuto')}
          autoComplete="new-password"
        />
      </FormField>
      {version === 6 ? (
        <>
          <FormField
            name={['settings', 'mode']}
            label={t('pages.inbounds.form.snellMode')}
            tooltip={t('pages.inbounds.form.snellModeTooltip')}
          >
            <Select
              options={[
                { value: 'default', label: 'default' },
                { value: 'unshaped', label: 'unshaped' },
              ]}
            />
          </FormField>
        </>
      ) : (
        <>
          <FormField
            name={['settings', 'obfsMode']}
            label={t('pages.inbounds.form.snellObfs')}
            tooltip={t('pages.inbounds.form.snellObfsTooltip')}
          >
            <Select
              options={[
                { value: 'none', label: 'none' },
                { value: 'http', label: 'http' },
              ]}
            />
          </FormField>
          {obfsMode === 'http' && (
            <>
              <FormField
                name={['settings', 'obfsHost']}
                label={t('pages.inbounds.form.snellObfsHost')}
                tooltip={t('pages.inbounds.form.snellObfsHostTooltip')}
              >
                <Input placeholder="bing.com" />
              </FormField>
            </>
          )}
        </>
      )}
    </>
  );

  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.snellHint')} />
      <FormField
        name={['settings', 'version']}
        label={t('pages.inbounds.form.snellVersion')}
        tooltip={t('pages.inbounds.form.snellVersionTooltip')}
        onAfterChange={(value) => setVersion(value === 5 ? 5 : 6)}
      >
        <Select
          options={[
            { value: 6, label: 'v6' },
            { value: 5, label: 'v5 (v4 client, no QUIC Proxy)' },
          ]}
        />
      </FormField>
      <Collapse
        ghost
        items={[
          {
            key: 'advanced',
            label: t('pages.inbounds.form.snellAdvanced'),
            children: advanced,
          },
        ]}
      />
    </>
  );
}
