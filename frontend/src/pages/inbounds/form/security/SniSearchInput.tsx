import { useState } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import type { Ref } from 'react';
import { Button, Input, Select, Space, message } from 'antd';
import type { InputProps, InputRef } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { runSNIScan } from './sni-scanner';
import type { SNIScanCallback } from './sni-scanner';
import type { RealityScanResult } from '@/generated/types';
import RealityTargetScannerModal, { scannedServerName } from './RealityTargetScannerModal';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import type { SniScanPurpose } from './RealityTargetScannerModal';

interface SniSearchInputProps extends Omit<InputProps, 'value' | 'onChange'> {
  value?: string;
  onChange?: (value: string) => void;
  ref?: Ref<InputRef>;
  purpose?: Exclude<SniScanPurpose, 'reality'>;
  onPickTarget?: (result: RealityScanResult) => void;
  onPickManagedDomain?: (domain: ManagedSniDomain) => void;
}

function managedDomainKey(domain: ManagedSniDomain): string {
  return JSON.stringify([domain.source, domain.nodeId ?? null, domain.name]);
}

// String-valued control works with FormField and keeps protocol-specific SNI
// handlers (e.g. TUIC certificate paths) identical for typing and picking.
export default function SniSearchInput({
  value,
  onChange,
  ref,
  purpose = 'tls',
  onPickTarget,
  onPickManagedDomain,
  disabled,
  style,
  ...props
}: SniSearchInputProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { data: managedDomains = [] } = useManagedSniDomains();
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
        {managedDomains.length > 0 && (
          <Select
            aria-label={t('pages.settings.domains.managedSni')}
            style={{ width: 170, minWidth: 110 }}
            value={undefined}
            placeholder={t('pages.settings.domains.managedSni')}
            disabled={disabled}
            showSearch={{ optionFilterProp: 'label' }}
            options={managedDomains.map((domain) => ({
              label: `${domain.name} · ${domain.source === 'telemt' ? 'Telemt WEB' : domain.source === 'remote' ? domain.nodeName || t('pages.settings.domains.remoteSNITag') : 'TLS'}`,
              value: managedDomainKey(domain),
            }))}
            onChange={(key) => {
              const domain = managedDomains.find((item) => managedDomainKey(item) === key);
              if (!domain) return;
              onChange?.(domain.name);
              onPickManagedDomain?.(domain);
              if (domain.source === 'remote' && domain.nodeId !== nodeId && purpose === 'tls') {
                messageApi.info(t('pages.settings.domains.remoteSNITLSWarning'));
              }
            }}
          />
        )}
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
