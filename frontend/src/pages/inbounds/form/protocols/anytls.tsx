import { useState } from 'react';
import { CloudDownloadOutlined } from '@ant-design/icons';
import { Alert, Button, Collapse, Form, Input, message } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { FormField } from '@/components/form/rhf';
import SniSearchInput from '../security/SniSearchInput';
import { HttpUtil } from '@/utils';
import { ANYTLS_DEFAULT_PADDING_SCHEME } from '@/schemas/protocols/inbound/anytls';

export default function AnyTlsFields() {
  const { t } = useTranslation();
  const { control, setValue } = useFormContext();
  const nodeId = useWatch({ control, name: 'nodeId' }) as number | null | undefined;
  const [loadingPanelSettings, setLoadingPanelSettings] = useState(false);
  const [messageApi, messageContextHolder] = message.useMessage();

  const insertPanelSettings = async () => {
    setLoadingPanelSettings(true);
    try {
      const response =
        typeof nodeId === 'number'
          ? await HttpUtil.get(`/panel/api/nodes/webCert/${nodeId}`, undefined, { silent: true })
          : await HttpUtil.get('/panel/api/server/getWebCertFiles', undefined, { silent: true });
      if (!response?.success) {
        messageApi.warning(response?.msg || t('pages.inbounds.setDefaultCertEmpty'));
        return;
      }
      const settings = response.obj as
        | { webCertFile?: string; webKeyFile?: string; webDomain?: string }
        | undefined;
      const certificatePath = settings?.webCertFile?.trim();
      const keyPath = settings?.webKeyFile?.trim();
      if (!certificatePath || !keyPath) {
        messageApi.warning(t('pages.inbounds.setDefaultCertEmpty'));
        return;
      }
      const options = { shouldDirty: true, shouldTouch: true, shouldValidate: true };
      setValue('settings.tls.certificatePath', certificatePath, options);
      setValue('settings.tls.keyPath', keyPath, options);
      setValue('settings.tls.enabled', true, options);
      // Both endpoints return the selected server's certificate identity.
      if (settings?.webDomain?.trim()) {
        setValue('settings.tls.serverName', settings.webDomain.trim(), options);
      }
      messageApi.success(t('pages.inbounds.setSuccess'));
    } catch {
      messageApi.error(t('somethingWentWrong'));
    } finally {
      setLoadingPanelSettings(false);
    }
  };

  return (
    <>
      {messageContextHolder}
      <Alert type="info" showIcon description={t('pages.inbounds.form.anytlsHint')} />
      <FormField
        name={['settings', 'tls', 'serverName']}
        label={t('pages.inbounds.form.anytlsServerName')}
        tooltip={t('pages.inbounds.form.anytlsServerNameHint')}
      >
        <SniSearchInput placeholder="vpn.example.com" />
      </FormField>
      <FormField
        name={['settings', 'tls', 'certificatePath']}
        label={t('pages.inbounds.form.anytlsCertificate')}
        tooltip={t('pages.inbounds.form.anytlsCertificateHint')}
      >
        <Input placeholder="/etc/letsencrypt/live/vpn.example.com/fullchain.pem" />
      </FormField>
      <FormField
        name={['settings', 'tls', 'keyPath']}
        label={t('pages.inbounds.form.anytlsPrivateKey')}
        tooltip={t('pages.inbounds.form.anytlsPrivateKeyHint')}
      >
        <Input placeholder="/etc/letsencrypt/live/vpn.example.com/privkey.pem" />
      </FormField>
      <Form.Item label=" ">
        <Button
          type="primary"
          icon={<CloudDownloadOutlined aria-hidden />}
          loading={loadingPanelSettings}
          onClick={insertPanelSettings}
        >
          {t('pages.inbounds.form.anytlsInsertPanelSettings')}
        </Button>
      </Form.Item>
      <Collapse
        ghost
        items={[
          {
            key: 'padding',
            label: t('pages.inbounds.form.anytlsAdvanced'),
            children: (
              <FormField
                name={['settings', 'paddingScheme']}
                label={t('pages.inbounds.form.anytlsPaddingScheme')}
                tooltip={t('pages.inbounds.form.anytlsPaddingSchemeHint')}
                transform={{
                  input: (value) =>
                    Array.isArray(value)
                      ? value.join('\n')
                      : ANYTLS_DEFAULT_PADDING_SCHEME.join('\n'),
                  output: (value) =>
                    String(value ?? '')
                      .split(/\r?\n/)
                      .map((line) => line.trim())
                      .filter(Boolean),
                }}
              >
                <Input.TextArea rows={8} spellCheck={false} />
              </FormField>
            ),
          },
        ]}
      />
    </>
  );
}
