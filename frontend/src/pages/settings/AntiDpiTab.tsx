import { useState } from 'react';
import { Alert, Button, Card, Input, Space, Table, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import type { AllSetting } from '@/models/setting';
import AddressReportCard from './AddressReportCard';
import ConnectivityReportCard from './ConnectivityReportCard';
import VetroffDiagnosticCard from './VetroffDiagnosticCard';
import SiberianActiveCheck from './SiberianActiveCheck';
import AdaptiveConnectPolicyCard from './AdaptiveConnectPolicyCard';
import AdaptiveConnectAudit from './AdaptiveConnectAudit';
import SubscriptionBalancersTab from './SubscriptionBalancersTab';
import { parseSiberianReport, type SiberianEvidence } from './anti-dpi-report';

interface Observation {
  ip: string;
  tcp: string;
  tls: string;
  detail?: string;
  durationMs: number;
}

interface ProbeReport {
  observations: Observation[];
  checkedAt: string;
}

export default function AntiDpiTab(props: {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
  saveSettings: () => Promise<boolean | undefined>;
  settingsDirty: boolean;
}) {
  const { i18n, t } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => ru ? russian : en;
  const [host, setHost] = useState('');
  const [busy, setBusy] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [importError, setImportError] = useState('');
  const [report, setReport] = useState<ProbeReport>();
  const [rows, setRows] = useState<SiberianEvidence[]>([]);

  async function probe() {
    setBusy(true);
    setError('');
    setReport(undefined);
    try {
      const result = await HttpUtil.post<ProbeReport>('/panel/api/setting/anti-dpi/probe',
        { host: host.trim(), port: 443 }, { timeout: 25000 });
      if (!result?.success || !result.obj) throw new Error(result?.msg || 'Probe failed');
      setReport(result.obj);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function importReport(file: File) {
    setImportError('');
    setRows([]);
    try {
      if (file.size > 1_000_000) throw new Error('Report exceeds 1 MB');
      setRows(parseSiberianReport(await file.text()));
    } catch (err) {
      setImportError(err instanceof Error ? err.message : String(err));
    }
  }

  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Alert showIcon type="info" title={l(
        'Server-side checks cannot establish which restrictions affect mobile subscribers. Test from the affected network.',
        'Проверка с сервера не определяет ограничения мобильного оператора. Нужны измерения из проблемной сети.',
      )} />
      <Card size="small" title={l('Server connectivity', 'Доступность сервера')}>
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Text type="secondary">{l(
            'Read-only DNS, TCP and verified TLS checks from this panel server. Enter a hostname you control.',
            'DNS, TCP и TLS с проверкой сертификата из сети панели. Укажите принадлежащий вам домен.',
          )}</Typography.Text>
          <Input value={host} onChange={(e) => setHost(e.target.value)} placeholder="vpn.example.com" aria-label={l('Hostname', 'Домен')} />
          <Button type="primary" loading={busy} disabled={!host.trim()} onClick={() => void probe()}>
            {l('Check HTTPS port 443', 'Проверить порт HTTPS 443')}
          </Button>
          {error && <Alert type="error" showIcon title={error} />}
          {report && <>
            <Typography.Text type="secondary">{report.checkedAt}</Typography.Text>
            <Table size="small" pagination={false} scroll={{ x: true }} rowKey="ip" dataSource={report.observations} columns={[
              { title: 'IP', dataIndex: 'ip' },
              { title: 'TCP', dataIndex: 'tcp' },
              { title: 'TLS', dataIndex: 'tls' },
              { title: l('Time, ms', 'Время, мс'), dataIndex: 'durationMs' },
              { title: l('Details', 'Подробности'), dataIndex: 'detail' },
            ]} />
            {report.observations.some((row) => row.tcp !== 'ok') &&
              <Alert type="warning" title={l('Check listener, firewall and routing; this is not proof of IP blocking.', 'Проверьте порт, firewall и маршрутизацию; это не доказательство блокировки IP.')} />}
            {report.observations.some((row) => row.tcp === 'ok' && row.tls !== 'ok') &&
              <Alert type="warning" title={l('Check TLS certificate, hostname, protocol and gateway configuration.', 'Проверьте сертификат, имя хоста, протокол и настройки шлюза.')} />}
          </>}
        </Space>
      </Card>
      <Card size="small" title="Siberian Check">
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Text>{l(
            'Import JSON from a test performed on the affected network without VPN. Reports are processed locally in the browser and never uploaded.',
            'Импортируйте JSON проверки из проблемной сети без VPN. Отчёт обрабатывается локально в браузере и не отправляется на сервер.',
          )}</Typography.Text>
          <a href="https://github.com/iddserg/siberian-check" target="_blank" rel="noreferrer">Siberian Check</a>
          <input type="file" accept=".json,application/json" aria-label={l('Import report', 'Импорт отчёта')} onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = '';
            if (file) void importReport(file);
          }} />
          {importError && <Alert type="error" title={importError} />}
          {rows.length > 0 && <>
            <Table size="small" scroll={{ x: true }} pagination={{ pageSize: 10 }} rowKey={(row) => JSON.stringify(row)} dataSource={rows} columns={[
              { title: 'IP', dataIndex: 'ip' },
              { title: 'TLS fingerprint', dataIndex: 'fingerprint' },
              { title: 'Alpha', dataIndex: 'alphaError' },
              { title: 'Beta', dataIndex: 'betaError' },
              { title: l('Evidence', 'Результат'), render: (_, row) => <Tag color={row.evidence === 'asymmetry' ? 'orange' : 'default'}>
                {row.evidence === 'asymmetry' ? l('Possible filtering', 'Возможная фильтрация') :
                  row.evidence === 'inconclusive' ? l('Inconclusive', 'Неопределённо') : l('No signal', 'Не выявлено')}
              </Tag> },
            ]} />
            <Alert type="info" title={l(
              'Asymmetry is a lead, not proof. Compare with a second network; inspect TLS rejection and server rate limits before changing settings.',
              'Асимметрия — признак, а не доказательство. Сравните с другой сетью, проверьте TLS и ограничения частоты соединений на сервере.',
            )} />
          </>}
        </Space>
      </Card>
      <SiberianActiveCheck />
      <VetroffDiagnosticCard />
      <ConnectivityReportCard />
      <AddressReportCard />
      <AdaptiveConnectAudit />
      <AdaptiveConnectPolicyCard />
      <Card size="small" title={l('Optional connection resilience', 'Опциональная устойчивость подключений')}>
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Alert type="info" title={l(
            'Reuse existing subscription balancers and Observatory. Changes only affect compatible client configurations; they cannot make a blocked IP reachable.',
            'Используются существующие балансировщики подписок и Observatory. Настройки действуют в совместимых клиентах и не снимают блокировку IP.',
          )} />
          <SubscriptionBalancersTab allSetting={props.allSetting} updateSetting={props.updateSetting} />
          <Button type="primary" loading={saving} disabled={!props.settingsDirty} onClick={async () => {
            setSaving(true);
            try { await props.saveSettings(); } finally { setSaving(false); }
          }}>{t('save')}</Button>
        </Space>
      </Card>
    </Space>
  );
}
