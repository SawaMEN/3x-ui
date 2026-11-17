import { useId, useState } from 'react';
import { Button, Form, Input, Typography } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import { strToU8, zipSync } from 'fflate';
import { useTranslation } from 'react-i18next';
import { FileManager } from '@/utils';
import { openFluxMAXClientFiles } from '@/lib/openflux';

export default function OpenFluxMAXClientSetup({
  secret,
  context,
  serverID,
  serverToken,
}: {
  secret: string;
  context: string;
  serverID: string;
  serverToken: string;
}) {
  const { t } = useTranslation();
  const uid = useId();
  const [clientToken, setClientToken] = useState('');
  const files =
    clientToken !== serverToken
      ? openFluxMAXClientFiles(
          secret,
          context,
          serverID,
          clientToken,
          t('pages.inbounds.form.openfluxMaxArchiveHelp'),
        )
      : undefined;
  return (
    <div className="openflux-max-client">
      <Typography.Paragraph>{t('pages.inbounds.form.openfluxMaxClientHelp')}</Typography.Paragraph>
      {(!secret || !context) && (
        <Typography.Text type="secondary">
          {t('pages.inbounds.form.openfluxMaxSaveFirst')}
        </Typography.Text>
      )}
      {!serverID && (
        <Typography.Text type="secondary">
          {t('pages.inbounds.form.openfluxMaxNeedID')}
        </Typography.Text>
      )}
      <Form.Item
        label={t('pages.inbounds.form.openfluxMaxClientToken')}
        htmlFor={uid}
        tooltip={t('pages.inbounds.form.openfluxMaxClientTokenHelp')}
        validateStatus={clientToken && clientToken === serverToken ? 'error' : undefined}
        help={
          clientToken && clientToken === serverToken
            ? t('pages.inbounds.form.openfluxMaxDifferentAccounts')
            : undefined
        }
      >
        <Input.Password
          id={uid}
          value={clientToken}
          autoComplete="new-password"
          onChange={(event) => setClientToken(event.target.value.trim())}
        />
      </Form.Item>
      <Button
        icon={<DownloadOutlined aria-hidden />}
        aria-label={t('pages.inbounds.form.openfluxMaxDownload')}
        disabled={!files}
        onClick={() => {
          if (!files) return;
          const archive = zipSync(
            Object.fromEntries(Object.entries(files).map(([name, text]) => [name, strToU8(text)])),
            { os: 3, attrs: 0o100600 << 16 },
          );
          FileManager.downloadTextFile(
            archive as Uint8Array<ArrayBuffer>,
            'openflux-max-client.zip',
            { type: 'application/zip' },
          );
        }}
      >
        {t('pages.inbounds.form.openfluxMaxDownload')}
      </Button>
      <Typography.Text type="secondary">
        {t('pages.inbounds.form.openfluxMaxArchiveNote')}
      </Typography.Text>
    </div>
  );
}
