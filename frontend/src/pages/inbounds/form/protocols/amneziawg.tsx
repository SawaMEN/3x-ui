import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Collapse,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useFormContext, useWatch } from 'react-hook-form';
import { awgVersion, generateAwgPreset, type AwgVersion } from '@/lib/xray/amneziawg-obfuscation';

import { FormField } from '@/components/form/rhf';

interface AmneziawgFieldsProps {
  awgPubKey: string;
  regenInboundAwg: () => void;
}

export default function AmneziawgFields({ awgPubKey, regenInboundAwg }: AmneziawgFieldsProps) {
  const { t } = useTranslation();
  const { control, getValues, setValue } = useFormContext();
  const server = useWatch({ control, name: 'settings.server' });
  const version = awgVersion(server ?? {});
  const applyPreset = (next: AwgVersion) => {
    const apply = () => {
      for (const [key, value] of Object.entries(generateAwgPreset(next)))
        setValue(`settings.server.${key}`, value, { shouldDirty: true });
    };
    if (getValues('settings.clients')?.length) {
      Modal.confirm({
        title: t('pages.xray.amneziawg.changePresetTitle'),
        content: t('pages.xray.amneziawg.changePresetWarning'),
        onOk: apply,
      });
    } else apply();
  };
  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        title={t('pages.xray.amneziawg.compatibility', { version })}
        description={t('pages.xray.amneziawg.compatibilityHint')}
      />
      <Form.Item label={t('pages.xray.amneziawg.preset')}>
        <Select
          value={version}
          options={(['3.1', '3.0', '2.0', '1.5', '1.0'] as const).map((v) => ({
            value: v,
            label: `AWG ${v}`,
          }))}
          onChange={applyPreset}
        />
      </Form.Item>
      <Form.Item label={t('pages.xray.amneziawg.privateKey')}>
        <Space.Compact block>
          <FormField name={['settings', 'server', 'privateKey']} noStyle>
            <Input style={{ width: 'calc(100% - 32px)' }} />
          </FormField>
          <Button
            aria-label={t('regenerate')}
            icon={<ReloadOutlined />}
            onClick={regenInboundAwg}
          />
        </Space.Compact>
      </Form.Item>
      <Form.Item label={t('pages.xray.amneziawg.publicKey')}>
        <Input value={awgPubKey} disabled />
      </Form.Item>
      <FormField
        name={['settings', 'server', 'subnetIp']}
        label={t('pages.xray.amneziawg.subnetIp')}
      >
        <Input placeholder="10.8.1.0" />
      </FormField>
      <FormField
        name={['settings', 'server', 'subnetCidr']}
        label={t('pages.xray.amneziawg.subnetCidr')}
      >
        <InputNumber min={1} max={32} style={{ width: '100%' }} />
      </FormField>
      <FormField name={['settings', 'server', 'mtu']} label={t('pages.xray.amneziawg.mtu')}>
        <InputNumber min={1} style={{ width: '100%' }} />
      </FormField>
      <FormField
        name={['settings', 'server', 'primaryDns']}
        label={t('pages.xray.amneziawg.primaryDns')}
      >
        <Input placeholder="8.8.8.8" />
      </FormField>
      <FormField
        name={['settings', 'server', 'secondaryDns']}
        label={t('pages.xray.amneziawg.secondaryDns')}
      >
        <Input placeholder="8.8.4.4" />
      </FormField>
      <FormField
        name={['settings', 'server', 'externalInterface']}
        label={t('pages.xray.amneziawg.externalInterface')}
        extra={t('pages.xray.amneziawg.externalInterfaceHint')}
      >
        <Input placeholder="eth0" />
      </FormField>
      <FormField
        name={['settings', 'server', 'ipv6Enabled']}
        label={t('pages.xray.amneziawg.ipv6Enabled')}
        valueProp="checked"
      >
        <Switch />
      </FormField>
      <FormField
        name={['settings', 'server', 'ipv6Subnet']}
        label={t('pages.xray.amneziawg.ipv6Subnet')}
        extra={t('pages.xray.amneziawg.ipv6SubnetHint')}
      >
        <Input placeholder="fd86:ea04:1115::/64" />
      </FormField>
      <FormField
        name={['settings', 'server', 'ipv6ExternalInterface']}
        label={t('pages.xray.amneziawg.ipv6ExternalInterface')}
        extra={t('pages.xray.amneziawg.ipv6ExternalInterfaceHint')}
      >
        <Input placeholder="eth0" />
      </FormField>
      <Form.Item label={t('pages.xray.amneziawg.obfuscation')}>
        <Button icon={<ReloadOutlined />} onClick={() => applyPreset(version)}>
          {t('pages.xray.amneziawg.regenerateObfuscation')}
        </Button>
      </Form.Item>
      <Collapse
        items={[
          {
            key: 'advanced',
            label: t('pages.xray.amneziawg.advancedSettings'),
            forceRender: true,
            children: (
              <>
                <FormField name={['settings', 'server', 'jc']} label={t('pages.xray.amneziawg.jc')}>
                  <InputNumber min={0} style={{ width: '100%' }} />
                </FormField>
                <FormField
                  name={['settings', 'server', 'jmin']}
                  label={t('pages.xray.amneziawg.jmin')}
                >
                  <InputNumber min={0} style={{ width: '100%' }} />
                </FormField>
                <FormField
                  name={['settings', 'server', 'jmax']}
                  label={t('pages.xray.amneziawg.jmax')}
                >
                  <InputNumber min={0} style={{ width: '100%' }} />
                </FormField>
                <FormField name={['settings', 'server', 's1']} label={t('pages.xray.amneziawg.s1')}>
                  <InputNumber min={0} style={{ width: '100%' }} />
                </FormField>
                <FormField name={['settings', 'server', 's2']} label={t('pages.xray.amneziawg.s2')}>
                  <InputNumber min={0} style={{ width: '100%' }} />
                </FormField>
                <FormField name={['settings', 'server', 's3']} label={t('pages.xray.amneziawg.s3')}>
                  <InputNumber min={0} max={64} style={{ width: '100%' }} />
                </FormField>
                <FormField name={['settings', 'server', 's4']} label={t('pages.xray.amneziawg.s4')}>
                  <InputNumber min={0} max={32} style={{ width: '100%' }} />
                </FormField>
                <FormField
                  name={['settings', 'server', 'h1']}
                  label={t('pages.xray.amneziawg.h1')}
                  extra={t('pages.xray.amneziawg.hHint')}
                >
                  <Input placeholder="1 or 100-800" />
                </FormField>
                <FormField name={['settings', 'server', 'h2']} label={t('pages.xray.amneziawg.h2')}>
                  <Input placeholder="2 or 100-800" />
                </FormField>
                <FormField name={['settings', 'server', 'h3']} label={t('pages.xray.amneziawg.h3')}>
                  <Input placeholder="3 or 100-800" />
                </FormField>
                <FormField name={['settings', 'server', 'h4']} label={t('pages.xray.amneziawg.h4')}>
                  <Input placeholder="4 or 100-800" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'i1']}
                  label={t('pages.xray.amneziawg.i1')}
                  extra={t('pages.xray.amneziawg.i1Hint')}
                >
                  <Input placeholder="<r 64>" />
                </FormField>
                <FormField name={['settings', 'server', 'i2']} label={t('pages.xray.amneziawg.i2')}>
                  <Input placeholder="<r 64>" />
                </FormField>
                <FormField name={['settings', 'server', 'i3']} label={t('pages.xray.amneziawg.i3')}>
                  <Input placeholder="<r 64>" />
                </FormField>
                <FormField name={['settings', 'server', 'i4']} label={t('pages.xray.amneziawg.i4')}>
                  <Input placeholder="<r 64>" />
                </FormField>
                <FormField name={['settings', 'server', 'i5']} label={t('pages.xray.amneziawg.i5')}>
                  <Input placeholder="<r 64>" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'headerProtectionKey']}
                  label={t('pages.xray.amneziawg.headerProtectionKey')}
                  extra={t('pages.xray.amneziawg.headerProtectionKeyHint')}
                >
                  <Input />
                </FormField>
                <FormField
                  name={['settings', 'server', 'contentPaddingAddition']}
                  label={t('pages.xray.amneziawg.contentPaddingAddition')}
                  extra={t('pages.xray.amneziawg.contentPaddingAdditionHint')}
                >
                  <Input placeholder="8-64" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'rekeyAfterTime']}
                  label={t('pages.xray.amneziawg.rekeyAfterTime')}
                  extra={t('pages.xray.amneziawg.timingRangeHint')}
                >
                  <Input placeholder="100-160" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'rekeyTimeout']}
                  label={t('pages.xray.amneziawg.rekeyTimeout')}
                >
                  <Input placeholder="3-10" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'rejectAfterTime']}
                  label={t('pages.xray.amneziawg.rejectAfterTime')}
                >
                  <Input placeholder="190-250" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'keepaliveTimeout']}
                  label={t('pages.xray.amneziawg.keepaliveTimeout')}
                >
                  <Input placeholder="8-20" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'maxHandshakeAttempts']}
                  label={t('pages.xray.amneziawg.maxHandshakeAttempts')}
                  extra={t('pages.xray.amneziawg.maxHandshakeAttemptsHint')}
                >
                  <Input placeholder="15-50" />
                </FormField>
                <FormField
                  name={['settings', 'server', 'randomTrailers']}
                  label={t('pages.xray.amneziawg.randomTrailers')}
                  extra={t('pages.xray.amneziawg.randomTrailersHint')}
                  valueProp="checked"
                >
                  <Switch />
                </FormField>
                <FormField
                  name={['settings', 'server', 'disableCookies']}
                  label={t('pages.xray.amneziawg.disableCookies')}
                  extra={t('pages.xray.amneziawg.disableCookiesHint')}
                  valueProp="checked"
                >
                  <Switch />
                </FormField>
              </>
            ),
          },
        ]}
      />
    </>
  );
}
