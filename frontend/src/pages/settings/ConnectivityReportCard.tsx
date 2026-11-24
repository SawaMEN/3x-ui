import { useRef, useState } from 'react';
import { Alert, Button, Card, Space, Table, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import {
  parseConnectivityReport,
  connectivityFindings,
  type ConnectivityReport,
  type Finding,
} from './connectivity-report';

export default function ConnectivityReportCard() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => (ru ? russian : en);
  const [report, setReport] = useState<ConnectivityReport>();
  const [error, setError] = useState('');
  const seq = useRef(0);
  async function load(file: File) {
    const id = ++seq.current;
    setReport(undefined);
    setError('');
    try {
      if (file.size > 65536) throw new Error('Report too large');
      const r = parseConnectivityReport(await file.text());
      if (id === seq.current) setReport(r);
    } catch {
      if (id === seq.current)
        setError(l('Invalid connectivity report', 'Некорректный отчёт доступности'));
    }
  }
  const messages: Record<Finding, string> = {
    incomplete: l(
      'Incomplete report: repeat the test on a stable network.',
      'Неполный отчёт: повторите проверку в стабильной сети.',
    ),
    stale: l(
      'Report time is stale or inconsistent: collect a new report.',
      'Отчёт устарел или время некорректно: соберите новый отчёт.',
    ),
    sampled: l('Only a sample of DNS addresses was tested.', 'Проверена только часть адресов DNS.'),
    'no-address': l(
      'No address for at least one family; this can be normal. Check A/AAAA records.',
      'Для одного из семейств нет адресов. Это может быть нормой; проверьте A/AAAA-записи.',
    ),
    'dns-error': l(
      'DNS error: compare resolver responses and domain records.',
      'Ошибка DNS: сравните ответы резолверов и записи домена.',
    ),
    'tcp-error': l(
      'TCP failure: inspect the listener, firewall, routing and another access network.',
      'Ошибка TCP: проверьте слушающий порт, firewall, маршрутизацию и другую сеть доступа.',
    ),
    'certificate-error': l(
      'Certificate verification failed: check certificate chain, validity and hostname.',
      'Ошибка сертификата: проверьте цепочку, срок действия и имя домена.',
    ),
    'tls-error': l(
      'TLS failure after TCP: inspect TLS configuration and logs.',
      'Ошибка TLS после TCP: проверьте конфигурацию TLS и журналы.',
    ),
    'http-error': l(
      'No final HTTPS response: inspect the web server and reverse proxy.',
      'Нет итогового HTTPS-ответа: проверьте веб-сервер и обратный прокси.',
    ),
    'http-denied': l(
      'HTTP 4xx response: inspect access policy or requested resource.',
      'Ответ HTTP 4xx: проверьте правила доступа или запрошенный ресурс.',
    ),
    'http-server-error': l(
      'HTTP 5xx response: inspect upstream and application logs.',
      'Ответ HTTP 5xx: проверьте upstream и журналы приложения.',
    ),
    'family-difference': l(
      'IPv4 and IPv6 samples differ: check routing and server bindings for both families.',
      'Результаты IPv4 и IPv6 различаются: проверьте маршрутизацию и привязку сервера к адресам обоих семейств.',
    ),
    reachable: l(
      'At least one tested address returned HTTPS. This does not establish VPN availability.',
      'Хотя бы один проверенный адрес вернул HTTPS-ответ. Это не подтверждает доступность VPN.',
    ),
  };
  return (
    <Card
      size="small"
      title={l('Layered connectivity — VetrOFF-Core', 'Многоуровневая диагностика — VetrOFF-Core')}
    >
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title={l(
            'Import a connectivity v1 JSON report. Analysis is local and does not establish DPI filtering or modify settings.',
            'Импортируйте JSON-отчёт connectivity v1. Анализ локальный, не доказывает фильтрацию DPI и не меняет настройки.',
          )}
        />
        <input
          type="file"
          accept=".json,application/json"
          aria-label={l('Import connectivity report', 'Импорт отчёта доступности')}
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = '';
            if (file) void load(file);
          }}
        />
        <Button
          onClick={() => {
            ++seq.current;
            setReport(undefined);
            setError('');
          }}
        >
          {l('Clear report', 'Очистить отчёт')}
        </Button>
        {error && <Alert type="error" title={error} />}
        {report && (
          <>
            <Typography.Text>
              {report.host} —{' '}
              {report.scope === 'client-physical'
                ? l('Physical network', 'Физическая сеть')
                : l('VPN tunnel', 'VPN-туннель')}
            </Typography.Text>
            <Typography.Text type="secondary">{report.checkedAt}</Typography.Text>
            {report.families.map((f) => (
              <Card key={f.family} size="small" title={f.family === 'ip4' ? 'IPv4' : 'IPv6'}>
                <Typography.Text>
                  DNS: {f.dns.outcome} · {f.dns.durationMs} ms · {l('Addresses', 'Адресов')}:{' '}
                  {f.resolved}
                </Typography.Text>
                <Table
                  size="small"
                  pagination={false}
                  scroll={{ x: true }}
                  rowKey="ip"
                  dataSource={f.addresses}
                  columns={[
                    { title: 'IP', dataIndex: 'ip' },
                    ...(['tcp', 'tls', 'http'] as const).map((key) => ({
                      title: key.toUpperCase(),
                      render: (_: unknown, a: (typeof f.addresses)[number]) =>
                        a[key].outcome + ' · ' + a[key].durationMs + ' ms',
                    })),
                    { title: 'HTTP status', dataIndex: 'httpStatus' },
                  ]}
                />
              </Card>
            ))}
            {connectivityFindings(report).map((key) => (
              <Alert
                key={key}
                type={
                  key === 'reachable' || key === 'no-address' || key === 'sampled'
                    ? 'info'
                    : 'warning'
                }
                title={messages[key]}
              />
            ))}
          </>
        )}
      </Space>
    </Card>
  );
}
