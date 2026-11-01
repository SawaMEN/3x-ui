import { useState } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import type { Ref } from 'react';
import { Button, Input, Space, message } from 'antd';
import type { InputProps, InputRef } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { runSNIScan } from './sni-scanner';
import type { SNIScanCallback } from './sni-scanner';
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
  const { control } = useFormContext();
  const nodeId = useWatch({ control, name: 'nodeId' }) as number | null | undefined;
  const scan: SNIScanCallback = async (targets, mode, signal, progress, recheck) => {
    try {
      return await runSNIScan(nodeId, purpose, targets, mode, signal, progress, recheck);
    } catch (error) {
      if (!signal?.aborted)
        messageApi.error(
          error instanceof Error
            ? error.message
            : t('pages.inbounds.toasts.scanRealityTargetError'),
        );
      throw error;
    }
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
        nodeId={nodeId}
        enableHistory
        scanRealityCandidates={scan}
        onPick={(result) => {
          onChange?.(scannedServerName(result));
          onPickTarget?.(result);
        }}
      />
    </>
  );
}
