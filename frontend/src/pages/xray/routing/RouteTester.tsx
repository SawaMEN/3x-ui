import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Col, Input, InputNumber, Row, Select, Space, Tag } from 'antd';
import { AimOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { buildRemarkByTag, formatInboundTag } from './helpers';

interface RouteTesterProps {
  inboundTags: string[];
  isMobile: boolean;
}

// Mirror of the /xray/routeTest response (RoutingService.TestRoute).
interface RouteTestResult {
  matched: boolean;
  outboundTag: string;
  groupTags?: string[];
}

type TestNetwork = 'tcp' | 'udp';
interface NetworkRouteResult {
  network: TestNetwork;
  result?: RouteTestResult;
  error?: string;
}

const PROTOCOL_OPTIONS = ['http', 'tls', 'quic', 'bittorrent'].map((p) => ({ label: p, value: p }));

export default function RouteTester({ inboundTags, isMobile }: RouteTesterProps) {
  const { t } = useTranslation();
  const { data: inboundOptions } = useInboundOptions();
  const remarkByTag = useMemo(() => buildRemarkByTag(inboundOptions || []), [inboundOptions]);
  const [dest, setDest] = useState('');
  const [port, setPort] = useState<number | null>(443);
  const [network, setNetwork] = useState<'both' | TestNetwork>('both');
  const [inboundTag, setInboundTag] = useState<string | undefined>(undefined);
  const [protocol, setProtocol] = useState<string | undefined>(undefined);
  const [testing, setTesting] = useState(false);
  const [results, setResults] = useState<NetworkRouteResult[]>([]);

  async function run() {
    const value = dest.trim();
    if (!value) return;
    // IPv6 contains ':'. IPv4 detection here only selects the field;
    // the server validates the actual address before querying Xray.
    const isIp = /^(\d{1,3}\.){3}\d{1,3}$/.test(value) || value.includes(':');
    const networks: TestNetwork[] = network === 'both' ? ['tcp', 'udp'] : [network];
    setTesting(true);
    setResults([]);
    try {
      const checked = await Promise.all(
        networks.map(async (selected): Promise<NetworkRouteResult> => {
          try {
            const msg = await HttpUtil.post<RouteTestResult>('/panel/api/xray/routeTest', {
              domain: isIp ? '' : value,
              ip: isIp ? value : '',
              port: port ?? 0,
              network: selected,
              inboundTag: inboundTag || '',
              protocol: protocol || '',
            }, { headers: { 'Content-Type': 'application/json' }, silent: true });
            if (!msg?.success || !msg.obj) {
              return { network: selected, error: msg?.msg || 'Routing API unavailable' };
            }
            return { network: selected, result: msg.obj };
          } catch (err) {
            return {
              network: selected,
              error: err instanceof Error ? err.message : String(err),
            };
          }
        }),
      );
      setResults(checked);
    } finally {
      setTesting(false);
    }
  }

  const fieldSpan = isMobile ? 24 : undefined;

  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Alert type="info" showIcon title={t('pages.xray.routeTesterDesc')} />
      <Row gutter={[8, 8]} align="bottom">
        <Col xs={fieldSpan} sm={7}>
          <Input
            aria-label={t('pages.xray.routeTesterDest')}
            placeholder={t('pages.xray.routeTesterDest')}
            value={dest}
            onChange={(e) => setDest(e.target.value)}
            onPressEnter={run}
            allowClear
          />
        </Col>
        <Col xs={12} sm={3}>
          <InputNumber
            aria-label={t('pages.xray.routeTesterPort')}
            style={{ width: '100%' }}
            min={0}
            max={65535}
            placeholder={t('pages.xray.routeTesterPort')}
            value={port}
            onChange={(v) => setPort(v)}
          />
        </Col>
        <Col xs={12} sm={3}>
          <Select
            aria-label={t('pages.inbounds.network')}
            style={{ width: '100%' }}
            value={network}
            onChange={setNetwork}
            options={[
              { label: 'TCP + UDP', value: 'both' },
              { label: 'TCP', value: 'tcp' },
              { label: 'UDP', value: 'udp' },
            ]}
          />
        </Col>
        <Col xs={12} sm={4}>
          <Select
            aria-label={t('pages.xray.routeTesterInbound')}
            style={{ width: '100%' }}
            placeholder={t('pages.xray.routeTesterInbound')}
            allowClear
            value={inboundTag}
            onChange={setInboundTag}
            options={inboundTags
              .filter(Boolean)
              .map((tag) => ({ label: formatInboundTag(tag, remarkByTag), value: tag }))}
          />
        </Col>
        <Col xs={12} sm={4}>
          <Select
            aria-label={t('pages.xray.routeTesterProtocol')}
            style={{ width: '100%' }}
            placeholder={t('pages.xray.routeTesterProtocol')}
            allowClear
            value={protocol}
            onChange={setProtocol}
            options={PROTOCOL_OPTIONS}
          />
        </Col>
        <Col xs={fieldSpan} sm={3}>
          <Button
            type="primary"
            icon={<AimOutlined aria-hidden />}
            loading={testing}
            disabled={!dest.trim()}
            onClick={run}
            block
          >
            {t('pages.xray.routeTesterTest')}
          </Button>
        </Col>
      </Row>

      {results.map((entry) => (
        <Alert
          key={entry.network}
          type={entry.error ? 'error' : entry.result?.matched ? 'success' : 'warning'}
          showIcon
          title={
            <Space wrap>
              <Tag>{entry.network.toUpperCase()}</Tag>
              {entry.error
                ? entry.error
                : entry.result?.matched
                  ? t('pages.xray.routeTesterMatchedOutbound')
                  : t('pages.xray.routeTesterDefaultOutbound')}
              {entry.result?.matched && (
                <Tag color="blue">{entry.result.outboundTag || '—'}</Tag>
              )}
              {(entry.result?.groupTags || []).length > 0 && (
                <>
                  <span>{t('pages.xray.routeTesterViaBalancer')}:</span>
                  {(entry.result?.groupTags || []).map((tag) => (
                    <Tag key={tag} color="orange">{tag}</Tag>
                  ))}
                </>
              )}
            </Space>
          }
        />
      ))}
    </Space>
  );
}
