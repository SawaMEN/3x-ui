import { useState } from 'react';
import type { Ref } from 'react';
import { Button, Input, Space, message } from 'antd';
import type { InputProps, InputRef } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { HttpUtil } from '@/utils';
import type { RealityScanResult } from '@/generated/types';
import RealityTargetScannerModal, { scannedServerName } from './RealityTargetScannerModal';
import type { SniScanPurpose } from './RealityTargetScannerModal';

interface SniSearchInputProps extends Omit<InputProps, 'value' | 'onChange'> {
  value?: string;
  onChange?: (value: string) => void;
  ref?: Ref<InputRef>;
  purpose?: Exclude<SniScanPurpose, 'reality'>;
  onPickTarget?: (result: RealityScanResult) => void;
}

// String-valued control works with FormField and keeps protocol-specific SNI
// handlers (e.g. TUIC certificate paths) identical for typing and picking.
export default function SniSearchInput({
  value,
  onChange,
  ref,
  purpose = 'tls',
  onPickTarget,
  disabled,
  style,
  ...props
}: SniSearchInputProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();
  const scan = async (targets?: string): Promise<RealityScanResult[]> => {
    const response = await HttpUtil.post<RealityScanResult[]>(
      '/panel/api/server/scanRealityTargets',
      targets ? { targets } : {},
      { silent: true, timeout: 360_000 },
    );
    if (!response?.success || !Array.isArray(response.obj)) {
      messageApi.error(response?.msg || t('pages.inbounds.toasts.scanRealityTargetError'));
      return [];
    }
    return response.obj;
  };

  return (
    <>
      {contextHolder}
      <Space.Compact block style={style}>
        <Input
          {...props}
          ref={ref}
          value={value}
          disabled={disabled}
          style={{ flex: 1, minWidth: 0 }}
          onChange={(event) => onChange?.(event.target.value)}
        />
        <Button
          disabled={disabled}
          icon={<SearchOutlined aria-hidden />}
          onClick={() => setOpen(true)}
        >
          {t('pages.inbounds.form.sniSearch')}
        </Button>
      </Space.Compact>
      <RealityTargetScannerModal
        open={open}
        onClose={() => setOpen(false)}
        purpose={purpose}
        scanRealityCandidates={scan}
        onPick={(result) => {
          onChange?.(scannedServerName(result));
          onPickTarget?.(result);
        }}
      />
    </>
  );
}
