import { Alert, Button, Collapse, Input, InputNumber, Select, Space } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import SniSearchInput from '../security/SniSearchInput';
import { MasqueInboundSettingsSchema } from '@/schemas/protocols/inbound/masque';

export default function MasqueFields() {
  const { t } = useTranslation();
  const { setValue, control } = useFormContext();
  const versions = (useWatch({ control, name: 'settings.version' }) ?? [3, 2, 1]) as number[];
  const certificate = useWatch({ control, name: 'settings.tls.certificatePath' });
  const key = useWatch({ control, name: 'settings.tls.keyPath' });
  const automatic = !certificate && !key;
  const hasHTTP2Options = versions.includes(2) || versions.includes(3);
  const advancedLabels: Record<string, string> = {
    udp_timeout: 'UDP timeout',
    idle_timeout: t('pages.inbounds.form.masqueIdleTimeout'),
    keep_alive_period: t('pages.inbounds.form.masqueKeepAlive'),
    stream_receive_window: t('pages.inbounds.form.masqueStreamWindow'),
    connection_receive_window: t('pages.inbounds.form.masqueConnectionWindow'),
    bind_interface: t('pages.inbounds.form.masqueInterface'),
  };
  const set = (name: string, value: unknown) =>
    setValue(name, value, { shouldDirty: true, shouldValidate: true });
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.masqueHint')} />
      <Space wrap style={{ margin: '12px 0' }}>
        <Button
          onClick={() => {
            const defaults = MasqueInboundSettingsSchema.parse({});
            for (const name of [
              'version',
              'path',
              'address',
              'advertiseRoutes',
              'mtu',
              'advanced',
            ] as const)
              set(`settings.${name}`, defaults[name]);
          }}
        >
          {t('pages.inbounds.form.masqueAuto')}
        </Button>
        <Button onClick={() => set('settings.version', [3])}>
          {t('pages.inbounds.form.masqueQUIC')}
        </Button>
        <Button onClick={() => set('settings.version', [2, 1])}>
          {t('pages.inbounds.form.masqueTCP')}
        </Button>
      </Space>
      <FormField name={['settings', 'version']} label={t('pages.inbounds.form.masqueVersion')}>
        <Select
          mode="multiple"
          options={[
            { value: 3, label: 'HTTP/3 (QUIC / UDP)' },
            { value: 2, label: 'HTTP/2 (TLS / TCP)' },
            { value: 1, label: 'HTTP/1.1 (TLS / TCP)' },
          ]}
        />
      </FormField>
      <Alert
        type="info"
        showIcon
        description={t(
          versions.includes(3)
            ? 'pages.inbounds.form.masqueUDPNotice'
            : 'pages.inbounds.form.masqueTCPNotice',
        )}
        style={{ marginBottom: 12 }}
      />
      <FormField name={['settings', 'tls', 'serverName']} label="SNI">
        <SniSearchInput placeholder="example.com" />
      </FormField>
      <Alert
        type="success"
        showIcon
        description={t(
          automatic
            ? 'pages.inbounds.form.masqueCertificateAuto'
            : 'pages.inbounds.form.masqueCertificateCustom',
        )}
      />
      {!automatic && (
        <Button
          style={{ margin: '12px 0' }}
          onClick={() => {
            set('settings.tls.certificatePath', '');
            set('settings.tls.keyPath', '');
          }}
        >
          {t('pages.inbounds.form.masqueUsePanelCertificate')}
        </Button>
      )}
      <Collapse
        style={{ marginTop: 12 }}
        defaultActiveKey={automatic ? [] : ['tls']}
        items={[
          {
            key: 'tls',
            label: t('pages.inbounds.form.masqueCustomCertificate'),
            children: (
              <>
                <FormField
                  name={['settings', 'tls', 'certificatePath']}
                  label={t('pages.inbounds.form.masqueCertificate')}
                >
                  <Input placeholder="/path/fullchain.pem" />
                </FormField>
                <FormField
                  name={['settings', 'tls', 'keyPath']}
                  label={t('pages.inbounds.form.masqueKey')}
                >
                  <Input placeholder="/path/privkey.pem" />
                </FormField>
              </>
            ),
          },
          {
            key: 'network',
            label: t('pages.inbounds.form.masqueAdvanced'),
            children: (
              <>
                <FormField name={['settings', 'path']} label={t('pages.inbounds.form.masquePath')}>
                  <Input />
                </FormField>
                <FormField
                  name={['settings', 'address']}
                  label={t('pages.inbounds.form.masqueAddress')}
                >
                  <Select mode="tags" tokenSeparators={[',', ' ']} />
                </FormField>
                <FormField
                  name={['settings', 'advertiseRoutes']}
                  label={t('pages.inbounds.form.masqueRoutes')}
                  tooltip={t('pages.inbounds.form.masqueRoutesHint')}
                >
                  <Select mode="tags" tokenSeparators={[',', ' ']} placeholder="0.0.0.0/0, ::/0" />
                </FormField>
                <FormField name={['settings', 'mtu']} label="MTU">
                  <InputNumber min={1280} max={65535} style={{ width: '100%' }} />
                </FormField>
                {[
                  'udp_timeout',
                  'idle_timeout',
                  'keep_alive_period',
                  'stream_receive_window',
                  'connection_receive_window',
                  'bind_interface',
                ].map((name) => (
                  <FormField
                    key={name}
                    name={['settings', 'advanced', name]}
                    label={advancedLabels[name] || name}
                    tooltip={
                      name.includes('receive_window')
                        ? t('pages.inbounds.form.masqueWindowHint')
                        : undefined
                    }
                  >
                    <Input
                      disabled={
                        name !== 'udp_timeout' && name !== 'bind_interface' && !hasHTTP2Options
                      }
                      placeholder={
                        name.includes('receive_window')
                          ? '64 MB'
                          : name.includes('timeout') || name.includes('period')
                            ? '30s'
                            : undefined
                      }
                    />
                  </FormField>
                ))}
                {['max_concurrent_streams', 'initial_packet_size', 'udp_nat_max'].map((name) => (
                  <FormField key={name} name={['settings', 'advanced', name]} label={name}>
                    <InputNumber
                      min={name === 'initial_packet_size' ? 1200 : 0}
                      disabled={
                        name === 'initial_packet_size'
                          ? !versions.includes(3)
                          : name === 'max_concurrent_streams' && !hasHTTP2Options
                      }
                      style={{ width: '100%' }}
                    />
                  </FormField>
                ))}
                <FormField
                  name={['settings', 'advanced', 'disable_path_mtu_discovery']}
                  label={t('pages.inbounds.form.masqueMTUDiscovery')}
                >
                  <Select
                    allowClear
                    disabled={!versions.includes(3)}
                    options={[
                      { value: true, label: t('yes') },
                      { value: false, label: t('no') },
                    ]}
                  />
                </FormField>
              </>
            ),
          },
        ]}
      />
    </>
  );
}
