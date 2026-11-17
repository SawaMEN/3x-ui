import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Collapse,
  Input,
  Modal,
  Progress,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';

import type { RealityScanResult } from '@/generated/types';
import { loadSNIHistory } from './sni-scanner';
import type { SNIScanCallback, SNISnapshot } from './sni-scanner';

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

export function canUseScannedTarget(
  result: RealityScanResult,
  purpose: SniScanPurpose,
  mldsa65Enabled = false,
): boolean {
  if (result.attempts && (result.successes ?? 0) < Math.ceil(result.attempts * 0.8)) return false;
  if (purpose === 'reality')
    return (
      result.feasible && (!mldsa65Enabled || result.certChainBytes >= MLDSA65_MIN_CERT_CHAIN_BYTES)
    );
  return result.certValid && !!scannedServerName(result) && (purpose === 'tls' || result.tls13);
}

interface RealityTargetScannerModalProps {
  open: boolean;
  onClose: () => void;
  scanRealityCandidates: SNIScanCallback;
  nodeId?: number | null;
  enableHistory?: boolean;
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
  nodeId,
  enableHistory = false,
}: RealityTargetScannerModalProps) {
  const { t } = useTranslation();
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(false);
  const [query, setQuery] = useState('');
  const [mode, setMode] = useState('candidates');
  const [results, setResults] = useState<RealityScanResult[]>([]);
  const [progress, setProgress] = useState<SNISnapshot | null>(null);
  const [history, setHistory] = useState<SNISnapshot[]>([]);
  const [historyId, setHistoryId] = useState<string>();
  const [historyError, setHistoryError] = useState(false);
  const scanRef = useRef(scanRealityCandidates);
  const requestId = useRef(0);
  const activeScan = useRef<AbortController | null>(null);
  useEffect(() => {
    scanRef.current = scanRealityCandidates;
  });

  const applyScan = useCallback(
    async (targets?: string, scanMode?: string, recheck?: RealityScanResult[]) => {
      activeScan.current?.abort();
      const controller = new AbortController();
      activeScan.current = controller;
      const id = ++requestId.current;
      try {
        const rows = await scanRef.current(
          targets,
          scanMode,
          controller.signal,
          (snapshot) => {
            if (id === requestId.current) {
              setProgress(snapshot);
              setResults(snapshot.results);
            }
          },
          recheck,
        );
        if (id === requestId.current) setResults(rows);
      } catch {
        if (id === requestId.current) setError(true);
      } finally {
        if (id === requestId.current) setLoading(false);
      }
    },
    [],
  );

  const runScan = useCallback(
    (targets?: string, recheck?: RealityScanResult[]) => {
      setLoading(true);
      setError(false);
      setResults([]);
      setProgress(null);
      setHistoryId(undefined);
      void applyScan(targets, recheck ? 'recheck' : mode, recheck);
    },
    [applyScan, mode],
  );

  // Clearing the previous results is done during render so the auto-scan effect
  // carries only the request itself.
  const [scannedOpen, setScannedOpen] = useState(false);
  const [scannedNode, setScannedNode] = useState(nodeId);
  if (open !== scannedOpen || nodeId !== scannedNode) {
    setScannedNode(nodeId);
    setScannedOpen(open);
    if (open) {
      setMode('candidates');
      setProgress(null);
      setHistory([]);
      setHistoryId(undefined);
      setQuery('');
      setError(false);
      setResults([]);
      setLoading(true);
    }
  }

  const invalidateScan = useCallback(() => {
    requestId.current++;
    activeScan.current?.abort();
  }, []);
  useEffect(() => {
    if (open) void applyScan();
    return invalidateScan;
  }, [open, nodeId, applyScan, invalidateScan]);

  const completedJob = progress?.state === 'done' ? progress.id : '';
  useEffect(() => {
    if (!open || !enableHistory) return;
    const controller = new AbortController();
    void loadSNIHistory(nodeId, controller.signal)
      .then((rows) => {
        if (!controller.signal.aborted) {
          setHistory(rows);
          setHistoryError(false);
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setHistoryError(true);
      });
    return () => controller.abort();
  }, [open, nodeId, enableHistory, completedJob]);

  const sortedResults = [...results].sort(
    (a, b) =>
      Number(canUseScannedTarget(b, purpose, mldsa65Enabled)) -
        Number(canUseScannedTarget(a, purpose, mldsa65Enabled)) ||
      Number((b.attempts ?? 0) > 0 && (b.successes ?? 0) >= 4) -
        Number((a.attempts ?? 0) > 0 && (a.successes ?? 0) >= 4) ||
      (b.attempts ? (b.successes ?? 0) / b.attempts : -1) -
        (a.attempts ? (a.successes ?? 0) / a.attempts : -1) ||
      (a.medianMs ?? a.latencyMs) - (b.medianMs ?? b.latencyMs) ||
      (a.jitterMs ?? 0) - (b.jitterMs ?? 0),
  );
  const selectedHistory = history.find((entry) => entry.id === historyId);

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
    { title: 'SNI', dataIndex: 'host', key: 'host', width: 180, ellipsis: true },
    {
      title: t('pages.inbounds.form.scanStatus'),
      dataIndex: 'feasible',
      key: 'feasible',
      width: 95,
      render: (_feasible: boolean, row) =>
        canUseScannedTarget(row, purpose, mldsa65Enabled) ? (
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
      render: (v: number, row) =>
        row.tlsVersion && !(row.attempts && !row.successes) ? (
          <Tooltip
            title={`TCP: ${row.tcpMs?.toFixed(2) ?? '—'} ms / TLS: ${row.tlsMs?.toFixed(2) ?? '—'} ms`}
          >
            <span>{(row.medianMs ?? v).toFixed(row.medianMs === undefined ? 0 : 1)} ms</span>
          </Tooltip>
        ) : (
          '—'
        ),
    },
    {
      title: t('pages.inbounds.form.sniStability'),
      key: 'stability',
      width: 130,
      render: (_, row) =>
        row.attempts ? (
          <Tooltip
            title={t('pages.inbounds.form.sniStabilityDetails', {
              successes: row.successes ?? 0,
              attempts: row.attempts,
              median: row.successes ? (row.medianMs ?? 0).toFixed(1) : '—',
              jitter: row.successes ? (row.jitterMs ?? 0).toFixed(1) : '—',
            })}
          >
            <span>
              <Tag color={(row.successes ?? 0) >= 4 ? 'success' : 'warning'}>
                {row.successes ?? 0}/{row.attempts}
              </Tag>
              {row === sortedResults[0] &&
              canUseScannedTarget(row, purpose, mldsa65Enabled) &&
              (!mldsa65Enabled || row.certChainBytes >= MLDSA65_MIN_CERT_CHAIN_BYTES) &&
              (row.successes ?? 0) >= 4 ? (
                <Tag color="blue">{t('pages.inbounds.form.sniRecommended')}</Tag>
              ) : null}
            </span>
          </Tooltip>
        ) : (
          <Tag>{t('pages.inbounds.form.sniPreliminary')}</Tag>
        ),
    },
    {
      title: t('pages.inbounds.form.sniDiagnostics'),
      key: 'diagnostics',
      width: 130,
      render: (_, row) => (
        <Space orientation="vertical" size={0}>
          {typeof row.http2OK === 'boolean' && (
            <Tag color={row.http2OK ? 'success' : 'warning'}>
              {row.http2OK ? 'HTTP/2 OK' : 'HTTP/2 ?'}
            </Tag>
          )}
          {(typeof row.dnsMatchesIP === 'boolean' || row.dnsLookupError) && (
            <Tooltip
              title={t(
                `pages.inbounds.form.${row.dnsLookupError ? 'sniDnsError' : row.dnsMatchesIP ? 'sniDnsMatch' : 'sniDnsDifferent'}`,
              )}
            >
              <Tag color={row.dnsMatchesIP ? 'success' : 'warning'}>
                DNS {row.dnsMatchesIP ? '✓' : '?'}
              </Tag>
            </Tooltip>
          )}
          {row.source && (
            <Typography.Text type="secondary">
              {t(`pages.inbounds.form.sniSource_${row.source}`)}
            </Typography.Text>
          )}
          {row.checkedAt && (
            <Tooltip title={row.checkedAt}>
              <Typography.Text type="secondary">
                {new Date(row.checkedAt).toLocaleString()}
              </Typography.Text>
            </Tooltip>
          )}
        </Space>
      ),
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
          disabled={
            !canUseScannedTarget(row, purpose, mldsa65Enabled) ||
            (purpose === 'reality' &&
              mldsa65Enabled &&
              row.certChainBytes < MLDSA65_MIN_CERT_CHAIN_BYTES)
          }
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
        ...(loading
          ? [
              <Button
                key="stop"
                danger
                onClick={() => {
                  invalidateScan();
                  setLoading(false);
                  setProgress((value) =>
                    value ? { ...value, state: 'canceled', phase: 'canceled' } : null,
                  );
                }}
              >
                {t('pages.inbounds.form.sniStop')}
              </Button>,
            ]
          : []),
        <Button key="rescan" onClick={() => runScan(query.trim() || undefined)} loading={loading}>
          {t('pages.inbounds.form.scanRescan')}
        </Button>,
        <Button key="close" type="primary" onClick={onClose}>
          {t('close')}
        </Button>,
      ]}
      title={t(`pages.inbounds.form.${purpose === 'reality' ? 'scanModalTitle' : 'sniScanTitle'}`)}
      width={1280}
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
            { value: 'combined', label: t('pages.inbounds.form.sniCombined') },
          ]}
          onChange={(value) => {
            invalidateScan();
            setLoading(false);
            setMode(value);
            setProgress(null);
            setHistoryId(undefined);
            setQuery('');
            setResults([]);
            setError(false);
          }}
        />
        {(mode === 'nearby' || mode === 'combined') && (
          <Alert type="info" showIcon title={t('pages.inbounds.form.scanNearbyDesc')} />
        )}
        <Typography.Text type="secondary">
          {t(`pages.inbounds.form.${nodeId ? 'sniRunNode' : 'sniRunLocal'}`, { id: nodeId })}
        </Typography.Text>
        {progress && (
          <>
            <Typography.Text>
              {t(`pages.inbounds.form.sniPhase_${progress.phase}`)} · {progress.completed}/
              {progress.total} ·{' '}
              {t('pages.inbounds.form.sniVerified', {
                count: progress.verified,
                total: progress.verifyTotal,
              })}
            </Typography.Text>
            <Progress
              percent={
                progress.phase === 'verifying'
                  ? Math.round((100 * progress.verified) / Math.max(1, progress.verifyTotal))
                  : Math.round((100 * progress.completed) / Math.max(1, progress.total))
              }
              status={
                progress.state === 'failed'
                  ? 'exception'
                  : progress.state === 'done'
                    ? 'success'
                    : 'normal'
              }
            />
            {progress.error && <Alert type="warning" showIcon title={progress.error} />}
          </>
        )}
        {enableHistory && (
          <Collapse
            size="small"
            items={[
              {
                key: 'history',
                label: t('pages.inbounds.form.sniHistory'),
                children: (
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    {historyError && (
                      <Alert type="warning" title={t('pages.inbounds.form.sniHistoryError')} />
                    )}
                    <Select
                      aria-label={t('pages.inbounds.form.sniHistory')}
                      value={historyId}
                      disabled={loading}
                      style={{ width: '100%' }}
                      placeholder={t('pages.inbounds.form.sniHistorySelect')}
                      options={history.map((entry) => ({
                        value: entry.id,
                        label: `${new Date(entry.finishedAt ?? entry.createdAt).toLocaleString()} · ${entry.results.length} SNI`,
                      }))}
                      onChange={(id) => {
                        setHistoryId(id);
                        setResults(history.find((entry) => entry.id === id)?.results ?? []);
                        setProgress(null);
                        setError(false);
                      }}
                    />
                    {selectedHistory && (
                      <>
                        <Typography.Text type="secondary">
                          {t('pages.inbounds.form.sniHistoryHint')}
                        </Typography.Text>
                        <Button
                          disabled={loading || selectedHistory.results.length === 0}
                          onClick={() => runScan(undefined, selectedHistory.results)}
                        >
                          {t('pages.inbounds.form.sniRecheck')}
                        </Button>
                      </>
                    )}
                  </Space>
                ),
              },
            ]}
          />
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
            `pages.inbounds.form.${mode === 'nearby' || mode === 'combined' ? 'scanNearbyPlaceholder' : 'scanDiscoverPlaceholder'}`,
          )}
        />
        {error && (
          <Alert type="error" showIcon title={t('pages.inbounds.toasts.scanRealityTargetError')} />
        )}
        <Table<RealityScanResult>
          size="small"
          rowKey={(row) => `${row.target}|${row.host}`}
          loading={loading}
          columns={
            purpose === 'reality'
              ? columns
              : columns.filter(
                  (column) => column.key !== 'curveID' && column.key !== 'certChainBytes',
                )
          }
          dataSource={sortedResults}
          pagination={false}
          scroll={{ x: 1200, y: 360 }}
        />
      </Space>
    </Modal>
  );
}
