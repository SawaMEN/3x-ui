import { Collapse, Input, InputNumber, Select, Switch, Typography } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import OpenFluxChannels from '@/components/openflux/OpenFluxChannels';
import OpenFluxMAXClientSetup from '@/components/openflux/OpenFluxMAXClientSetup';
import type { OpenFluxTransport } from '@/lib/openflux';

export default function OpenFluxFields({ inboundId = 0 }: { inboundId?: number }) {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const settings = useWatch({ control, name: 'settings' }) ?? {};
  const nodeId = useWatch({ control, name: 'nodeId' }) ?? 0;
  const values = (settings.transports ?? []) as OpenFluxTransport[];
  const max = values.find((entry) => entry.type === 'oneme');
  const secret =
    settings.clients?.find((client: { enable?: boolean }) => client.enable !== false)?.password ??
    '';
  return (
    <div className="openflux-setup">
      <OpenFluxChannels
        nodeId={nodeId}
        value={values}
        onChange={(next) =>
          setValue('settings.transports', next, { shouldDirty: true, shouldValidate: true })
        }
      />
      <Typography.Text type="secondary">
        {t('pages.inbounds.form.openfluxSessionNote')}
      </Typography.Text>
      {max && (
        <Collapse
          items={[
            {
              key: 'client',
              label: t('pages.inbounds.form.openfluxMaxClientSetup'),
              children: (
                <OpenFluxMAXClientSetup
                  secret={secret}
                  context={settings.context ?? ''}
                  serverID={max.uid ?? ''}
                  serverToken={max.token ?? ''}
                />
              ),
            },
          ]}
        />
      )}
      <Collapse
        ghost
        items={[
          {
            key: 'advanced',
            label: t('pages.inbounds.advancedTitle'),
            children: (
              <>
                <FormField
                  name={['settings', 'context']}
                  label={t('pages.inbounds.form.openfluxContext')}
                  tooltip={t('pages.inbounds.form.openfluxContextHint')}
                >
                  <Input
                    maxLength={4096}
                    placeholder={t('pages.inbounds.form.additionalSecretAuto')}
                  />
                </FormField>
                <FormField
                  name={['settings', 'maxPacketSize']}
                  label={t('pages.inbounds.form.openfluxPacket')}
                  tooltip={t('pages.inbounds.form.openfluxPacketHint')}
                >
                  <InputNumber min={0} max={65000} precision={0} />
                </FormField>
                <FormField
                  name={['settings', 'refusePeerLogin']}
                  label={t('pages.inbounds.form.openfluxRefuseLogin')}
                  tooltip={t('pages.inbounds.form.openfluxRefuseLoginHint')}
                  valueProp="checked"
                >
                  <Switch />
                </FormField>
                <FormField
                  name={['settings', 'debug']}
                  label={t('pages.inbounds.form.openfluxDebug')}
                  tooltip={t('pages.inbounds.form.openfluxDebugHint')}
                >
                  <Select
                    options={[0, 1, 2].map((value) => ({
                      value,
                      label: t(`pages.inbounds.form.openfluxDebug${value}`),
                    }))}
                  />
                </FormField>
              </>
            ),
          },
        ]}
      />
    </div>
  );
}
