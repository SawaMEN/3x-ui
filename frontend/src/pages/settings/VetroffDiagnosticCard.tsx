import { useRef, useState } from 'react';
import { Alert, Button, Card, Space, Table, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { compareClientDiagnostics, parseClientDiagnostic, type ClientDiagnostic, type Comparison } from './vetroff-diagnostic-report';

export default function VetroffDiagnosticCard() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => ru ? russian : en;
  const [physical, setPhysical] = useState<ClientDiagnostic>();
  const [tunnel, setTunnel] = useState<ClientDiagnostic>();
  const [error, setError] = useState('');
  const request = useRef(0);

  async function load(file: File) {
    const id = ++request.current;
    setError('');
    try {
      if (file.size > 65536) throw new Error('Report too large');
      const r = parseClientDiagnostic(await file.text());
      if (id !== request.current) return;
      if (r.scope === 'client-physical') setPhysical(r);
      else setTunnel(r);
    } catch {
      if (id === request.current) setError(l('Unsupported or malformed VetrOFF-Core report', 'Некорректный или неподдерживаемый отчёт VetrOFF-Core'));
    }
  }
  const result = physical && tunnel ? compareClientDiagnostics(physical, tunnel) : undefined;
  const messages: Record<Comparison, string> = {
    'not-comparable': l('Different targets, fingerprints or time window. Repeat both tests under comparable conditions.', 'Различаются цели, отпечатки или время проверки. Повторите оба теста в сопоставимых условиях.'),
    'physical-only-signal': l('Asymmetry only on the physical path. Compare with another access network and server logs.', 'Асимметрия только в физической сети. Сравните с другой сетью доступа и журналами сервера.'),
    'tunnel-only-signal': l('Asymmetry only through the tunnel. Inspect its egress, routing and server logs.', 'Асимметрия только через туннель. Проверьте выходной узел, маршрутизацию и журналы сервера.'),
    'both-signal': l('Both paths show asymmetry. Investigate shared server and TLS configuration.', 'Асимметрия на обоих путях. Проверьте общий сервер и конфигурацию TLS.'),
    'neither-signal': l('No signal in these tests. Other restrictions or outages are not excluded.', 'В этих тестах признаки не выявлены. Другие ограничения и сбои не исключены.'),
    'inconclusive': l('At least one comparison is incomplete or failed. Inspect the individual attempts.', 'Хотя бы один тест неполный или завершился ошибкой. Проверьте отдельные попытки.'),
  };

  const reports = [physical, tunnel].filter((r): r is ClientDiagnostic => r !== undefined);
  return <Card size="small" title={l('VetrOFF-Core client diagnostics', 'Диагностика клиента VetrOFF-Core')}>
    <Space orientation="vertical" style={{width:'100%'}}>
      <Alert type="info" showIcon title={l(
        'Local file comparison only. The reports do not prove operator filtering or that both tests used the same access network. No settings are applied.',
        'Локальное сравнение файлов. Отчёты не доказывают фильтрацию оператором или использование одной сети доступа. Настройки не применяются.',
      )}/>
      <Typography.Text>{l('Import one physical-path and one tunnel-path JSON report (v1).', 'Импортируйте по одному JSON-отчёту физической сети и VPN-туннеля (v1).')}</Typography.Text>
      <input type="file" accept=".json,application/json" aria-label={l('Import VetrOFF report','Импорт отчёта VetrOFF')} onChange={(e)=>{
        const file=e.target.files?.[0]; e.target.value=''; if(file) void load(file);
      }}/>
      <Button onClick={()=>{++request.current;setPhysical(undefined);setTunnel(undefined);setError('');}}>{l('Clear reports','Очистить отчёты')}</Button>
      {error && <Alert type="error" title={error}/>}
      {reports.map((r)=><Card key={r.scope} size="small" title={r.scope==='client-physical'?l('Physical network','Физическая сеть'):l('VPN tunnel','VPN-туннель')}>
        <Space orientation="vertical" style={{width:'100%'}}>
          <Typography.Text>{r.host} / {r.controlHost} — {r.ip || '—'}</Typography.Text>
          <Typography.Text type="secondary">{r.checkedAt} · {r.fingerprint}</Typography.Text>
          <Typography.Text>{r.evidence==='asymmetry'?l('TLS asymmetry','Асимметрия TLS'):r.evidence==='no-signal'?l('No signal','Признаки не выявлены'):l('Inconclusive','Неопределённо')}</Typography.Text>
          <Table size="small" pagination={false} scroll={{x:true}} rowKey="key" dataSource={[
            ...r.baseline.map((a,i)=>({...a,key:'b'+i,phase:l('Baseline','Базовая')+' '+(i+1)})),
            ...r.burst.map((a,i)=>({...a,key:'p'+i,phase:l('Parallel','Параллельная')+' '+(i+1)})),
            ...(r.control?[{...r.control,key:'c',phase:l('Control','Контрольная')}]:[]),
          ]} columns={[
            {title:l('Phase','Этап'),dataIndex:'phase'},
            {title:l('Stage','Уровень'),dataIndex:'stage'},
            {title:l('Outcome','Результат'),dataIndex:'outcome'},
            {title:l('Time, ms','Время, мс'),dataIndex:'durationMs'},
          ]}/>
        </Space>
      </Card>)}
      {result && <Alert type={result==='physical-only-signal'||result==='tunnel-only-signal'||result==='both-signal'?'warning':'info'} title={messages[result]}/>}
    </Space>
  </Card>;
}
