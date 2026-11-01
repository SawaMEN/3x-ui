import { useTranslation } from 'react-i18next';
import { Button, QRCode, Tag } from 'antd';
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons';

import SubQrButton from './SubQrButton';

interface SubLinksTabProps {
  subUrl: string;
  subJsonUrl: string;
  subFormatMode?: string;
  subClashUrl: string;
  onCopy: (value: string) => void;
}

const appendRawView = (url: string) => `${url}${url.includes('?') ? '&' : '?'}view=raw`;
const appendNativeFormat = (url: string, format: string) =>
  url ? `${url}${url.includes('?') ? '&' : '?'}format=${format}` : '';

export default function SubLinksTab({
  subUrl,
  subJsonUrl,
  subClashUrl,
  subFormatMode = 'both',
  onCopy,
}: SubLinksTabProps) {
  const { t } = useTranslation();
  const subLabel = t('pages.settings.subSettings');
  const rows = [
    { kind: 'SUB', color: 'green', url: subUrl, title: subLabel, downloadable: false },
    {
      kind: 'ALL',
      color: 'green',
      url: subFormatMode === 'both' ? appendNativeFormat(subUrl, 'links') : '',
      title: 'Все подключения',
      downloadable: false,
    },
    {
      kind: 'XRAY',
      color: 'purple',
      url: subJsonUrl,
      title: 'Xray JSON',
      downloadable: true,
    },
    { kind: 'CLASH', color: 'gold', url: subClashUrl, title: 'Clash / Mihomo', downloadable: true },
    {
      kind: 'SING-BOX',
      color: 'cyan',
      url: subFormatMode === 'xray' ? '' : appendNativeFormat(subUrl, 'sing-box'),
      title: 'Sing-box JSON (AnyTLS / ShadowTLS)',
      downloadable: true,
    },
    {
      kind: 'HIDDIFY',
      color: 'blue',
      url: subFormatMode === 'xray' ? '' : appendNativeFormat(subUrl, 'hiddify'),
      title: 'Hiddify JSON (AnyTLS / ShadowTLS)',
      downloadable: true,
    },
  ].filter((row) => row.url);

  return (
    <div className="sub-rows">
      {rows.map((row) => (
        <div key={row.kind} className="sub-row">
          <Tag color={row.color} className="sub-row-tag">
            {row.kind}
          </Tag>
          <div className="sub-row-main">
            <a href={row.url} target="_blank" rel="noopener noreferrer" className="sub-row-title">
              {row.title}
            </a>
            <div className="sub-row-url" dir="ltr" title={row.url}>
              {row.url}
            </div>
          </div>
          <div className="sub-row-actions">
            {row.downloadable && (
              <Button
                href={appendRawView(row.url)}
                target="_blank"
                rel="noopener noreferrer"
                icon={<DownloadOutlined aria-hidden />}
                aria-label={t('download')}
                title={t('download')}
              />
            )}
            <Button
              icon={<CopyOutlined aria-hidden />}
              onClick={() => onCopy(row.url)}
              aria-label={t('copy')}
              title={t('copy')}
            />
            <SubQrButton value={row.url} label={row.title} onCopy={onCopy} />
          </div>
        </div>
      ))}
      {subUrl && (
        <div className="sub-qr-card">
          <div className="sub-qr-code">
            <QRCode
              value={subUrl}
              size={112}
              type="svg"
              bordered={false}
              color="#000000"
              bgColor="#ffffff"
            />
          </div>
          <div>
            <div className="sub-qr-title">{t('subscription.scanTitle')}</div>
            <div className="sub-muted">{t('subscription.scanHint')}</div>
          </div>
        </div>
      )}
    </div>
  );
}
