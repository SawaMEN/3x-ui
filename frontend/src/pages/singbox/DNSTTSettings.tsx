import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Input,
  InputNumber,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import { HttpUtil } from '@/utils';

type Config = {
  domain: string;
  serverIP: string;
  listenPort: number;
  targetPort: number;
  enabled: boolean;
};
type State = Config & {
  installed: boolean;
  running: boolean;
  supported: boolean;
  unsupportedReason?: string;
  publicKey?: string;
  nameServer?: string;
  installedVersion?: string;
  latestVersion?: string;
  updateAvailable: boolean;
};
type Response = { success?: boolean; msg?: string; obj?: State };
type Delegation = {
  zone: string;
  nameServer: string;
  expectedIP?: string;
  addresses: string[];
  nsFound: boolean;
  ipMatches: boolean;
  ready: boolean;
  details: string;
};
type DelegationResponse = { success?: boolean; msg?: string; obj?: Delegation };
const empty: State = {
  domain: '',
  serverIP: '',
  listenPort: 53,
  targetPort: 22,
  enabled: false,
  installed: false,
  running: false,
  supported: true,
  updateAvailable: false,
};
const domainValid = (domain: string) =>
  domain.length <= 253 &&
  domain.split('.').length >= 2 &&
  domain.split('.').every((label) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label));

export default function DNSTTSettings() {
  const [api, holder] = message.useMessage();
  const [state, setState] = useState<State>(empty);
  const [form, setForm] = useState<Config>(empty);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [mode, setMode] = useState<'doh' | 'dot' | 'udp'>('doh');
  const [clientPort, setClientPort] = useState(7000);
  const [resolver, setResolver] = useState('https://cloudflare-dns.com/dns-query');
  const [checkingDelegation, setCheckingDelegation] = useState(false);
  const [delegation, setDelegation] = useState<Delegation | null>(null);

  const reload = useCallback(async (reset = true) => {
    try {
      const res = (await HttpUtil.get('/panel/api/dnstt/status', undefined, {
        silent: true,
      })) as Response;
      if (!res.success || !res.obj)
        throw new Error(res.msg || 'Не удалось получить состояние DNSTT');
      setState(res.obj);
      if (reset) {
        setForm({
          domain: res.obj.domain || '',
          serverIP: res.obj.serverIP || '',
          listenPort: res.obj.listenPort || 53,
          targetPort: res.obj.targetPort || 22,
          enabled: res.obj.enabled,
        });
      }
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void reload();
    }, 0);
    const changed = () => void reload();
    window.addEventListener('dnstt:changed', changed);
    return () => {
      window.clearTimeout(timeout);
      window.removeEventListener('dnstt:changed', changed);
    };
  }, [reload]);
  const patch = (partial: Partial<Config>) => {
    setForm((v) => ({ ...v, ...partial }));
    setDelegation(null);
    setError('');
  };
  const verifyDelegation = async () => {
    setCheckingDelegation(true);
    setError('');
    try {
      const result = (await HttpUtil.get('/panel/api/dnstt/check-delegation', undefined, {
        silent: true,
        timeout: 12000,
      })) as DelegationResponse;
      if (!result.success || !result.obj) {
        throw new Error(result.msg || 'Не удалось проверить NS-делегирование');
      }
      setDelegation(result.obj);
    } catch (cause) {
      setDelegation(null);
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setCheckingDelegation(false);
    }
  };
  const perform = async (path: string, body?: unknown) => {
    setBusy(true);
    setError('');
    try {
      const res = (await HttpUtil.post('/panel/api/dnstt/' + path, body, {
        silent: true,
        timeout: 180000,
      })) as Response;
      if (!res.success || !res.obj) throw new Error(res.msg || 'Действие DNSTT не выполнено');
      setState(res.obj);
      setDelegation(null);
      setForm({
        domain: res.obj.domain || '',
        serverIP: res.obj.serverIP || '',
        listenPort: res.obj.listenPort || 53,
        targetPort: res.obj.targetPort || 22,
        enabled: res.obj.enabled,
      });
      api.success(path === 'remove' ? 'DNSTT удалён вместе с ключами' : 'Операция DNSTT выполнена');
      window.dispatchEvent(new Event('dnstt:changed'));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      await reload(false);
    } finally {
      setBusy(false);
    }
  };

  const nameServer = useMemo(() => {
    const parts = form.domain.trim().toLowerCase().replace(/\.$/, '').split('.');
    return parts.length >= 2 ? 'ns.' + parts.slice(1).join('.') : '';
  }, [form.domain]);
  const publicKey = state.publicKey || '';
  const zone = form.domain.trim().toLowerCase().replace(/\.$/, '');
  // Client resolvers are independent from the DNS listener on the VPS.
  // Reject whitespace/shell characters so the copyable command cannot
  // accidentally become a different shell command.
  const resolverValid =
    mode === 'doh'
      ? /^https:\/\/[a-zA-Z0-9.-]+(?::\d{1,5})?\/[a-zA-Z0-9_./%~-]+$/.test(resolver)
      : /^(?:[a-zA-Z0-9.-]+|\[[0-9a-fA-F:]+\]):\d{1,5}$/.test(resolver);
  const command = [
    'dnstt-client',
    mode === 'doh' ? '-doh' : mode === 'dot' ? '-dot' : '-udp',
    resolverValid ? resolver : '<DNS_RESOLVER>',
    '-pubkey',
    publicKey || '<PUBLIC_KEY>',
    zone || '<TUNNEL_DOMAIN>',
    '127.0.0.1:' + clientPort,
  ].join(' ');
  const changed =
    form.domain !== state.domain ||
    form.serverIP !== state.serverIP ||
    form.listenPort !== state.listenPort ||
    form.targetPort !== state.targetPort;
  const valid =
    domainValid(zone) &&
    form.targetPort >= 1 &&
    form.targetPort <= 65535 &&
    form.listenPort >= 1 &&
    form.listenPort <= 65535 &&
    (!form.serverIP || /^(?:\d{1,3}\.){3}\d{1,3}$/.test(form.serverIP));

  return (
    <Card
      title="DNS-туннель — DNSTT"
      loading={loading}
      extra={
        <Tag color={state.enabled && state.running ? 'success' : 'default'}>
          {state.enabled && state.running
            ? 'Запущен'
            : state.installed
              ? 'Остановлен'
              : 'Не установлен'}
        </Tag>
      }
    >
      {holder}
      <Space orientation="vertical" size={14} style={{ width: '100%' }}>
        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          DNSTT передаёт зашифрованный TCP-трафик через DNS-запросы. Это отдельный туннель, не
          замена обычного DNS и не встроенный VPN. Для работы требуется выделенный поддомен с
          NS-делегированием и локальная служба с собственной авторизацией.
        </Typography.Paragraph>
        {error && (
          <Alert
            type="error"
            showIcon
            title={error}
            action={
              <Button size="small" onClick={() => void reload()}>
                Повторить
              </Button>
            }
          />
        )}
        {state.supported === false && (
          <Alert
            type="warning"
            showIcon
            title="Эта система не поддерживает DNSTT"
            description={
              state.unsupportedReason || 'Требуется Linux с systemd и архитектура amd64 или arm64.'
            }
          />
        )}

        {!state.installed ? (
          <Space orientation="vertical" size={8}>
            <Typography.Text>Сначала установите отдельный сервер DNSTT.</Typography.Text>
            <Button
              type="primary"
              loading={busy}
              disabled={state.supported === false}
              onClick={() => void perform('install')}
            >
              Установить DNSTT
            </Button>
            <Typography.Text type="secondary">
              Скачивается закреплённый проверяемый SHA-256 бинарник из GitHub Releases зеркала
              NNdroid. Это сторонняя сборка, не официальный релиз автора протокола.
            </Typography.Text>
          </Space>
        ) : (
          <>
            <div>
              <Typography.Text strong>1. Поддомен DNS-туннеля</Typography.Text>
              <Input
                aria-label="Домен DNSTT"
                style={{ marginTop: 6 }}
                placeholder="t.example.com"
                value={form.domain}
                disabled={busy}
                onChange={(e) => patch({ domain: e.target.value })}
              />
              <Typography.Text type="secondary">
                Укажите отдельную зону, например t.example.com. Она будет делегирована на ваш
                сервер.
              </Typography.Text>
            </div>
            <div>
              <Typography.Text strong>2. Публичный IPv4 сервера</Typography.Text>
              <Input
                aria-label="IPv4 DNSTT"
                style={{ marginTop: 6 }}
                placeholder="203.0.113.2"
                value={form.serverIP}
                disabled={busy}
                onChange={(e) => patch({ serverIP: e.target.value })}
              />
              <Typography.Text type="secondary">
                Нужен для подсказок DNS-записей; панель не изменяет DNS у регистратора.
              </Typography.Text>
            </div>
            {domainValid(zone) && (
              <Alert
                type="info"
                showIcon
                title="Добавьте записи у DNS-провайдера"
                description={
                  <Space orientation="vertical" size={3}>
                    <Typography.Text
                      code
                      copyable
                    >{`A  ${nameServer}  ${form.serverIP || '<SERVER_IP>'}`}</Typography.Text>
                    <Typography.Text code copyable>{`NS  ${zone}  ${nameServer}`}</Typography.Text>
                    <Typography.Text type="secondary">
                      Записи создаются в родительской зоне домена; UDP/53 должен вести к серверу
                      DNSTT.
                    </Typography.Text>
                  </Space>
                }
              />
            )}
            <div>
              <Typography.Text strong>3. Подключение к локальному TCP-сервису</Typography.Text>
              <Space wrap style={{ marginTop: 6 }}>
                <Typography.Text>Входящий DNS UDP-порт</Typography.Text>
                <InputNumber
                  aria-label="UDP-порт DNSTT"
                  min={1}
                  max={65535}
                  value={form.listenPort}
                  disabled={busy}
                  onChange={(v) => patch({ listenPort: v ?? 53 })}
                />
                <Typography.Text>Локальный TCP-порт</Typography.Text>
                <InputNumber
                  aria-label="TCP-цель DNSTT"
                  min={1}
                  max={65535}
                  value={form.targetPort}
                  disabled={busy}
                  onChange={(v) => patch({ targetPort: v ?? 22 })}
                />
              </Space>
              <Alert
                type="warning"
                showIcon
                style={{ marginTop: 8 }}
                title="Убедитесь, что локальный сервис требует авторизацию"
                description="По умолчанию DNSTT направляет подключения на 127.0.0.1:22 (SSH). Публичный ключ DNSTT подтверждает сервер, но не авторизует клиентов: не направляйте туннель на открытый SOCKS/HTTP-прокси. Порт 53/UDP нужен для NS-делегирования; при выборе другого порта обеспечьте перенаправление внешнего UDP/53 самостоятельно."
              />
            </div>
            {state.enabled && state.running && !changed && (
              <Space orientation="vertical" size={8} style={{ width: '100%' }}>
                <Button
                  loading={checkingDelegation}
                  disabled={busy}
                  onClick={() => void verifyDelegation()}
                >
                  Проверить NS-делегирование домена
                </Button>
                {delegation && (
                  <Alert
                    type={delegation.ready ? 'success' : 'warning'}
                    showIcon
                    title={
                      delegation.ready
                        ? 'DNS-делегирование найдено'
                        : 'DNS-делегирование пока не готово'
                    }
                    description={
                      <Space orientation="vertical" size={3}>
                        <Typography.Text>{delegation.details}</Typography.Text>
                        <Typography.Text code>{delegation.nameServer}</Typography.Text>
                        {delegation.addresses.length > 0 && (
                          <Typography.Text>
                            Адреса NS: {delegation.addresses.join(', ')}
                          </Typography.Text>
                        )}
                        <Typography.Text type="secondary">
                          Проверка NS/A-записей не подтверждает доступность входящего UDP/53 или
                          работу туннеля у оператора.
                        </Typography.Text>
                      </Space>
                    }
                  />
                )}
              </Space>
            )}
            {changed && state.enabled && (
              <Alert type="warning" showIcon title="Есть неприменённые изменения" />
            )}
            <Space wrap>
              <Button
                type="primary"
                loading={busy}
                disabled={!valid || state.supported === false}
                onClick={() => void perform('setup', form)}
              >
                {state.enabled ? 'Сохранить и перезапустить' : 'Создать и включить DNSTT'}
              </Button>
              {state.enabled && (
                <Button danger disabled={busy} onClick={() => void perform('disable')}>
                  Отключить
                </Button>
              )}
              {changed && (
                <Button disabled={busy} onClick={() => void reload()}>
                  Отменить изменения
                </Button>
              )}
              {state.updateAvailable && (
                <Button disabled={busy} onClick={() => void perform('update')}>
                  Обновить DNSTT
                </Button>
              )}
              <Button disabled={busy} onClick={() => void reload()}>
                Обновить состояние
              </Button>
            </Space>
            {publicKey && (
              <Alert
                type="success"
                showIcon
                title="Публичный ключ сервера"
                description={
                  <Typography.Text code copyable>
                    {publicKey}
                  </Typography.Text>
                }
              />
            )}
            {publicKey && state.enabled && state.running && (
              <div>
                <Typography.Text strong>4. Подключение клиента DNSTT</Typography.Text>
                <Space orientation="vertical" style={{ width: '100%', marginTop: 8 }}>
                  <Select
                    aria-label="DNS-транспорт клиента"
                    value={mode}
                    style={{ width: '100%' }}
                    options={[
                      { value: 'doh', label: 'DNS over HTTPS — DoH' },
                      { value: 'dot', label: 'DNS over TLS — DoT' },
                      { value: 'udp', label: 'Обычный UDP DNS' },
                    ]}
                    onChange={(next) => {
                      setMode(next);
                      setResolver(
                        next === 'doh'
                          ? 'https://cloudflare-dns.com/dns-query'
                          : next === 'dot'
                            ? '1.1.1.1:853'
                            : '1.1.1.1:53',
                      );
                    }}
                  />
                  <Input
                    aria-label="DNS-резолвер клиента"
                    value={resolver}
                    status={!resolverValid ? 'error' : undefined}
                    placeholder={
                      mode === 'doh' ? 'https://cloudflare-dns.com/dns-query' : '1.1.1.1:853'
                    }
                    onChange={(event) => setResolver(event.target.value.trim())}
                  />
                  {!resolverValid && (
                    <Typography.Text type="danger">
                      Проверьте адрес DNS-резолвера клиента. Для DoH нужен HTTPS URL, для DoT/UDP —
                      адрес:порт.
                    </Typography.Text>
                  )}
                  <Space>
                    <Typography.Text>Локальный TCP-порт клиента:</Typography.Text>
                    <InputNumber
                      aria-label="TCP-порт клиента"
                      min={1}
                      max={65535}
                      value={clientPort}
                      onChange={(v) => setClientPort(v ?? 7000)}
                    />
                  </Space>
                  <Typography.Text
                    code
                    copyable={resolverValid}
                    style={{ overflowWrap: 'anywhere' }}
                  >
                    {command}
                  </Typography.Text>
                  <Typography.Text type="secondary">
                    Установите dnstt-client на клиентском устройстве, запустите команду и подключите
                    приложение к 127.0.0.1:{clientPort}. Для SSH-цели используйте SSH-клиент с
                    авторизацией на этом порту. Параметры клиента — DoH/DoT/UDP, а не дополнительные
                    публичные серверы на VPS.
                  </Typography.Text>
                </Space>
              </div>
            )}
            <Button
              danger
              disabled={busy}
              onClick={() => {
                if (window.confirm('Удалить DNSTT и безвозвратно уничтожить серверные ключи?')) {
                  void perform('remove');
                }
              }}
            >
              Удалить DNSTT и ключи
            </Button>
          </>
        )}
      </Space>
    </Card>
  );
}
