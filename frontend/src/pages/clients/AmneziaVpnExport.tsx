import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Segmented, Space, Spin, Typography } from 'antd';
import { QrPanel } from '@/pages/inbounds/qr';
import { awgVersion } from '@/lib/xray/amneziawg-obfuscation';
import type { AwgClientOptions } from '@/schemas/awg-client-options';
import type { InboundOption } from '@/hooks/useClients';
import { buildAmneziaVpnProfile } from './amneziaVpnProfile';

export default function AmneziaVpnExport({
  config,
  remark,
  downloadName,
  inbound,
  clientOptions,
}: {
  config: string;
  remark: string;
  downloadName: string;
  inbound: InboundOption;
  clientOptions?: AwgClientOptions | null;
}) {
  const { t } = useTranslation();
  const [format, setFormat] = useState<'conf' | 'amnezia'>('conf');
  const [result, setResult] = useState<Awaited<ReturnType<typeof buildAmneziaVpnProfile>> | null>(
    null,
  );
  const [failed, setFailed] = useState(false);
  const [index, setIndex] = useState(0);
  const scope = `${config}\0${remark}\0${format}`;
  const [preparedScope, setPreparedScope] = useState(scope);
  if (preparedScope !== scope) {
    setPreparedScope(scope);
    setResult(null);
    setFailed(false);
    setIndex(0);
  }
  useEffect(() => {
    if (format !== 'amnezia') return;
    let cancelled = false;
    buildAmneziaVpnProfile(config, remark)
      .then((profile) => {
        if (!cancelled) setResult(profile);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [config, remark, format]);
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      <Typography.Text type="secondary">
        {t('pages.clients.awgRequiredVersion', {
          version: awgVersion({ ...inbound.awgServer, ...clientOptions }),
        })}
      </Typography.Text>
      <Segmented
        value={format}
        options={[
          { label: '.conf', value: 'conf' },
          { label: 'Amnezia VPN', value: 'amnezia' },
        ]}
        onChange={setFormat}
      />
      {format === 'conf' ? (
        <QrPanel
          value={config}
          remark={remark}
          downloadName={downloadName}
          showQr={new TextEncoder().encode(config).length <= 2953}
        />
      ) : failed ? (
        <Alert type="error" title={t('pages.clients.amneziaExportError')} />
      ) : !result ? (
        <Spin />
      ) : (
        <>
          <Alert type="info" showIcon title={t('pages.clients.amneziaMtuImportHint')} />
          <Button href={result.link}>{t('pages.clients.openAmnezia')}</Button>
          <QrPanel
            value={result.link}
            remark={remark}
            downloadName={downloadName.replace(/\.conf$/, '.vpn')}
            showQr={false}
          />
          <Typography.Text>
            {t('pages.clients.amneziaScanChunks', { current: index + 1, total: result.qr.length })}
          </Typography.Text>
          <QrPanel
            value={result.qr[index]}
            remark={`${remark} (${index + 1}/${result.qr.length})`}
          />
          {result.qr.length > 1 && (
            <Space>
              <Button disabled={index === 0} onClick={() => setIndex(index - 1)}>
                {t('pages.clients.amneziaPreviousQr')}
              </Button>
              <Button disabled={index === result.qr.length - 1} onClick={() => setIndex(index + 1)}>
                {t('pages.clients.amneziaNextQr')}
              </Button>
            </Space>
          )}
        </>
      )}
    </Space>
  );
}
