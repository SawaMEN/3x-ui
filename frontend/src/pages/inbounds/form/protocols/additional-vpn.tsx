import { useState } from 'react';
import { Alert, Button, Collapse, Input, InputNumber, Select, Space, Switch, message } from 'antd';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import { HttpUtil } from '@/utils';
import SniSearchInput from '../security/SniSearchInput';

export function InstallVPNButton({ protocol }: { protocol: 'fptn' | 'openflux' | 'vetroff' }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      loading={busy}
      onClick={async () => {
        setBusy(true);
        try {
          const result = await HttpUtil.post(
            `/panel/api/server/externalvpn/update/${protocol}`,
            undefined,
            { timeout: 600000 },
          );
          if (result?.success) message.success(t('pages.inbounds.form.vpnInstalled'));
          else message.error(result?.msg || t('pages.inbounds.form.vpnInstallFailed'));
        } catch (error) {
          message.error(
            error instanceof Error ? error.message : t('pages.inbounds.form.vpnInstallFailed'),
          );
        } finally {
          setBusy(false);
        }
      }}
    >
      {t('pages.inbounds.form.vpnInstall', {
        protocol:
          protocol === 'fptn'
            ? 'FPTN client'
            : protocol === 'vetroff'
              ? 'VetroffProxy'
              : 'OpenFlux',
      })}
    </Button>
  );
}

export function FptnFields() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const adsFilter = useWatch({ control, name: 'settings.adsFilter' });
  function preset(mobile: boolean) {
    for (const [key, value] of Object.entries({
      mtu: mobile ? 1280 : 1400,
      maxSessions: 3,
      detectProbing: true,
      spamFilter: true,
      allowedSni: 'www.bing.com',
    })) {
      setValue(`settings.${key}`, value, { shouldDirty: true });
    }
  }
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.fptnHint')} />
      <Space wrap>
        <Button onClick={() => preset(false)}>{t('pages.inbounds.form.vpnAuto')}</Button>
        <Button onClick={() => preset(true)}>{t('pages.inbounds.form.vpnMobile')}</Button>
      </Space>
      <FormField name={['settings', 'mtu']} label="MTU">
        <InputNumber min={576} max={9000} />
      </FormField>
      <FormField name={['settings', 'maxSessions']} label={t('pages.inbounds.form.fptnSessions')}>
        <InputNumber min={1} max={1000} />
      </FormField>
      <FormField name={['settings', 'bandwidth']} label={t('pages.inbounds.form.fptnBandwidth')}>
        <InputNumber min={0} max={2000} />
      </FormField>
      <FormField
        name={['settings', 'detectProbing']}
        label={t('pages.inbounds.form.fptnProbing')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
      <FormField name={['settings', 'allowedSni']} label="SNI">
        <SniSearchInput placeholder="www.bing.com" />
      </FormField>
      <Collapse
        ghost
        items={[
          {
            key: 'fptn-tls',
            label: t('pages.inbounds.advancedTitle'),
            children: (
              <>
                <Alert
                  showIcon
                  type="info"
                  description="FPTN requires an RSA TLS certificate matching the hostname. Use a certificate/key file pair or a PEM pair; leave both pairs empty for the server-managed certificate."
                />
                <FormField name={['settings', 'hostname']} label="FPTN TLS hostname (SNI)">
                  <SniSearchInput placeholder="vpn.example.com" />
                </FormField>
                <FormField name={['settings', 'certificate']} label="RSA certificate file">
                  <Input placeholder="/etc/ssl/vpn/fullchain.pem" />
                </FormField>
                <FormField name={['settings', 'privateKey']} label="RSA private key file">
                  <Input placeholder="/etc/ssl/vpn/privkey.pem" />
                </FormField>
                <FormField name={['settings', 'certificatePEM']} label="RSA certificate (PEM)">
                  <Input.TextArea rows={3} />
                </FormField>
                <FormField name={['settings', 'privateKeyPEM']} label="RSA private key (PEM)">
                  <Input.TextArea rows={3} />
                </FormField>
                <FormField name={['settings', 'metricsKey']} label="Metrics access key">
                  <Input.Password autoComplete="new-password" />
                </FormField>
                <FormField name={['settings', 'image']} label="FPTN container image">
                  <Input placeholder="fptnvpn/fptn-vpn-server:0.4.4" />
                </FormField>
              </>
            ),
          },
        ]}
      />
      <FormField
        name={['settings', 'adsFilter']}
        label={t('pages.inbounds.form.fptnAds')}
        extra={t('pages.inbounds.form.fptnAdsUnsupported')}
        valueProp="checked"
      >
        <Switch disabled={adsFilter !== true} />
      </FormField>
      <FormField
        name={['settings', 'torrentFilter']}
        label={t('pages.inbounds.form.fptnTorrent')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
      <FormField
        name={['settings', 'spamFilter']}
        label={t('pages.inbounds.form.fptnSpam')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
    </>
  );
}

export function OpenFluxFields() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const transports = useFieldArray({ control, name: 'settings.transports' });
  const values = useWatch({ control, name: 'settings.transports' }) as
    | { type: string }[]
    | undefined;
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.openfluxHint')} />
      <Space wrap>
        <Button
          onClick={() => {
            setValue('settings.context', '', { shouldDirty: true });
            transports.replace([{ type: 'direct', url: '', priority: 100 }]);
          }}
        >
          {t('pages.inbounds.form.openfluxDirect')}
        </Button>
        <Button
          onClick={() => {
            transports.replace([
              { type: 'direct', url: '', priority: 100 },
              { type: 'yandex', url: '', priority: 50 },
            ]);
          }}
        >
          {t('pages.inbounds.form.openfluxFailover')}
        </Button>
        <InstallVPNButton protocol="openflux" />
      </Space>
      <FormField name={['settings', 'context']} label="OpenFlux context">
        <Input.TextArea rows={2} />
      </FormField>
      {transports.fields.map((field, i) => (
        <Space key={field.id} wrap align="start">
          <FormField
            name={['settings', 'transports', i, 'type']}
            label={t('pages.inbounds.form.openfluxTransport')}
          >
            <Select
              style={{ width: 150 }}
              options={['direct', 'yandex', 'vyandex', 'boards', 'mailru'].map((value) => ({
                value,
                label: value,
              }))}
            />
          </FormField>
          <FormField
            name={['settings', 'transports', i, 'priority']}
            label={t('pages.inbounds.form.openfluxPriority')}
          >
            <InputNumber min={0} max={1000} />
          </FormField>
          {values?.[i]?.type !== 'direct' && (
            <FormField name={['settings', 'transports', i, 'url']} label="URL">
              <Input style={{ width: 250 }} placeholder="https://…" />
            </FormField>
          )}
          <Button disabled={transports.fields.length === 1} onClick={() => transports.remove(i)}>
            {t('remove')}
          </Button>
        </Space>
      ))}
      <Button
        disabled={transports.fields.length >= 6}
        onClick={() => transports.append({ type: 'yandex', url: '', priority: 50 })}
      >
        {t('pages.inbounds.form.openfluxAdd')}
      </Button>
    </>
  );
}
