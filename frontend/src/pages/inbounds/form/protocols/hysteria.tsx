import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Collapse, Form, Input, InputNumber, Select, Switch } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';

import { HeaderMapEditor } from '@/components/form';
import { FormField } from '@/components/form/rhf';

const MASQ_PATH = ['streamSettings', 'hysteriaSettings', 'masquerade'];

export default function HysteriaFields() {
  const { t } = useTranslation();
  const uid = useId();
  const { control, setValue } = useFormContext();
  const masq = useWatch({ control, name: 'streamSettings.hysteriaSettings.masquerade' }) as
    | { type?: string }
    | undefined;
  const masqType = useWatch({
    control,
    name: 'streamSettings.hysteriaSettings.masquerade.type',
  }) as string | undefined;
  const version = useWatch({ control, name: 'streamSettings.hysteriaSettings.version' }) ?? 2;
  const obfs = useWatch({ control, name: 'streamSettings.hysteriaSettings.obfs' });
  const incompatible = useWatch({ control, name: 'streamSettings.hysteriaSettings.masquerade' });
  const set = (name: string, value: unknown) =>
    setValue(name, value, { shouldDirty: true, shouldValidate: true });
  return (
    <>
      <FormField
        label={t('pages.inbounds.form.version')}
        name={['streamSettings', 'hysteriaSettings', 'version']}
        onAfterChange={(v) => {
          set('settings.version', v);
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
        label={t('pages.inbounds.form.udpIdleTimeout')}
        name={['streamSettings', 'hysteriaSettings', 'udpIdleTimeout']}
      >
        <InputNumber min={1} max={4294967295} precision={0} style={{ width: '100%' }} />
      </FormField>

      <Alert
        type="info"
        showIcon
        description={t('pages.inbounds.form.hyNativeHint')}
        style={{ marginBottom: 12 }}
      />
      <Collapse
        items={[
          {
            key: 'native',
            label: t('pages.inbounds.form.hyNativeAdvanced'),
            children: (
              <>
                {(['up', 'down'] as const).map((name) => (
                  <FormField
                    key={name}
                    name={['streamSettings', 'hysteriaSettings', name]}
                    label={t(`pages.inbounds.form.hy${name === 'up' ? 'Up' : 'Down'}`)}
                    tooltip={t('pages.inbounds.form.hyBandwidthHint')}
                  >
                    <InputNumber min={0} precision={0} style={{ width: '100%' }} placeholder="0" />
                  </FormField>
                ))}
                {version === 2 && (
                  <>
                    <FormField
                      name={['streamSettings', 'hysteriaSettings', 'ignoreClientBandwidth']}
                      label={t('pages.inbounds.form.hyIgnoreBandwidth')}
                      valueProp="checked"
                      tooltip={t('pages.inbounds.form.hyIgnoreBandwidthHint')}
                    >
                      <Switch />
                    </FormField>
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
                    <Form.Item label={t('pages.inbounds.form.hyObfs')} htmlFor={`${uid}-obfs`}>
                      <Select
                        id={`${uid}-obfs`}
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
                              ? {
                                  type,
                                  password: typeof obfs === 'object' ? (obfs?.password ?? '') : '',
                                }
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
                        <Input.Password autoComplete="new-password" />
                      </FormField>
                    )}
                    {typeof obfs === 'object' && obfs?.type === 'gecko' && (
                      <>
                        <FormField
                          name={['streamSettings', 'hysteriaSettings', 'obfs', 'minPacketSize']}
                          label={t('pages.inbounds.form.hyPacketMin')}
                        >
                          <InputNumber min={1} max={2048} precision={0} style={{ width: '100%' }} />
                        </FormField>
                        <FormField
                          name={['streamSettings', 'hysteriaSettings', 'obfs', 'maxPacketSize']}
                          label={t('pages.inbounds.form.hyPacketMax')}
                        >
                          <InputNumber min={1} max={2048} precision={0} style={{ width: '100%' }} />
                        </FormField>
                      </>
                    )}
                  </>
                )}
                {version === 1 && (
                  <FormField
                    name={['streamSettings', 'hysteriaSettings', 'obfs']}
                    label={t('pages.inbounds.form.hyObfsPassword')}
                  >
                    <Input.Password />
                  </FormField>
                )}
              </>
            ),
          },
        ]}
        style={{ margin: '12px 0' }}
      />
      {(incompatible?.insecure || incompatible?.xForwarded) && (
        <Alert
          type="warning"
          showIcon
          description={t('pages.inbounds.form.hyUnsupportedMasquerade')}
          action={
            <Button
              onClick={() => {
                set('streamSettings.hysteriaSettings.masquerade.insecure', false);
                set('streamSettings.hysteriaSettings.masquerade.xForwarded', false);
              }}
            >
              {t('clear')}
            </Button>
          }
        />
      )}
      {version === 2 && (
        <>
          <Form.Item htmlFor={`${uid}-masquerade`} label={t('pages.inbounds.form.masquerade')}>
            <Switch
              id={`${uid}-masquerade`}
              checked={!!masq}
              onChange={(checked) =>
                set(
                  'streamSettings.hysteriaSettings.masquerade',
                  checked
                    ? {
                        type: '',
                        dir: '',
                        url: '',
                        rewriteHost: false,
                        insecure: false,
                        content: '',
                        headers: {},
                        statusCode: 0,
                      }
                    : undefined,
                )
              }
            />
          </Form.Item>
          {masq && (
            <>
              <FormField label={t('pages.inbounds.form.type')} name={[...MASQ_PATH, 'type']}>
                <Select
                  options={[
                    { value: '', label: t('pages.inbounds.form.hyMasqueradeDefault') },
                    { value: 'proxy', label: t('pages.inbounds.form.hyMasqueradeProxy') },
                    { value: 'file', label: t('pages.inbounds.form.hyMasqueradeFile') },
                    { value: 'string', label: t('pages.inbounds.form.hyMasqueradeString') },
                  ]}
                />
              </FormField>
              {masqType === 'proxy' && (
                <>
                  <FormField
                    label={t('pages.inbounds.form.upstreamUrl')}
                    name={[...MASQ_PATH, 'url']}
                  >
                    <Input placeholder="https://www.example.com" />
                  </FormField>
                  <FormField
                    label={t('pages.inbounds.form.rewriteHost')}
                    name={[...MASQ_PATH, 'rewriteHost']}
                    valueProp="checked"
                  >
                    <Switch />
                  </FormField>
                </>
              )}
              {masqType === 'file' && (
                <FormField label={t('pages.inbounds.form.directory')} name={[...MASQ_PATH, 'dir']}>
                  <Input placeholder="/var/www/html" />
                </FormField>
              )}
              {masqType === 'string' && (
                <>
                  <FormField
                    label={t('pages.inbounds.form.statusCode')}
                    name={[...MASQ_PATH, 'statusCode']}
                  >
                    <InputNumber min={0} max={599} style={{ width: '100%' }} />
                  </FormField>
                  <FormField label={t('pages.inbounds.form.body')} name={[...MASQ_PATH, 'content']}>
                    <Input.TextArea autoSize={{ minRows: 3 }} />
                  </FormField>
                  <FormField
                    label={t('pages.inbounds.form.headers')}
                    name={[...MASQ_PATH, 'headers']}
                  >
                    <HeaderMapEditor mode="v1" />
                  </FormField>
                </>
              )}
            </>
          )}
        </>
      )}
    </>
  );
}
