import { useRef, useState } from 'react';
import { Alert, Button, Card, Space, Table, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import {
  addressFindings,
  compareAddressReports,
  parseAddressReport,
  type AddressComparison,
  type AddressFinding,
  type AddressReport,
} from './address-report';

export default function AddressReportCard() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => (ru ? russian : en);
  const [physical, setPhysical] = useState<AddressReport>();
  const [tunnel, setTunnel] = useState<AddressReport>();
  const [error, setError] = useState('');
  const seq = useRef(0);
  async function load(file: File) {
    const id = ++seq.current;
    setError('');
    try {
      if (file.size > 65536) throw new Error('Report too large');
      const r = parseAddressReport(await file.text());
      if (id !== seq.current) return;
      if (r.scope === 'client-physical') setPhysical(r);
      else setTunnel(r);
    } catch {
      if (id === seq.current)
        setError(l('Invalid single-address report', 'Некорректный отчёт проверки IP'));
    }
  }
  const labels: Record<AddressFinding, string> = {
    incomplete: l(
      'Incomplete observation; repeat the test.',
      'Неполное измерение; повторите проверку.',
    ),
    stale: l(
      'Report is stale or has an inconsistent time.',
      'Отчёт устарел или имеет некорректное время.',
    ),
    'tcp-error': l(
      'TCP failed: check routing, listener and firewall.',
      'Ошибка TCP: проверьте маршрутизацию, порт и firewall.',
    ),
    'certificate-error': l(
      'Certificate validation failed: check hostname and chain.',
      'Ошибка сертификата: проверьте имя и цепочку.',
    ),
    'tls-error': l(
      'TLS failed after TCP: inspect server configuration.',
      'Ошибка TLS после TCP: проверьте конфигурацию сервера.',
    ),
    'http-error': l(
      'No final HTTPS response. Inspect the web server.',
      'Нет итогового HTTPS-ответа. Проверьте веб-сервер.',
    ),
    'http-denied': l(
      'HTTP 4xx response; check access policy.',
      'Ответ HTTP 4xx; проверьте правила доступа.',
    ),
    'http-server-error': l(
      'HTTP 5xx response; inspect application logs.',
      'Ответ HTTP 5xx; проверьте журналы приложения.',
    ),
    reachable: l(
      'This IP returned HTTPS at the time of the test.',
      'Этот IP вернул HTTPS-ответ в момент проверки.',
    ),
  };
  const comparisons: Record<AddressComparison, string> = {
    'not-comparable': l(
      'Targets, addresses or times are not comparable.',
      'Цели, IP или время измерений несопоставимы.',
    ),
    inconclusive: l(
      'One or both observations are incomplete or canceled.',
      'Одно или оба измерения неполные или отменены.',
    ),
    'physical-only': l(
      'HTTPS responded only on the physical path. Check tunnel egress and routing.',
      'HTTPS ответил только в физической сети. Проверьте выходной узел VPN и маршрутизацию.',
    ),
    'tunnel-only': l(
      'HTTPS responded only through the tunnel. Compare access-network routing and server logs.',
      'HTTPS ответил только через VPN. Сравните маршрутизацию сети доступа и журналы сервера.',
    ),
    'both-responded': l('Both paths returned HTTPS responses.', 'Оба пути вернули HTTPS-ответ.'),
    'neither-responded': l(
      'Neither path returned HTTPS. Inspect the individual stages and server.',
      'Ни один путь не вернул HTTPS. Проверьте отдельные этапы и сервер.',
    ),
  };
  const reports = [physical, tunnel].filter((r): r is AddressReport => r !== undefined);
  const comparison = physical && tunnel ? compareAddressReports(physical, tunnel) : undefined;
  return (
    <Card
      size="small"
      title={l('Single IP connectivity — VetrOFF-Core', 'Доступность отдельного IP — VetrOFF-Core')}
    >
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title={l(
            'Local import of address-connectivity v1. No DNS test was performed. A response or failure at one IP does not establish an operator whitelist, subnet reachability or DPI filtering.',
            'Локальный импорт address-connectivity v1. DNS не проверялся. Ответ или сбой одного IP не доказывает белый список оператора, доступность подсети или фильтрацию DPI.',
          )}
        />
        <input
          type="file"
          accept=".json,application/json"
          aria-label={l('Import address report', 'Импорт отчёта IP')}
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = '';
            if (file) void load(file);
          }}
        />
        <Button
          onClick={() => {
            ++seq.current;
            setPhysical(undefined);
            setTunnel(undefined);
            setError('');
          }}
        >
          {l('Clear reports', 'Очистить отчёты')}
        </Button>
        {error && <Alert type="error" title={error} />}
        {reports.map((r) => (
          <Card
            key={r.scope}
            size="small"
            title={
              r.scope === 'client-physical'
                ? l('Physical network', 'Физическая сеть')
                : l('VPN tunnel', 'VPN-туннель')
            }
          >
            <Space orientation="vertical" style={{ width: '100%' }}>
              <Typography.Text>
                {r.host} — {r.address?.ip || '—'}
              </Typography.Text>
              <Typography.Text type="secondary">{r.checkedAt}</Typography.Text>
              {r.address && (
                <Table
                  size="small"
                  pagination={false}
                  rowKey="ip"
                  scroll={{ x: true }}
                  dataSource={[r.address]}
                  columns={[
                    { title: 'IP', dataIndex: 'ip' },
                    ...(['tcp', 'tls', 'http'] as const).map((key) => ({
                      title: key.toUpperCase(),
                      render: (_: unknown, a: NonNullable<AddressReport['address']>) =>
                        a[key].outcome + ' · ' + a[key].durationMs + ' ms',
                    })),
                    { title: 'HTTP status', dataIndex: 'httpStatus' },
                  ]}
                />
              )}
              {addressFindings(r).map((key) => (
                <Alert
                  key={key}
                  type={key === 'reachable' ? 'info' : 'warning'}
                  title={labels[key]}
                />
              ))}
            </Space>
          </Card>
        ))}
        {comparison && (
          <Alert
            type="info"
            title={comparisons[comparison]}
            description={l(
              'The files do not verify that both observations used the same access network. No settings are changed.',
              'Файлы не подтверждают использование одной сети доступа. Настройки не изменяются.',
            )}
          />
        )}
      </Space>
    </Card>
  );
}
