import { useState } from 'react';
import {
  Alert,
  Button,
  Collapse,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  message,
} from 'antd';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import { HttpUtil } from '@/utils';
import SniSearchInput from '../security/SniSearchInput';
import {
  OPENFLUX_TRANSPORTS,
  suggestOpenFluxTransport,
  validOpenFluxChannelURL,
  type OpenFluxTransportType,
} from '@/lib/openflux';

export function InstallVPNButton({ protocol }: { protocol: 'fptn' | 'vetroff' }) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      loading={busy}
      onClick={async () => {
        setBusy(true);
        try {
          const result = await HttpUtil.post(
            `/panel/api/server/externalvpn/update/${protocol}`, undefined,
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
        protocol: protocol === 'fptn' ? 'FPTN client' : 'VetroffProxy',
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
        <SniSearchInput placeholder="www.bing.com" showCertificateAssistant={false} />
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
  const { control, getValues } = useFormContext();
  const transports = useFieldArray({ control, name: 'settings.transports' });
  const values = (useWatch({ control, name: 'settings.transports' }) ?? []) as {
    type: OpenFluxTransportType;
    url?: string;
    priority?: number;
  }[];
  const [adding, setAdding] = useState(false);
  const [addDirect, setAddDirect] = useState(false);
  const [channelType, setChannelType] = useState<OpenFluxTransportType>('vyandex');
  const [channelURL, setChannelURL] = useState('');
  const used = new Set(values.map((entry) => entry.type));
  const available = OPENFLUX_TRANSPORTS.filter((type) => !used.has(type));
  const openAdd = (withDirect: boolean) => {
    const next = ['vyandex', ...available].find(
      (type) =>
        available.includes(type as OpenFluxTransportType) && (!withDirect || type !== 'direct'),
    );
    if (!next) return;
    setChannelType(next as OpenFluxTransportType);
    setChannelURL('');
    setAddDirect(withDirect);
    setAdding(true);
  };
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.openfluxHint')} />
      <Space wrap>
        <Button
          onClick={() => {
            const direct = values.find((entry) => entry.type === 'direct');
            // Context is part of the encryption identity. A channel preset
            // must never reset it or rotate the client secret.
            transports.replace([direct ?? { type: 'direct', url: '', priority: 100 }]);
          }}
        >
          {t('pages.inbounds.form.openfluxDirect')}
        </Button>
        <Button
          disabled={!available.some((type) => type !== 'direct')}
          onClick={() => openAdd(true)}
        >
          {t('pages.inbounds.form.openfluxFailover')}
        </Button>
      </Space>
      <Alert type="info" showIcon description={t('pages.inbounds.form.openfluxComponentHint')} />
      {transports.fields.map((field, i) => (
        <Space key={field.id} wrap align="start">
          <FormField
            name={['settings', 'transports', i, 'type']}
            label={t('pages.inbounds.form.openfluxTransport')}
            rules={{
              validate: (value) =>
                values.every((entry, index) => index === i || entry.type !== value) ||
                t('pages.inbounds.form.openfluxDuplicate'),
            }}
          >
            <Select
              style={{ width: 150 }}
              options={OPENFLUX_TRANSPORTS.map((value) => ({
                value,
                label: value,
                disabled: used.has(value) && values[i]?.type !== value,
              }))}
            />
          </FormField>
          <FormField
            name={['settings', 'transports', i, 'priority']}
            label={t('pages.inbounds.form.openfluxPriority')}
            extra={t('pages.inbounds.form.openfluxPriorityHint')}
          >
            <InputNumber min={0} max={1000} />
          </FormField>
          {values[i]?.type !== 'direct' && (
            <FormField
              name={['settings', 'transports', i, 'url']}
              label="URL"
              rules={{
                validate: (value) =>
                  validOpenFluxChannelURL(String(value ?? '')) ||
                  t('pages.inbounds.form.openfluxURLRequired'),
              }}
            >
              <Input style={{ width: 250 }} placeholder="https://…" />
            </FormField>
          )}
          <Button disabled={transports.fields.length === 1} onClick={() => transports.remove(i)}>
            {t('remove')}
          </Button>
        </Space>
      ))}
      <Button disabled={available.length === 0} onClick={() => openAdd(false)}>
        {t('pages.inbounds.form.openfluxAdd')}
      </Button>
      <Collapse
        ghost
        items={[
          {
            key: 'context',
            label: t('pages.inbounds.advancedTitle'),
            children: (
              <FormField
                name={['settings', 'context']}
                label={t('pages.inbounds.form.openfluxContext')}
                extra={t('pages.inbounds.form.openfluxContextHint')}
              >
                <Input maxLength={4096} />
              </FormField>
            ),
          },
        ]}
      />
      <Modal
        title={t('pages.inbounds.form.openfluxAdd')}
        open={adding}
        onCancel={() => setAdding(false)}
        okButtonProps={{
          disabled: channelType !== 'direct' && !validOpenFluxChannelURL(channelURL.trim()),
        }}
        onOk={() => {
          const current = (getValues('settings.transports') ?? []) as typeof values;
          if (current.some((entry) => entry.type === channelType)) return;
          const additions = [];
          if (addDirect && !current.some((entry) => entry.type === 'direct'))
            additions.push({ type: 'direct', url: '', priority: 100 });
          additions.push({
            type: channelType,
            url: channelType === 'direct' ? '' : channelURL.trim(),
            priority: channelType === 'direct' ? 100 : 50,
          });
          transports.append(additions);
          setAdding(false);
        }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Select
            aria-label={t('pages.inbounds.form.openfluxTransport')}
            style={{ width: '100%' }}
            value={channelType}
            options={available
              .filter((type) => !addDirect || type !== 'direct')
              .map((type) => ({ value: type, label: type }))}
            onChange={setChannelType}
          />
          {channelType !== 'direct' && (
            <Input
              aria-label={t('pages.inbounds.form.openfluxChannelURL')}
              placeholder="https://docs.yandex.ru/edit/d/…"
              value={channelURL}
              onChange={(event) => {
                setChannelURL(event.target.value);
                const inferred = suggestOpenFluxTransport(event.target.value.trim());
                if (inferred && available.includes(inferred)) setChannelType(inferred);
              }}
            />
          )}
          <Alert type="info" showIcon description={t('pages.inbounds.form.openfluxChannelHint')} />
        </Space>
      </Modal>
    </>
  );
}
