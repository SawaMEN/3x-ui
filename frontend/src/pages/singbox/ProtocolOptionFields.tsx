import { useId, useState } from 'react';
import { Button, Card, Col, Collapse, Form, Input, InputNumber, Row, Select, Space } from 'antd';
import fields from './native-outbound-fields.json';
import { object, type ConfigObject } from './simple-settings';
import {
  optionVisible,
  patchOption,
  type OptionDescriptor,
  type OptionSchema,
} from './protocol-option-model';

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
  network: 'Сеть',
  peers: 'Узлы',
  address: 'Адрес',
  port: 'Порт',
  portRange: 'Диапазон портов',
  protocol: 'Транспорт',
  allowed_ips: 'Разрешённые сети',
  public_key: 'Публичный ключ',
  pre_shared_key: 'Общий ключ',
  preshared_key: 'Общий ключ',
  persistent_keepalive_interval: 'Период поддержки соединения',
  listen_port: 'Локальный порт',
  name: 'Имя',
  awg: 'Маскировка AmneziaWG',
  profile: 'Профиль WARP',
  recreate: 'Пересоздать профиль',
  license: 'Лицензия',
  transports: 'Транспорты',
  type: 'Тип',
  url: 'Ссылка',
  dial: 'Адрес подключения',
  priority: 'Приоритет',
  server_public_key: 'Публичный ключ сервера',
  server_disco_key: 'Ключ обнаружения сервера',
  domain: 'Домен',
  pubkey: 'Публичный ключ',
  resolvers: 'DNS-резолверы',
  script_keys: 'Ключи скриптов',
  tunnel_key: 'Ключ туннеля',
  google_host: 'Сервер Google',
  obfs: 'Маскировка',
  obfs_mode: 'Маскировка Snell',
  obfs_host: 'Домен маскировки',
  min_packet_size: 'Минимальный размер пакета',
  max_packet_size: 'Максимальный размер пакета',
  multiplexing: 'Мультиплексирование',
  handshake_mode: 'Рукопожатие',
  idle_session_timeout: 'Тайм-аут свободной сессии',
  idle_session_check_interval: 'Проверка свободных сессий',
  hop_interval: 'Интервал смены портов',
  hop_interval_max: 'Максимальный интервал смены портов',
  congestion_control: 'Контроль перегрузки',
  udp_relay_mode: 'Передача UDP',
  connect_timeout: 'Тайм-аут подключения',
  tcp_fast_open: 'TCP Fast Open',
  domain_resolver: 'DNS для подключения',
  auth_group: 'Группа авторизации',
  flavor: 'Совместимость с сервером',
  user_agent: 'User-Agent',
  cookie: 'Cookie авторизации',
  token: 'Токен',
  auth_token: 'Токен авторизации',
  httpmask: 'HTTP-маскировка',
  padding_min: 'Минимальное дополнение',
  padding_max: 'Максимальное дополнение',
  custom_tables: 'Таблицы маскировки',
  aead: 'Шифрование',
  ascii: 'Алфавит',
  secret: 'Секрет',
  context: 'Контекст подключения',
  codec: 'Кодек',
  system: 'Системный интерфейс',
  useIntegratedTun: 'Встроенный TUN',
  on_demand: 'Подключаться по требованию',
  mode: 'Режим',
  reuse: 'Повторно использовать соединения',
};
// These fields already have dedicated controls in the connection/security forms.
export function dedicatedFields(type: string): Set<string> {
  const result = new Set(['tag', 'type', 'detour']);
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
  if (type === 'mieru') add('server_port');
  if (type === 'ssh') add('user');
  if (type === 'hysteria') add('auth_str', 'up_mbps', 'down_mbps');
  // Version-specific Snell fields already have one dedicated editor each.
  // Network and connection reuse remain optional under advanced settings.
  if (type === 'snell') add('psk', 'userkey', 'version', 'mode', 'obfs_mode', 'obfs_host');
  if (type === 'shadowtls') add('version');
  if (type === 'shadowsocks') add('method');
  if (type === 'vmess') add('security');
  if (type === 'vless') add('encryption', 'flow');
  if (type === 'hysteria2') add('server_ports');
  if (['selector', 'urltest', 'balancer'].includes(type)) add('outbounds');
  if (type === 'selector') add('default');
  if (type === 'urltest') add('url', 'interval', 'tolerance');
  if (['masque', 'masque-client'].includes(type))
    add('version', 'path', 'username', 'password', 'mtu', 'use_http2', 'use_ipv6', 'udp_timeout');
  if (type === 'masque') add('udp_keepalive_period', 'reconnect_delay');
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

const registry = fields as unknown as {
  protocols: Record<string, OptionSchema>;
  definitions: Record<string, OptionSchema>;
  dialerFields: string[];
};

function HeadersOption({
  value,
  label,
  onChange,
  onValidity,
}: {
  value: unknown;
  label: string;
  onChange: (value: unknown) => void;
  onValidity: (valid: boolean) => void;
}) {
  const [rows, setRows] = useState(() =>
    Object.entries(object(value)).map(([name, contents]) => ({
      name,
      contents: Array.isArray(contents) ? contents.join('\n') : String(contents),
    })),
  );
  const [invalid, setInvalid] = useState(false);
  function update(next: typeof rows) {
    setRows(next);
    const names = next.map((row) => row.name.trim());
    const valid =
      names.every((name) => /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name)) &&
      new Set(names.map((name) => name.toLowerCase())).size === names.length;
    setInvalid(!valid);
    onValidity(valid);
    if (valid)
      onChange(
        next.length
          ? Object.fromEntries(next.map((row) => [row.name.trim(), row.contents.split('\n')]))
          : undefined,
      );
  }
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      {rows.map((row, index) => (
        <Space.Compact key={index} style={{ width: '100%' }}>
          <Input
            aria-label={`${label}: имя ${index + 1}`}
            placeholder="User-Agent"
            value={row.name}
            onChange={(e) =>
              update(rows.map((r, i) => (i === index ? { ...r, name: e.target.value } : r)))
            }
          />
          <Input.TextArea
            aria-label={`${label}: значение ${index + 1}`}
            placeholder="Значение"
            autoSize={{ minRows: 1, maxRows: 4 }}
            value={row.contents}
            onChange={(e) =>
              update(rows.map((r, i) => (i === index ? { ...r, contents: e.target.value } : r)))
            }
          />
          <Button
            aria-label={`${label}: удалить ${index + 1}`}
            onClick={() => update(rows.filter((_, i) => i !== index))}
          >
            Удалить
          </Button>
        </Space.Compact>
      ))}
      <Button onClick={() => update([...rows, { name: '', contents: '' }])}>
        Добавить заголовок
      </Button>
      {invalid && (
        <span role="alert">Укажите неповторяющееся имя HTTP-заголовка без пробелов.</span>
      )}
    </Space>
  );
}

function OptionControl({
  descriptor,
  value,
  label,
  path,
  onChange,
  onValidity,
  depth,
}: {
  descriptor: OptionDescriptor;
  value: unknown;
  label: string;
  path: string;
  onChange: (next: unknown) => void;
  onValidity: (key: string, valid: boolean) => void;
  depth: number;
}) {
  const id = useId();
  const props = { id, 'aria-label': label };
  const options = descriptor.enum?.map((v) => ({
    value: v,
    label: String(v).replace(/^MULTIPLEXING_|^HANDSHAKE_/, ''),
  }));
  if (
    options &&
    (typeof value === 'number' || typeof value === 'string') &&
    !options.some((option) => option.value === value)
  )
    options.push({ value, label: String(value) });
  if (options)
    return (
      <Select
        {...props}
        allowClear
        placeholder="Автоматически"
        value={typeof value === 'number' || typeof value === 'string' ? value : undefined}
        options={options}
        onChange={onChange}
      />
    );
  if (descriptor.kind === 'number')
    return (
      <InputNumber
        {...props}
        style={{ width: '100%' }}
        placeholder="Автоматически"
        min={descriptor.min}
        max={descriptor.max}
        precision={descriptor.integer ? 0 : undefined}
        value={typeof value === 'number' ? value : null}
        onChange={onChange}
      />
    );
  if (descriptor.kind === 'boolean')
    return (
      <Select
        {...props}
        allowClear
        placeholder="Автоматически"
        value={typeof value === 'boolean' ? String(value) : undefined}
        options={[
          { value: 'true', label: 'Включено' },
          { value: 'false', label: 'Выключено' },
        ]}
        onChange={(next) => onChange(next === undefined ? undefined : next === 'true')}
      />
    );
  if (descriptor.kind === 'strings' || descriptor.kind === 'network')
    return (
      <Select
        {...props}
        mode={descriptor.kind === 'network' ? 'multiple' : 'tags'}
        tokenSeparators={['\n']}
        options={
          descriptor.kind === 'network'
            ? ['tcp', 'udp'].map((v) => ({ value: v, label: v.toUpperCase() }))
            : undefined
        }
        value={
          Array.isArray(value) ? (value as string[]) : typeof value === 'string' ? [value] : []
        }
        onChange={(next) => onChange(next.length ? next : undefined)}
      />
    );
  if (descriptor.kind === 'headers')
    return (
      <HeadersOption
        label={label}
        value={value}
        onChange={onChange}
        onValidity={(valid) => onValidity(path, valid)}
      />
    );
  const childSchema = descriptor.ref ? registry.definitions[descriptor.ref] : undefined;
  if (descriptor.kind === 'object' && childSchema && depth < 6) {
    if (typeof value === 'string' || typeof value === 'boolean')
      return (
        <Space orientation="vertical" style={{ width: '100%' }}>
          {typeof value === 'boolean' ? (
            <Select
              {...props}
              value={String(value)}
              options={[
                { value: 'true', label: 'Включено' },
                { value: 'false', label: 'Выключено' },
              ]}
              onChange={(next) => onChange(next === 'true')}
            />
          ) : (
            <Input {...props} value={value} onChange={(e) => onChange(e.target.value)} />
          )}
          <Button
            onClick={() =>
              onChange(typeof value === 'boolean' ? { enabled: value } : { server: value })
            }
          >
            Настроить подробно
          </Button>
        </Space>
      );
    return (
      <Card size="small">
        <OptionGrid
          schema={childSchema}
          value={object(value)}
          prefix={path}
          labelPrefix={label}
          depth={depth + 1}
          onChange={(key, next) => {
            const updated = patchOption(object(value), key, next, childSchema);
            onChange(Object.keys(updated).length ? updated : undefined);
          }}
          onValidity={onValidity}
        />
      </Card>
    );
  }
  if (descriptor.kind === 'array' && descriptor.item && depth < 6) {
    // Some native listables also accept one scalar. Keep that representation
    // intact until the user edits it, then write the equivalent array.
    const values = Array.isArray(value) ? value : value == null ? [] : [value];
    return (
      <Space orientation="vertical" style={{ width: '100%' }}>
        {values.map((item, index) => (
          <Card
            size="small"
            key={index}
            title={`${label} ${index + 1}`}
            extra={
              <Button
                onClick={() => {
                  onValidity(path, true);
                  onChange(values.filter((_, i) => i !== index));
                }}
              >
                Удалить
              </Button>
            }
          >
            <OptionControl
              descriptor={descriptor.item!}
              value={item}
              label={`${label} ${index + 1}`}
              path={`${path}.${index}`}
              depth={depth + 1}
              onChange={(next) =>
                onChange(values.map((existing, i) => (i === index ? next : existing)))
              }
              onValidity={(_key, valid) => onValidity(path, valid)}
            />
          </Card>
        ))}
        <Button
          onClick={() =>
            onChange([
              ...values,
              descriptor.item!.ref === 'MieruPortBinding'
                ? { protocol: 'TCP', port: 443 }
                : descriptor.item!.ref === 'OpenFluxTransportOptions'
                  ? { type: 'direct', priority: 100 }
                  : descriptor.item!.kind === 'object'
                    ? {}
                    : descriptor.item!.kind === 'number'
                      ? 0
                      : '',
            ])
          }
        >
          Добавить: {label}
        </Button>
      </Space>
    );
  }
  if (descriptor.kind === 'string') {
    const inputProps = {
      ...props,
      placeholder: descriptor.required ? 'Из ссылки или конфигурации сервера' : 'Автоматически',
      value: typeof value === 'string' ? value : '',
      onChange: (event: React.ChangeEvent<HTMLInputElement>) => onChange(event.target.value),
    };
    return /password|secret|private_key|preshared|pre_shared|token|(?:^|\.)key$/.test(path) ? (
      <Input.Password {...inputProps} />
    ) : (
      <Input {...inputProps} />
    );
  }
  return (
    <JsonOption
      name={label}
      value={value}
      onChange={onChange}
      onValidity={(_name, valid) => onValidity(path, valid)}
    />
  );
}

function OptionGrid({
  schema,
  value,
  prefix = '',
  labelPrefix = '',
  depth = 0,
  onChange,
  onValidity,
}: {
  schema: OptionSchema;
  value: ConfigObject;
  prefix?: string;
  labelPrefix?: string;
  depth?: number;
  onChange: (key: string, next: unknown) => void;
  onValidity: (key: string, valid: boolean) => void;
}) {
  return (
    <Row gutter={16}>
      {Object.entries(schema)
        .filter(([, descriptor]) => optionVisible(descriptor, value))
        .map(([key, descriptor]) => {
          const title = labels[key] || key.replaceAll('_', ' ').replaceAll('-', ' ');
          const label = labelPrefix ? `${labelPrefix}: ${title}` : title;
          const path = prefix ? `${prefix}.${key}` : key;
          return (
            <Col
              key={key}
              xs={24}
              sm={['object', 'array', 'json', 'headers'].includes(descriptor.kind) ? 24 : 12}
            >
              <Form.Item label={title}>
                <OptionControl
                  descriptor={descriptor}
                  value={value[key]}
                  label={label}
                  path={path}
                  depth={depth}
                  onChange={(next) => onChange(key, next)}
                  onValidity={onValidity}
                />
              </Form.Item>
            </Col>
          );
        })}
    </Row>
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
  const type = String(value.type || '');
  const hidden = dedicatedFields(type);
  const schema = registry.protocols[type] || {};
  const own = Object.fromEntries(
    Object.entries(schema).filter(
      ([key]) => !hidden.has(key) && !registry.dialerFields.includes(key),
    ),
  );
  const primaryKeys = [
    'peers',
    'address',
    'private_key',
    'portBindings',
    'transports',
    'username',
    'password',
    'psk',
    'key',
    'public_key',
    'server_public_key',
    'server_disco_key',
    'pubkey',
    'domain',
    'resolvers',
    'script_keys',
    'tunnel_key',
    'strategy',
  ];
  const primary = Object.fromEntries(
    Object.entries(own).filter(
      ([key, descriptor]) => descriptor.required || primaryKeys.includes(key),
    ),
  );
  const advanced = Object.fromEntries(Object.entries(own).filter(([key]) => !(key in primary)));
  const dialer = Object.fromEntries(
    Object.entries(schema).filter(
      ([key]) => !hidden.has(key) && registry.dialerFields.includes(key),
    ),
  );
  return (
    <Col span={24}>
      <OptionGrid schema={primary} value={value} onChange={onChange} onValidity={onValidity} />
      {Object.keys(advanced).length > 0 && (
        <Collapse
          ghost
          items={[
            {
              key: 'protocol',
              label: `Дополнительные параметры ${type}`,
              children: (
                <OptionGrid
                  schema={advanced}
                  value={value}
                  onChange={onChange}
                  onValidity={onValidity}
                />
              ),
            },
          ]}
        />
      )}
      {Object.keys(dialer).length > 0 && (
        <Collapse
          ghost
          items={[
            {
              key: 'dialer',
              label: 'Сеть, DNS и тайм-ауты',
              children: (
                <OptionGrid
                  schema={dialer}
                  value={value}
                  onChange={onChange}
                  onValidity={onValidity}
                />
              ),
            },
          ]}
        />
      )}
    </Col>
  );
}
