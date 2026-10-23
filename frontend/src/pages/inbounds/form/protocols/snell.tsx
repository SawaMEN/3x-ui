import { Alert, Button, Input, Select } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';

export default function SnellFields() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const version = useWatch({ control, name: 'settings.version' });
  const obfsMode = useWatch({ control, name: 'settings.obfsMode' });
  function preset(nextVersion: 5 | 6) {
    setValue('settings.version', nextVersion, { shouldDirty: true, shouldValidate: true });
    setValue('settings.mode', 'default', { shouldDirty: true });
    // Prefer the wire-compatible default. HTTP obfuscation is opt-in:
    // it must match every v4 client and is not a TLS or QUIC transport.
    setValue('settings.obfsMode', 'none', { shouldDirty: true });
    setValue('settings.obfsHost', 'bing.com', { shouldDirty: true });
  }
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.snellHint')} />
      <Button onClick={() => preset(6)}>{t('pages.inbounds.form.snellAuto')}</Button>
      <Button onClick={() => preset(5)}>{t('pages.inbounds.form.snellCompatible')}</Button>
      <FormField
        name={['settings', 'version']}
        label={t('pages.inbounds.form.snellVersion')}
        onAfterChange={(v) => preset(v === 5 ? 5 : 6)}
      >
        <Select
          options={[
            { value: 6, label: 'v6' },
            { value: 5, label: 'v5 (v4 client, no QUIC Proxy)' },
          ]}
        />
      </FormField>
      <FormField name={['settings', 'psk']} label="PSK">
        <Input.Password
          placeholder={t('pages.inbounds.form.additionalSecretAuto')}
          autoComplete="new-password"
        />
      </FormField>
      <Alert type="info" showIcon description={t('pages.inbounds.form.snellPskHint')} />
      {version === 6 ? (
        <>
          <FormField name={['settings', 'mode']} label={t('pages.inbounds.form.snellMode')}>
            <Select options={[
              { value: 'default', label: 'default' },
              { value: 'unshaped', label: 'unshaped' },
            ]} />
          </FormField>
          <Alert type="info" description={t('pages.inbounds.form.snellModeHint')} />
        </>
      ) : (
        <>
          <FormField name={['settings', 'obfsMode']} label={t('pages.inbounds.form.snellObfs')}>
            <Select options={[
              { value: 'none', label: 'none' },
              { value: 'http', label: 'http' },
            ]} />
          </FormField>
          {obfsMode === 'http' && (
            <>
              <FormField name={['settings', 'obfsHost']} label={t('pages.inbounds.form.snellObfsHost')}>
                <Input placeholder="bing.com" />
              </FormField>
              <Alert type="warning" showIcon description={t('pages.inbounds.form.snellObfsHint')} />
            </>
          )}
        </>
      )}
    </>
  );
}
