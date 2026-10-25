import { Alert } from 'antd';
import { useTranslation } from 'react-i18next';
import { useFormContext, useWatch } from 'react-hook-form';

type Protocol = 'mieru' | 'trusttunnel' | 'pingtunnel' | 'sudoku' | 'vk';

export default function CascadeHint({ protocol }: { protocol: Protocol }) {
  const { t } = useTranslation();
  const { control } = useFormContext();
  const nodeId = useWatch({ control, name: 'nodeId' });
  const forwardType = useWatch({ control, name: 'settings.forward.type' });
  const details = {
    mieru: t('pages.inbounds.form.cascadeHints.users'),
    trusttunnel: t('pages.inbounds.form.cascadeHints.users'),
    pingtunnel: t('pages.inbounds.form.cascadeHints.pingtunnel'),
    sudoku: t('pages.inbounds.form.cascadeHints.sudoku'),
    vk:
      forwardType === 'wireguardInbound'
        ? t('pages.inbounds.form.cascadeHints.vkWireguard')
        : forwardType === 'hysteria2Inbound'
          ? t('pages.inbounds.form.cascadeHints.vkLocal')
          : t('pages.inbounds.form.cascadeHints.vkExternal'),
  };
  return (
    <Alert
      type="info"
      showIcon
      style={{ marginBottom: 16 }}
      title={t('pages.inbounds.form.cascadeHints.title')}
      description={
        <>
          <div>
            {t(
              nodeId != null
                ? 'pages.inbounds.form.cascadeHints.remote'
                : 'pages.inbounds.form.cascadeHints.local',
            )}
          </div>
          <div>{t('pages.inbounds.form.cascadeHints.udp')}</div>
          <div>{details[protocol]}</div>
        </>
      }
    />
  );
}
