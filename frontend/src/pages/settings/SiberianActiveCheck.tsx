import { useState } from 'react';
import { Alert, Button, Card, Checkbox, Input, Select, Space, Table, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';

interface Attempt {
  stage: string;
  outcome: string;
  detail?: string;
  durationMs: number;
}
interface Result {
  ip: string;
  host: string;
  controlHost: string;
  fingerprint: string;
  baseline: Attempt[];
  burst: Attempt[] | null;
  control?: Attempt;
  evidence: 'asymmetry' | 'inconclusive' | 'no-signal';
}

export default function SiberianActiveCheck() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => ru ? russian : en;
  const [host, setHost] = useState('');
  const [controlHost, setControlHost] = useState('');
  const [fingerprint, setFingerprint] = useState('chrome');
  const [confirmed, setConfirmed] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [result, setResult] = useState<Result>();

  async function run() {
    setBusy(true);
    setConfirmed(false);
    setError('');
    setResult(undefined);
    try {
      const response = await HttpUtil.post('/panel/api/setting/anti-dpi/siberian',
        { host: host.trim(), controlHost: controlHost.trim(), fingerprint, confirmed: true },
        { timeout: 35000 }) as { success?: boolean; msg?: string; obj?: Result };
      if (!response?.success || !response.obj) throw new Error(response?.msg || 'Diagnostic failed');
      setResult(response.obj);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const rows = result ? [
    ...result.baseline.map((a, i) => ({ ...a, key: 'baseline-' + i, phase: 'Baseline ' + (i + 1) })),
    ...(result.burst ?? []).map((a, i) => ({ ...a, key: 'burst-' + i, phase: 'Burst ' + (i + 1) })),
    ...(result.control ? [{ ...result.control, key: 'control', phase: 'Control' }] : []),
  ] : [];

  return <Card size="small" title={l('Active Siberian comparison','Активная проверка Siberian')}>
    <Space orientation="vertical" style={{width:'100%'}}>
      <Alert type="warning" showIcon title={l(
        'Server network only. The active test may temporarily disrupt connectivity; use two owned test hostnames on the same IP with valid certificates.',
        'Только сеть сервера. Активная проверка может временно нарушить доступность; используйте два своих тестовых домена на одном IP с действительными сертификатами.',
      )}/>
      <Typography.Text type="secondary">{l(
        'Adapted comparison: two baseline handshakes, four concurrent handshakes and one control. Not identical to upstream Siberian Check.',
        'Адаптированная проверка: два базовых TLS-соединения, четыре параллельных и одно контрольное. Не полная копия Siberian Check.',
      )}</Typography.Text>
      <Input disabled={busy} value={host} onChange={(e)=>{setHost(e.target.value);setConfirmed(false);}} placeholder="test.example.com" aria-label={l('Target hostname','Проверяемый домен')}/>
      <Input disabled={busy} value={controlHost} onChange={(e)=>{setControlHost(e.target.value);setConfirmed(false);}} placeholder="control.example.com" aria-label={l('Control hostname','Контрольный домен')}/>
      <Select disabled={busy} value={fingerprint} onChange={(v)=>{setFingerprint(v);setConfirmed(false);}} options={['chrome','firefox','safari','android'].map(value=>({value,label:value}))} aria-label="TLS fingerprint"/>
      <Checkbox disabled={busy} checked={confirmed} onChange={(e)=>setConfirmed(e.target.checked)}>{l('I own both test hostnames and accept the risk of interruption','Оба тестовых домена принадлежат мне; принимаю риск временного нарушения доступности')}</Checkbox>
      <Button type="primary" loading={busy} disabled={!confirmed || !host.trim() || !controlHost.trim() || host.trim().toLowerCase()===controlHost.trim().toLowerCase()} onClick={()=>void run()}>{l('Run active test','Запустить активную проверку')}</Button>
      {error && <Alert type="error" title={error}/>}
      {result && <>
        <Typography.Text>{result.host} / {result.controlHost} — {result.ip} ({result.fingerprint})</Typography.Text>
        <Alert type={result.evidence==='asymmetry'?'warning':'info'} title={result.evidence==='asymmetry'
          ? l('TLS timeout asymmetry: compare with another network and server logs','Асимметрия TLS-таймаутов: сравните с другой сетью и журналами сервера')
          : result.evidence==='no-signal' ? l('No signal in this test','В этом тесте признаки не выявлены')
          : l('Inconclusive; verify baseline and control','Неопределённо; проверьте базовые и контрольное соединения')}/>
        <Table size="small" rowKey="key" pagination={false} scroll={{x:true}} dataSource={rows} columns={[
          {title:l('Phase','Этап'),dataIndex:'phase'},
          {title:l('Stage','Уровень'),dataIndex:'stage'},
          {title:l('Outcome','Результат'),dataIndex:'outcome'},
          {title:l('Time, ms','Время, мс'),dataIndex:'durationMs'},
          {title:l('Details','Подробности'),dataIndex:'detail'},
        ]}/>
      </>}
    </Space>
  </Card>;
}
