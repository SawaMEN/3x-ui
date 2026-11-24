import { Alert, Button, Form, InputNumber, Select, Space, Switch } from 'antd';
import { useEffect, useRef, useState } from 'react';
import { HttpUtil } from '@/utils';
import {
  Zapret2OutboundSettingsSchema,
  type Zapret2Calibration,
} from '@/schemas/protocols/outbound/zapret2';
import type { ConfigObject } from './simple-settings';

export default function Zapret2Fields({
  value,
  onChange,
  onValidity,
}: {
  value: ConfigObject;
  onChange: (value: ConfigObject) => void;
  onValidity: (valid: boolean) => void;
}) {
  const [installed, setInstalled] = useState(false);
  const [statusLoaded, setStatusLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [level, setLevel] = useState<'quick' | 'standard' | 'force'>('standard');
  const [repeats, setRepeats] = useState(2);
  const [ipv6, setIpv6] = useState(false);
  const [report, setReport] = useState('');
  const active = useRef(true);
  const parsed = Zapret2OutboundSettingsSchema.safeParse(value);
  const settings = parsed.success ? parsed.data : Zapret2OutboundSettingsSchema.parse({});
  const valid = parsed.success && (settings.tcp || settings.udp) && installed && !busy;
  const validity = useRef(onValidity);
  const change = useRef(onChange);
  useEffect(() => {
    validity.current = onValidity;
    change.current = onChange;
  }, [onValidity, onChange]);
  useEffect(() => {
    validity.current(valid);
  }, [valid]);
  useEffect(() => {
    active.current = true;
    const refresh = () => {
      void HttpUtil.get('/panel/api/setting/zapret2/status', undefined, { silent: true })
        .then((response: unknown) => {
          const status = response as { success?: boolean; obj?: { installed?: boolean } };
          if (active.current) {
            setInstalled(status.success === true && status.obj?.installed === true);
            setStatusLoaded(true);
          }
        })
        .catch(() => {
          if (active.current) {
            setInstalled(false);
            setStatusLoaded(true);
          }
        });
    };
    refresh();
    window.addEventListener('zapret2:changed', refresh);
    return () => {
      active.current = false;
      window.removeEventListener('zapret2:changed', refresh);
    };
  }, []);
  const patch = (next: Partial<typeof settings>) => change.current({ ...settings, ...next });
  async function diagnose() {
    const domains = settings.testDomains;
    if (domains.length < 1 || domains.length > 12) {
      setReport('Укажите от 1 до 12 тестовых доменов.');
      return;
    }
    setBusy(true);
    setReport('');
    try {
      const response = (await HttpUtil.post(
        '/panel/api/setting/zapret2/diagnose',
        { domains, scanLevel: level, repeats, checkQuic: settings.udp, checkIpv6: ipv6 },
        { timeout: 1860000, headers: { 'Content-Type': 'application/json' } },
      )) as {
        success?: boolean;
        msg?: string;
        obj?: { summary?: string; warnings?: string[]; calibration?: Zapret2Calibration };
      };
      if (!active.current) return;
      if (!response.success) throw new Error(response.msg || 'Ошибка диагностики Zapret2');
      const result = response.obj;
      if (result?.calibration) patch({ profile: 'auto', calibration: result.calibration });
      setReport(
        [
          result?.calibration ? 'Стратегия подобрана. Сохраните подключение.' : 'Настройки не изменены.',
          ...(result?.warnings ?? []),
          result?.summary || '',
        ].join('\n'),
      );
    } catch (error) {
      if (active.current) setReport(error instanceof Error ? error.message : String(error));
    } finally {
      if (active.current) setBusy(false);
    }
  }
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      <Alert
        type={installed ? 'info' : 'warning'}
        showIcon
        title={
          installed
            ? 'Zapret2 установлен'
            : statusLoaded
              ? 'Zapret2 недоступен'
              : 'Проверка компонента'
        }
        description="Виртуальный локальный выход. Для применения выберите его в правиле маршрутизации. Не назначайте выходом по умолчанию."
      />
      <Form.Item label="Локальный SOCKS-порт">
        <InputNumber
          disabled={busy}
          min={1025}
          max={65535}
          precision={0}
          value={settings.port}
          onChange={(port) => port !== null && patch({ port })}
        />
      </Form.Item>
      <Form.Item label="Профиль">
        <Select
          disabled={busy}
          value={settings.profile}
          options={['auto', 'safe', 'balanced', 'aggressive'].map((v) => ({ value: v, label: v }))}
          onChange={(profile: typeof settings.profile) => patch({ profile })}
        />
      </Form.Item>
      <Space wrap>
        TCP <Switch disabled={busy} checked={settings.tcp} onChange={(tcp) => patch({ tcp })} />
        UDP/QUIC <Switch disabled={busy} checked={settings.udp} onChange={(udp) => patch({ udp })} />
      </Space>
      <Form.Item label="Тестовые домены">
        <Select
          mode="tags"
          disabled={busy}
          tokenSeparators={[',', ' ']}
          value={settings.testDomains}
          onChange={(testDomains: string[]) => patch({ testDomains })}
        />
      </Form.Item>
      <Space wrap>
        <Select
          disabled={busy}
          value={level}
          onChange={setLevel}
          options={[
            { value: 'quick', label: 'Быстрая' },
            { value: 'standard', label: 'Полная' },
            { value: 'force', label: 'Максимальная' },
          ]}
        />
        Повторы{' '}
        <InputNumber
          disabled={busy}
          min={1}
          max={3}
          precision={0}
          value={repeats}
          onChange={(v) => setRepeats(v ?? 2)}
        />
        IPv6 <Switch disabled={busy} checked={ipv6} onChange={setIpv6} />
      </Space>
      {settings.calibration && (
        <Alert type="success" title={`Стратегия проверена: ${settings.calibration.testedAt}`} />
      )}
      <Button disabled={!installed || busy} loading={busy} onClick={() => void diagnose()}>
        Автоматически проверить и подобрать стратегию
      </Button>
      {report && (
        <pre style={{ whiteSpace: 'pre-wrap', maxHeight: 320, overflow: 'auto' }}>{report}</pre>
      )}
    </Space>
  );
}
