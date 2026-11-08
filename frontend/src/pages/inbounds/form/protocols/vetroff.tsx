import { useEffect } from 'react';
import { Alert, Button, Collapse, Input, InputNumber, Select, Space, Switch } from 'antd';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import {
  VETROFF_TRANSPORTS,
  VetroffOptionsSchema,
  VetroffUserPolicySchema,
} from '@/schemas/protocols/inbound/vetroff';

const prefix = 'settings.vetroff';
export function VetroffFields() {
  const { t } = useTranslation();
  const { control, getValues, setValue } = useFormContext();
  const options = useWatch({ control, name: prefix });
  const nodes = useFieldArray({ control, name: `${prefix}.nodes` });
  const policies = useFieldArray({ control, name: `${prefix}.userPolicies` });
  const set = (key: string, value: unknown) =>
    setValue(`${prefix}.${key}`, value, { shouldDirty: true });
  const label = (key: string) => t(`pages.inbounds.form.vetroffOptions.${key.split('.').at(-1)}`);
  useEffect(() => {
    if (getValues(prefix)) return;
    const defaults = VetroffOptionsSchema.parse({});
    defaults.autoTLS = !getValues('settings.certificate');
    setValue(prefix, defaults);
  }, [getValues, setValue]);
  const text = (key: string, placeholder?: string) => (
    <FormField key={key} name={`${prefix}.${key}`} label={label(key)}>
      <Input placeholder={placeholder} />
    </FormField>
  );
  const number = (key: string, min: number, max: number, step = 1) => (
    <FormField key={key} name={`${prefix}.${key}`} label={label(key)}>
      <InputNumber min={min} max={max} step={step} />
    </FormField>
  );
  const toggle = (key: string) => (
    <FormField key={key} name={`${prefix}.${key}`} label={label(key)} valueProp="checked">
      <Switch />
    </FormField>
  );
  const select = (key: string, values: readonly string[]) => (
    <FormField key={key} name={`${prefix}.${key}`} label={label(key)}>
      <Select options={values.map((value) => ({ value, label: value }))} />
    </FormField>
  );
  function preset(transport: string) {
    const current = VetroffOptionsSchema.parse({});
    setValue(prefix, { ...current, transport }, { shouldDirty: true });
  }
  const userPolicy = (path: string) => (
    <>
      {['maxRequests', 'maxTunnels', 'requestsPerSecond', 'burst', 'quotaBytes'].map((key) => (
        <FormField key={key} name={`${path}.${key}`} label={label(key)}>
          <InputNumber
            min={key === 'quotaBytes' ? 0 : key === 'requestsPerSecond' ? 0.1 : 1}
            step={key === 'requestsPerSecond' ? 0.1 : 1}
            max={
              key === 'quotaBytes'
                ? Number.MAX_SAFE_INTEGER
                : key === 'maxRequests' || key === 'maxTunnels'
                  ? 4096
                  : key === 'burst'
                    ? 20000
                    : 10000
            }
          />
        </FormField>
      ))}
    </>
  );
  return (
    <>
      <Alert type="info" showIcon description={t('pages.inbounds.form.vetroffHint')} />
      <Space wrap style={{ margin: '12px 0' }}>
        <Button onClick={() => preset('h2-connect')}>{label('automatic')}</Button>
        <Button onClick={() => preset('h3-connect')}>HTTP/3</Button>
        <Button onClick={() => preset('xhttp-stream-up')}>XHTTP</Button>
      </Space>
      <FormField
        name={`${prefix}.transport`}
        label={label('transport')}
        onAfterChange={(value) => {
          if (String(value).startsWith('h3-')) {
            set('http3', true);
            if (getValues(`${prefix}.tlsBackend`) === 'nss') set('tlsBackend', 'auto');
          }
        }}
      >
        <Select options={VETROFF_TRANSPORTS.map((value) => ({ value, label: value }))} />
      </FormField>
      <FormField name={`${prefix}.autoTLS`} label={label('autoTLS')} valueProp="checked">
        <Switch />
      </FormField>
      <FormField name="settings.hostname" label="SNI">
        <Input placeholder={label('automatic')} />
      </FormField>
      {!options?.autoTLS && (
        <>
          <FormField name="settings.certificate" label={t('pages.inbounds.form.trusttunnelCert')}>
            <Input placeholder="/etc/ssl/vetroff/fullchain.pem" />
          </FormField>
          <FormField name="settings.privateKey" label={t('pages.inbounds.form.trusttunnelKey')}>
            <Input placeholder="/etc/ssl/vetroff/privkey.pem" />
          </FormField>
        </>
      )}
      <Collapse
        ghost
        items={[
          {
            key: 'server',
            label: label('server'),
            children: (
              <>
                {toggle('http3')}
                {toggle('enableUDP')}
                {text('h3Listen', '0.0.0.0:8443')}
                {number('maxConnections', 1, 65536)}
                {number('maxTunnels', 1, 65536)}
                {text('xhttpPath', '/.well-known/vetroff-xhttp/')}
                {text('xhttpHost')}
                {text('webRoot')}
                {toggle('allowPrivate')}
                {text('egressProxy', 'socks5://127.0.0.1:1080')}
                {text('egressDNS', '1.1.1.1:53')}
              </>
            ),
          },
          {
            key: 'profile',
            label: label('profile'),
            children: (
              <>
                {select('tlsBackend', ['auto', 'go', 'nss'])}
                {select('protocolPolicy', ['builtin', 'strict'])}
                {toggle('autoPin')}
                {text('pin', 'sha256/...')}
                {toggle('padding')}
                {text('publicServer', 'https://vpn.example.com:8443')}
                <FormField name={`${prefix}.fallbackServers`} label={label('fallbackServers')}>
                  <Select mode="tags" tokenSeparators={[',', '\n']} />
                </FormField>
                {select('balance', ['failover', 'round-robin', 'least-active', 'adaptive'])}
                {select('pipeline', ['auto', '4', '8'])}
                {text('connectTimeout', '15s')}
                {text('attemptTimeout', '5s')}
              </>
            ),
          },
          {
            key: 'bond',
            label: 'Bond / discovery',
            children: (
              <>
                <FormField
                  name={`${prefix}.bond`}
                  label={label('bond')}
                  valueProp="checked"
                  onAfterChange={(value) => {
                    if (value) {
                      set('bondHub', true);
                      set('discover', true);
                      set('protocolPolicy', 'builtin');
                      set('padding', false);
                      set('balance', 'failover');
                      set('fallbackServers', []);
                    } else set('discover', false);
                  }}
                >
                  <Switch />
                </FormField>
                {toggle('discover')}
                {toggle('bondHub')}
                {text('bondUpstream')}
                {text('bondCA')}
                {text('bondSNI')}
                {text('bondPin')}
                <Alert type="info" description={label('clusterAuto')} />
                {nodes.fields.map((row, index) => (
                  <div key={row.id}>
                    {text(`nodes.${index}.url`, 'https://node.example.com')}
                    {number(`nodes.${index}.weight`, 1, 100)}
                    {text(`nodes.${index}.sni`)}
                    {text(`nodes.${index}.pin`)}
                    {toggle(`nodes.${index}.disabled`)}
                    <Button onClick={() => nodes.remove(index)}>{t('delete')}</Button>
                  </div>
                ))}
                <Button
                  onClick={() =>
                    nodes.append({ url: '', weight: 100, sni: '', pin: '', disabled: false })
                  }
                  disabled={nodes.fields.length >= 8}
                >
                  {label('addNode')}
                </Button>
              </>
            ),
          },
          {
            key: 'access',
            label: label('access'),
            children: (
              <>
                {number('attemptsPerSecond', 0.1, 10000, 0.1)}
                {number('attemptBurst', 1, 20000)}
                {number('accessMaxRequests', 1, 65536)}
                {userPolicy(`${prefix}.userPolicy`)}
                <Alert type="info" description={label('nativeQuota')} />
                {policies.fields.map((row, index) => (
                  <div key={row.id}>
                    <FormField
                      name={`${prefix}.userPolicies.${index}.email`}
                      label={t('pages.clients.email')}
                    >
                      <Input />
                    </FormField>
                    {userPolicy(`${prefix}.userPolicies.${index}`)}
                    <Button onClick={() => policies.remove(index)}>{t('delete')}</Button>
                  </div>
                ))}
                <Button onClick={() => policies.append(VetroffUserPolicySchema.parse({}))}>
                  {label('addPolicy')}
                </Button>
              </>
            ),
          },
        ]}
      />
    </>
  );
}
