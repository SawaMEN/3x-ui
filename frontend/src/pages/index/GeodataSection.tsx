import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Empty, Space, Spin, Tag, Typography } from 'antd';

import { useGeodataFiles } from '@/api/queries/useGeodata';
import GeoBrowserModal from '@/components/geodata/GeoBrowserModal';
import type { GeoKind } from '@/generated/types';

interface GeodataSectionProps {
  active: boolean;
  onBusy: (e: { busy: boolean; tip?: string }) => void;
  onClose: () => void;
}

export default function GeodataSection({ active }: GeodataSectionProps) {
  const { t } = useTranslation();
  const files = useGeodataFiles(active);
  const [kind, setKind] = useState<GeoKind | null>(null);
  return (
    <Spin spinning={files.isLoading}>
      <Alert type="info" className="mb-12" title={t('pages.index.geodataHint')} showIcon />
      {files.isError && <Alert type="error" title={String(files.error)} showIcon />}
      {!files.isLoading && !files.isError && files.data?.length === 0 && <Empty />}
      <Space orientation="vertical" style={{ width: '100%' }}>
        {(files.data ?? []).map((file) => (
          <div key={file.name}>
            <Tag color={file.error ? 'error' : 'success'}>{file.name}</Tag>
            <Typography.Text type={file.error ? 'danger' : 'secondary'}>
              {file.error || `${file.categories}`}
            </Typography.Text>
          </div>
        ))}
        <Space wrap>
          <Button onClick={() => void files.refetch()}>{t('refresh')}</Button>
          <Button onClick={() => setKind('site')}>Geosite</Button>
          <Button onClick={() => setKind('ip')}>GeoIP</Button>
        </Space>
      </Space>
      {kind && (
        <GeoBrowserModal
          open
          readOnly
          kind={kind}
          value=""
          onApply={() => setKind(null)}
          onClose={() => setKind(null)}
        />
      )}
    </Spin>
  );
}
