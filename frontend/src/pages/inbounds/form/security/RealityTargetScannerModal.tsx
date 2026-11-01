import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Select, Space, Table, Tag, Tooltip, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';

import type { RealityScanResult } from '@/generated/types';

// xray-core ML-DSA-65 REALITY min peer cert-chain size (not defined in this repo).
export const MLDSA65_MIN_CERT_CHAIN_BYTES = 3500;

export type SniScanPurpose = 'reality' | 'tls' | 'shadowtls' | 'fakeTls';

export function scannedServerName(result: RealityScanResult): string {
  // A verified hostname is usable even when covered only by a wildcard SAN
  // (the backend intentionally omits wildcard patterns from serverNames).
  if (result.certValid && /[a-z]/i.test(result.host) && !result.host.includes(':')) {
    return result.host;
  }
  return result.serverNames?.[0] ?? '';
}

export function canUseScannedTarget(result: RealityScanResult, purpose: SniScanPurpose): boolean {
  if (purpose === 'reality') return true;
  return result.certValid && !!scannedServerName(result) && (purpose === 'tls' || result.tls13);
}

interface RealityTargetScannerModalProps {
  open: boolean;
  onClose: () => void;
  scanRealityCandidates: (targets?: string, mode?: string) => Promise<RealityScanResult[]>;
  onPick: (result: RealityScanResult) => void;
  mldsa65Enabled?: boolean;
  purpose?: SniScanPurpose;
}

export default function RealityTargetScannerModal({
  open,
  onClose,
  scanRealityCandidates,
  onPick,
  mldsa65Enabled = false,
  purpose = 'reality',
}: RealityTargetScannerModalProps) {
  const { t } = useTranslation();
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState('candidates');
  const [results, setResults] = useState<RealityScanResult[]>([]);
  const scanRef = useRef(scanRealityCandidates);
  const requestId = useRef(0);
  useEffect(() => {
    scanRef.current = scanRealityCandidates;
  });

  const applyScan = useCallback(async (targets?: string, scanMode?: string) => {
    const id = ++requestId.current;
    try {
      const rows = await scanRef.current(targets, scanMode);
      if (id === requestId.current) setResults(rows);
    } catch {
      if (id === requestId.current) setError(true);
    } finally {
      if (id === requestId.current) setLoading(false);
    }
  }, []);

  const runScan = useCallback(
    (targets?: string) => {
      setLoading(true);
      setError(false);
      setResults([]);
      void applyScan(targets, mode);
    },
    [applyScan, mode],
  );

  // Clearing the previous results is done during render so the auto-scan effect
  // carries only the request itself.
  const [scannedOpen, setScannedOpen] = useState(false);
  if (open !== scannedOpen) {
    setScannedOpen(open);
    if (open) {
      setMode('candidates');
      setQuery('');
      setError(false);
      setResults([]);
      setLoading(true);
    }
  }

  const invalidateScan = useCallback(() => {
    requestId.current++;
  }, []);
  useEffect(() => {
    if (open) void applyScan();
    return invalidateScan;
  }, [open, applyScan, invalidateScan]);

  const columns: ColumnsType<RealityScanResult> = [
    {
      title: t('pages.inbounds.form.target'),
      dataIndex: 'target',
      key: 'target',
      width: 200,
      render: (target: string, row) => (
        <Tooltip title={row.ip ? `${target} — ${row.ip}` : target}>
          <div style={{ lineHeight: 1.25 }}>
            <div style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {target}
            </div>
            {row.ip ? <div style={{ color: '#999', fontSize: 12 }}>{row.ip}</div> : null}
          </div>
        </Tooltip>
      ),
    },
    {
      title: t('pages.inbounds.form.scanStatus'),
      dataIndex: 'feasible',
      key: 'feasible',
      width: 95,
      render: (feasible: boolean, row) =>
        (purpose === 'reality' ? feasible : canUseScannedTarget(row, purpose)) ? (
          <Tag color="success">{t('pages.inbounds.form.scanFeasible')}</Tag>
        ) : (
          <Tooltip
            title={purpose === 'reality' ? row.reason : t('pages.inbounds.form.sniScanUnavailable')}
          >
            <Tag color="warning">{t('pages.inbounds.form.scanNotFeasible')}</Tag>
          </Tooltip>
        ),
    },
    {
      title: 'TLS',
      dataIndex: 'tlsVersion',
      key: 'tlsVersion',
      width: 60,
      render: (v: string) => v || '—',
    },
    {
      title: 'ALPN',
      dataIndex: 'alpn',
      key: 'alpn',
      width: 75,
      render: (v: string) => v || '—',
    },
    {
      title: t('pages.inbounds.form.scanCurve'),
      dataIndex: 'curveID',
      key: 'curveID',
      width: 130,
      render: (v: string) => v || '—',
    },
    {
      title: t('pages.inbounds.form.scanCert'),
      dataIndex: 'certSubject',
      key: 'certSubject',
      width: 160,
      ellipsis: true,
      render: (_: string, row) =>
        row.certValid ? (
          <Tooltip title={`${row.certSubject} (${row.certIssuer})`}>
            <span>{row.certSubject || '—'}</span>
          </Tooltip>
        ) : (
          <Tag>{t('pages.inbounds.form.scanCertInvalid')}</Tag>
        ),
    },
    {
      title: t('pages.inbounds.form.scanCertChain'),
      dataIndex: 'certChainBytes',
      key: 'certChainBytes',
      width: 100,
      render: (bytes: number) => {
        if (!bytes) return '—';
        if (mldsa65Enabled && bytes < MLDSA65_MIN_CERT_CHAIN_BYTES) {
          return (
            <Tooltip
              title={t('pages.inbounds.form.scanMldsaCertChainTooSmall', {
                length: bytes,
                min: MLDSA65_MIN_CERT_CHAIN_BYTES,
              })}
            >
              <Tag color="warning">{bytes} B</Tag>
            </Tooltip>
          );
        }
        return `${bytes} B`;
      },
    },
    {
      title: t('pages.inbounds.form.scanLatency'),
      dataIndex: 'latencyMs',
      key: 'latencyMs',
      width: 85,
      render: (v: number) => (v > 0 ? `${v} ms` : '—'),
    },
    {
      title: '',
      key: 'action',
      width: 100,
      fixed: 'right',
      render: (_, row) => (
        <Button
          type="link"
          size="small"
          disabled={!canUseScannedTarget(row, purpose)}
          onClick={() => {
            onPick(row);
            onClose();
          }}
        >
          {t('pages.inbounds.form.scanUse')}
        </Button>
      ),
    },
  ];

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={[
        <Button key="rescan" onClick={() => runScan(query.trim() || undefined)} loading={loading}>
          {t('pages.inbounds.form.scanRescan')}
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          {t('close')}
        </Button>,
      ]}
      title={t(`pages.inbounds.form.${purpose === 'reality' ? 'scanModalTitle' : 'sniScanTitle'}`)}
      width={1080}
    >
      <Space orientation="vertical" size="small" style={{ width: '100%' }}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          {t(
            `pages.inbounds.form.${purpose === 'reality' ? 'scanModalDesc' : purpose === 'tls' ? 'sniScanTlsDesc' : 'sniScanTargetDesc'}`,
          )}
        </Typography.Paragraph>
        <Select
          aria-label={t('pages.inbounds.form.scanMode')}
          value={mode}
          style={{ width: '100%' }}
          options={[
            { value: 'candidates', label: t('pages.inbounds.form.scanModeCandidates') },
            { value: 'nearby', label: t('pages.inbounds.form.scanModeNearby') },
          ]}
          onChange={(value) => {
            invalidateScan();
            setLoading(false);
            setMode(value);
            setQuery('');
            setResults([]);
            setError(false);
          }}
        />
        {mode === 'nearby' && (
          <Alert type="info" showIcon title={t('pages.inbounds.form.scanNearbyDesc')} />
        )}
        <Input.Search
          aria-label={t('pages.inbounds.form.scanDiscoverPlaceholder')}
          allowClear
          enterButton={t('pages.inbounds.form.scan')}
          loading={loading}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onSearch={() => runScan(query.trim() || undefined)}
          placeholder={t(
            `pages.inbounds.form.${mode === 'nearby' ? 'scanNearbyPlaceholder' : 'scanDiscoverPlaceholder'}`,
          )}
        />
        {error && (
          <Alert type="error" showIcon title={t('pages.inbounds.toasts.scanRealityTargetError')} />
        )}
        <Table<RealityScanResult>
          size="small"
          rowKey="target"
          loading={loading}
          columns={
            purpose === 'reality'
              ? columns
              : columns.filter(
                  (column) => column.key !== 'curveID' && column.key !== 'certChainBytes',
                )
          }
          dataSource={
            purpose === 'reality'
              ? results
              : [...results].sort(
                  (a, b) =>
                    Number(canUseScannedTarget(b, purpose)) -
                      Number(canUseScannedTarget(a, purpose)) || a.latencyMs - b.latencyMs,
                )
          }
          pagination={false}
          scroll={{ x: 850, y: 360 }}
        />
      </Space>
    </Modal>
  );
}
