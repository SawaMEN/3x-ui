import { Alert, Button, Form, InputNumber, Select, Space, Switch } from 'antd';
import { useEffect, useRef, useState } from 'react';
import { HttpUtil } from '@/utils';
import { useRequestSlot } from '@/hooks/useRequestSlot';
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
  const diagnostic = useRequestSlot();
  const parsed = Zapret2OutboundSettingsSchema.safeParse(value);
  const settings = parsed.success ? parsed.data : Zapret2OutboundSettingsSchema.parse({});
  const valid = parsed.success && (settings.tcp || settings.udp) && installed && !busy;
  const validity = useRef(onValidity);
  const change = useRef(onChange);
  const latestSettings = useRef(settings);
  useEffect(() => {
    validity.current = onValidity;
    change.current = onChange;
  }, [onValidity, onChange]);
  useEffect(() => {
    latestSettings.current = settings;
  }, [settings]);
  useEffect(() => {
    validity.current(valid);
  }, [valid]);
  useEffect(() => {
    let current: AbortController | undefined;
    const refresh = () => {
      current?.abort();
      const controller = new AbortController();
      current = controller;
      void HttpUtil.get('/panel/api/setting/zapret2/status', undefined, {
        silent: true,
        signal: controller.signal,
      })
        .then((response: unknown) => {
          const status = response as { success?: boolean; obj?: { installed?: boolean } };
          if (!controller.signal.aborted) {
            setInstalled(status.success === true && status.obj?.installed === true);
            setStatusLoaded(true);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted) {
            setInstalled(false);
            setStatusLoaded(true);
          }
        });
    };
    refresh();
    window.addEventListener('zapret2:changed', refresh);
    return () => {
      current?.abort();
      window.removeEventListener('zapret2:changed', refresh);
    };
  }, []);
  const patch = (next: Partial<typeof settings>) => change.current({ ...settings, ...next });
  async function diagnose() {
    if (diagnostic.isPending()) return;
    const domains = settings.testDomains;
    if (domains.length < 1 || domains.length > 12) {
      setReport('Укажите от 1 до 12 тестовых доменов.');
      return;
    }
    const controller = diagnostic.begin();
    setBusy(true);
    setReport('');
    try {
      const response = (await HttpUtil.post(
        '/panel/api/setting/zapret2/diagnose',
        { domains, scanLevel: level, repeats, checkQuic: settings.udp, checkIpv6: ipv6 },
        {
          timeout: 1860000,
          headers: { 'Content-Type': 'application/json' },
          signal: controller.signal,
          silent: true,
        },
      )) as {
        success?: boolean;
        msg?: string;
        obj?: { summary?: string; warnings?: string[]; calibration?: Zapret2Calibration };
      };
      if (controller.signal.aborted) return;
      if (!response.success) throw new Error(response.msg || 'Ошибка диагностики Zapret2');
      const result = response.obj;
      if (result?.calibration) {
        change.current({ ...latestSettings.current, profile: 'auto', calibration: result.calibration });
      }
      setReport(
        [
          result?.calibration ? 'Стратегия подобрана. Сохраните подключение.' : 'Настройки не изменены.',
          ...(result?.warnings ?? []),
          result?.summary || '',
        ].join('\n'),
      );
    } catch (error) {
      if (!controller.signal.aborted) setReport(error instanceof Error ? error.message : String(error));
    } finally {
      if (diagnostic.finish(controller)) setBusy(false);
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
