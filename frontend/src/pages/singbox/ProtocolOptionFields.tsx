import { useId, useState } from 'react';
import { Col, Form, Input, InputNumber, Select } from 'antd';
import fields from './native-outbound-fields.json';
import type { ConfigObject } from './simple-settings';

const labels: Record<string, string> = {
  insecure_concurrency: 'Одновременные соединения',
  quic: 'Использовать QUIC',
  quic_congestion_control: 'Контроль перегрузки QUIC',
  stream_receive_window: 'Окно приёма потока',
  quic_session_receive_window: 'Окно приёма QUIC-сессии',
  extra_headers: 'Дополнительные HTTP-заголовки',
  min_idle_session: 'Минимум свободных сессий',
  zero_rtt_handshake: 'Рукопожатие 0-RTT',
  heartbeat: 'Интервал heartbeat',
  udp_over_stream: 'UDP через поток',
  udp_over_tcp: 'UDP через TCP',
  up_mbps: 'Отправка, Мбит/с',
  down_mbps: 'Загрузка, Мбит/с',
  server: 'Адрес сервера',
  server_port: 'Порт сервера',
  username: 'Логин',
  password: 'Пароль',
  private_key: 'Приватный ключ',
  private_key_path: 'Файл приватного ключа',
  host_key: 'Ключи SSH-сервера',
  host_key_algorithms: 'Алгоритмы ключа SSH',
  client_version: 'Версия клиента',
  interface: 'Интерфейс',
  egress_region: 'Регион выхода',
  data_directory: 'Каталог данных',
  portBindings: 'Порты Mieru',
  key: 'Ключ',
  md5_fingerprint: 'Отпечаток сервера MD5',
  strategy: 'Стратегия балансировки',
  interrupt_exist_connections: 'Прерывать существующие соединения при переключении',
  health_check: 'Проверять работоспособность',
  idle_timeout: 'Тайм-аут бездействия',
  bind_interface: 'Сетевой интерфейс',
  mtu: 'MTU',
  network: 'Сеть (tcp / udp)',
};
const choices: Record<string, string[]> = {
  quic_congestion_control: ['bbr', 'bbr2', 'cubic', 'reno'],
};

// These fields already have dedicated controls in the connection/security forms.
export function dedicatedFields(type: string): Set<string> {
  const result = new Set(['tag', 'type', 'detour', 'connect_timeout', 'tcp_fast_open']);
  if (
    [
      'http',
      'vless',
      'vmess',
      'trojan',
      'hysteria',
      'hysteria2',
      'tuic',
      'anytls',
      'shadowtls',
      'naive',
      'trusttunnel',
      'masque-client',
    ].includes(type)
  )
    result.add('tls');
  if (['vless', 'vmess', 'trojan'].includes(type)) result.add('transport');
  const add = (...keys: string[]) => keys.forEach((key) => result.add(key));
  // A field is hidden here only if a visible control for this protocol exists.
  if (
    [
      'socks',
      'http',
      'shadowsocks',
      'snell',
      'vmess',
      'trojan',
      'naive',
      'ssh',
      'shadowtls',
      'vless',
      'anytls',
      'mieru',
      'sudoku',
      'fptn',
      'trusttunnel',
      'hysteria',
      'hysteria2',
      'tuic',
      'wireguard_legacy',
      'warp',
      'masque-client',
      'openvpn-client',
      'openconnect',
      'pingtunnel',
    ].includes(type)
  ) {
    add('server');
    if (!['mieru', 'openconnect', 'pingtunnel'].includes(type)) add('server_port');
  }
  if (['vless', 'vmess', 'tuic'].includes(type)) add('uuid');
  if (['http', 'socks', 'mieru', 'naive'].includes(type)) add('username');
  if (
    [
      'trojan',
      'shadowsocks',
      'hysteria2',
      'tuic',
      'anytls',
      'shadowtls',
      'http',
      'socks',
      'mieru',
      'naive',
      'ssh',
    ].includes(type)
  )
    add('password');
  if (type === 'naive') add('insecure_concurrency');
  if (type === 'ssh') add('user');
  if (type === 'hysteria') add('auth_str', 'up_mbps', 'down_mbps');
  if (type === 'snell') add('psk', 'version', 'userkey', 'mode');
  if (type === 'shadowtls') add('version');
  if (type === 'shadowsocks') add('method');
  if (type === 'vmess') add('security');
  if (type === 'vless') add('encryption', 'flow');
  if (type === 'hysteria2') add('server_ports', 'hop_interval', 'obfs');
  if (type === 'tuic') add('congestion_control', 'udp_relay_mode');
  if (type === 'anytls') add('idle_session_timeout', 'idle_session_check_interval');
  if (type === 'mieru') add('mtu', 'multiplexing', 'handshake_mode');
  if (['selector', 'urltest', 'balancer'].includes(type)) add('outbounds');
  if (type === 'selector') add('default');
  if (type === 'urltest') add('url', 'interval', 'tolerance');
  if (['masque', 'masque-client'].includes(type))
    add('version', 'path', 'username', 'password', 'mtu', 'use_http2', 'use_ipv6', 'profile');
  return result;
}

function JsonOption({
  name,
  value,
  onChange,
  onValidity,
}: {
  name: string;
  value: unknown;
  onChange: (value: unknown) => void;
  onValidity: (key: string, valid: boolean) => void;
}) {
  const [invalidRaw, setInvalidRaw] = useState<string | null>(null);
  const raw = invalidRaw ?? (value === undefined ? '' : JSON.stringify(value, null, 2));
  const error = invalidRaw !== null;
  return (
    <Form.Item
      validateStatus={error ? 'error' : undefined}
      help={error ? 'Исправьте JSON этого параметра.' : undefined}
      noStyle={!error}
    >
      <Input.TextArea
        aria-label={name}
        value={raw}
        autoSize={{ minRows: 2, maxRows: 8 }}
        placeholder="JSON"
        onChange={(event) => {
          const next = event.target.value;
          try {
            const parsed: unknown = next.trim() ? JSON.parse(next) : undefined;
            setInvalidRaw(null);
            onValidity(name, true);
            onChange(parsed);
          } catch {
            setInvalidRaw(next);
            onValidity(name, false);
          }
        }}
      />
    </Form.Item>
  );
}

export default function ProtocolOptionFields({
  value,
  onChange,
  onValidity,
}: {
  value: ConfigObject;
  onChange: (key: string, value: unknown) => void;
  onValidity: (key: string, valid: boolean) => void;
}) {
  const id = useId();
  const type = String(value.type || '');
  const schema = (fields as Record<string, Record<string, string>>)[type] || {};
  const hidden = dedicatedFields(type);
  return (
    <>
      {Object.entries(schema)
        .filter(([key]) => !hidden.has(key))
        .map(([key, kind]) => {
          const label = labels[key] || key.replaceAll('_', ' ').replaceAll('-', ' ');
          const controlId = `${id}-${key}`;
          const current = value[key];
          const integer = !['rps'].includes(key);
          return (
            <Col
              xs={24}
              sm={kind === 'object' || kind === 'json' || kind === 'array' ? 24 : 12}
              key={key}
            >
              <Form.Item
                label={label}
                htmlFor={controlId}
                extra={
                  key === 'insecure_concurrency'
                    ? 'Пусто или 0 — выбор ядра. Большее число соединений может ослабить маскировку Naive.'
                    : undefined
                }
              >
                {kind === 'number' ? (
                  <InputNumber
                    id={controlId}
                    style={{ width: '100%' }}
                    min={0}
                    precision={integer ? 0 : undefined}
                    value={typeof current === 'number' ? current : null}
                    onChange={(next) => onChange(key, next)}
                  />
                ) : kind === 'boolean' ? (
                  <Select
                    id={controlId}
                    allowClear
                    placeholder="По умолчанию"
                    value={typeof current === 'boolean' ? String(current) : undefined}
                    options={[
                      { value: 'true', label: 'Включено' },
                      { value: 'false', label: 'Выключено' },
                    ]}
                    onChange={(next) =>
                      onChange(key, next === undefined ? undefined : next === 'true')
                    }
                  />
                ) : choices[key] ? (
                  <Select
                    id={controlId}
                    allowClear
                    value={typeof current === 'string' ? current : undefined}
                    options={choices[key].map((v) => ({ value: v, label: v }))}
                    onChange={(next) => onChange(key, next)}
                  />
                ) : kind === 'strings' ? (
                  <Select
                    id={controlId}
                    mode="tags"
                    tokenSeparators={['\n']}
                    value={
                      Array.isArray(current)
                        ? (current as string[])
                        : typeof current === 'string'
                          ? [current]
                          : []
                    }
                    onChange={(next) => onChange(key, next.length ? next : undefined)}
                  />
                ) : ['array', 'object', 'json'].includes(kind) ? (
                  <JsonOption
                    name={label}
                    value={current}
                    onChange={(next) => onChange(key, next)}
                    onValidity={(_name, valid) => onValidity(key, valid)}
                  />
                ) : /password|secret|private_key|token|^key$/.test(key) ? (
                  <Input.Password
                    id={controlId}
                    value={typeof current === 'string' ? current : ''}
                    onChange={(e) => onChange(key, e.target.value)}
                  />
                ) : (
                  <Input
                    id={controlId}
                    value={typeof current === 'string' ? current : ''}
                    onChange={(e) => onChange(key, e.target.value)}
                  />
                )}
              </Form.Item>
            </Col>
          );
        })}
    </>
  );
}
