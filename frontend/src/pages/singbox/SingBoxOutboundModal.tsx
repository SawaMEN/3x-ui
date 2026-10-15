import {
  useEffect,
  useRef,
  useState,
  useId,
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  Form,
  Input,
  InputNumber,
  Modal,
  Row,
  Select,
  Space,
  Switch,
  Tabs,
  Tag,
} from 'antd';
import {
  ApiOutlined,
  CodeOutlined,
  ImportOutlined,
  SafetyCertificateOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import JsonEditor from '@/components/form/JsonEditor';
import { outboundDefaults, outboundErrors, type ConfigObject } from './simple-settings';
import { uniqueTag } from './share-links';
import {
  importNativeConnections,
  isNativeEndpoint,
  nativeConnectionLink,
  nativeEndpointTypes,
  nativeServerTypes,
  renameImportedConnections,
  rewriteNativeReferences,
} from './native-import';
import {
  changeProtocol,
  changeSecurity,
  changeServer,
  changeTransport,
  object,
  outboundProtocols,
  outboundProtocolLabel,
  parseOutboundJson,
  requiredTlsProtocols,
  realityProtocols,
  strings,
  text,
  tlsProtocols,
  transportProtocols,
} from './outbound-editor-model';
import { generateOutboundCredentials, outboundCredentialSummary } from './outbound-credentials';
import { ClipboardManager } from '@/utils';
import { autoConfigureOutbound } from './outbound-automation';
import ProtocolOptionFields from './ProtocolOptionFields';
import nativeFields from './native-outbound-fields.json';
import { patchOption, type OptionSchema } from './protocol-option-model';
import MasqueConnectionFields from './MasqueConnectionFields';
import './SingBoxOutboundModal.css';
function Field({
  label,
  children,
  wide = false,
  hint,
}: {
  label: string;
  children: ReactNode;
  wide?: boolean;
  hint?: string;
}) {
  const id = useId();
  return (
    <Col xs={24} sm={wide ? 24 : 12}>
      <Form.Item label={label} extra={hint} htmlFor={id}>
        {isValidElement(children)
          ? cloneElement(children as ReactElement<{ id?: string }>, { id })
          : children}
      </Form.Item>
    </Col>
  );
}
const options = (values: string[]) => values.map((value) => ({ value, label: value }));
export default function SingBoxOutboundModal({
  open,
  value,
  existingTags,
  onCancel,
  onSubscriptionURL,
  onSave,
}: {
  open: boolean;
  value: ConfigObject | null;
  existingTags: string[];
  onCancel: () => void;
  onSubscriptionURL?: (url: string) => void;
  onSave: (value: ConfigObject, additional?: ConfigObject[]) => void;
}) {
  const [optionsRevision, setOptionsRevision] = useState(0);
  const [draft, setDraft] = useState<ConfigObject>({});
  const [additional, setAdditional] = useState<ConfigObject[]>([]);
  const [exportedLink, setExportedLink] = useState('');
  const [tab, setTab] = useState('connection'),
    [json, setJson] = useState(''),
    [jsonError, setJsonError] = useState('');
  const [invalidOptions, setInvalidOptions] = useState<string[]>([]);
  const [error, setError] = useState(''),
    [link, setLink] = useState('');
  const [importing, setImporting] = useState(false),
    [imported, setImported] = useState(false);
  const session = useRef(0),
    tagsRef = useRef(existingTags);
  useEffect(() => {
    tagsRef.current = existingTags;
  }, [existingTags]);
  const savedProtocols = useRef<Record<string, ConfigObject>>({});
  const autoTagRef = useRef('');
  useEffect(() => {
    session.current++;
    if (!open) return;
    const initial = value
      ? structuredClone(value)
      : outboundDefaults('vless', uniqueTag('proxy', tagsRef.current));
    autoTagRef.current = value ? '' : text(initial.tag);
    setDraft(initial);
    setJson(JSON.stringify(initial, null, 2));
    setTab('connection');
    setError('');
    setJsonError('');
    setLink('');
    setImported(false);
    setImporting(false);
    setAdditional([]);
    setExportedLink('');
    setInvalidOptions([]);
    setOptionsRevision((revision) => revision + 1);
    savedProtocols.current = {};
  }, [open, value]);
  const type = text(draft.type),
    tls = object(draft.tls),
    reality = object(tls.reality),
    transport = object(draft.transport);
  const group = ['selector', 'urltest', 'balancer'].includes(type),
    remote = !group && !['direct', 'block'].includes(type);
  const availableTags = existingTags.filter((t) => t !== text(value?.tag));
  const update = (next: ConfigObject) => {
    setDraft(next);
    setJson(JSON.stringify(next, null, 2));
    setJsonError('');
    setError('');
  };
  const patch = (key: string, v: unknown) => {
    if (key === 'tag' && typeof v === 'string') {
      autoTagRef.current = '';
      setAdditional((items) =>
        items.map(
          (item) => rewriteNativeReferences(item, new Map([[text(draft.tag), v]])) as ConfigObject,
        ),
      );
    }
    const schema =
      (nativeFields.protocols as unknown as Record<string, OptionSchema>)[text(draft.type)] || {};
    const next = patchOption(draft, key, v, schema);
    if (draft.type === 'mieru' && key === 'portBindings') delete next.server_port;
    update(next);
  };
  const nested = (key: string, field: string, v: unknown) => {
    const child = { ...object(draft[key]) };
    if (v === '' || v === undefined) delete child[field];
    else child[field] = v;
    update({ ...draft, [key]: child });
  };
  const input = (key: string, password = false, placeholder?: string) =>
    password ? (
      <Input.Password
        value={text(draft[key])}
        onChange={(e) => patch(key, e.target.value)}
        placeholder={placeholder}
      />
    ) : (
      <Input
        value={text(draft[key])}
        onChange={(e) => patch(key, e.target.value)}
        placeholder={placeholder}
      />
    );
  const number = (key: string, min = 0, max?: number) => (
    <InputNumber
      style={{ width: '100%' }}
      value={typeof draft[key] === 'number' ? (draft[key] as number) : undefined}
      min={min}
      precision={0}
      max={max}
      onChange={(v) => patch(key, v)}
    />
  );
  const security = tls.enabled ? (reality.enabled ? 'reality' : 'tls') : 'none',
    network = text(transport.type) || 'tcp';
  async function importLink() {
    if (/^https?:\/\//i.test(link.trim()) && onSubscriptionURL) {
      try {
        const url = new URL(link.trim());
        if (!url.username && !url.password) {
          onSubscriptionURL(url.href);
          return;
        }
      } catch {
        /* The normal importer reports malformed URLs. */
      }
    }
    const current = session.current;
    setImporting(true);
    setError('');
    try {
      const items = await importNativeConnections(link);
      if (current !== session.current) return;
      const renamed = renameImportedConnections(items, availableTags);
      autoTagRef.current = '';
      update(renamed[0]);
      setInvalidOptions([]);
      setOptionsRevision((revision) => revision + 1);
      setAdditional(renamed.slice(1));
      setExportedLink('');
      savedProtocols.current = {};
      setImported(true);
      setLink('');
    } catch (e) {
      if (current === session.current) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (current === session.current) setImporting(false);
    }
  }
  const connection = (
    <Form layout="vertical">
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 16 }}
        title={
          type === 'masque'
            ? 'Cloudflare WARP через MASQUE'
            : 'Автоматическая настройка подключения'
        }
        description={
          type === 'masque'
            ? 'Устройство WARP регистрируется ядром sing-box при первом запуске. Выберите HTTP/3 или HTTP/2, сохраните подключение и примените настройки ядра. Кэш регистрации включается автоматически — свой MASQUE-сервер и отдельные ключи не нужны.'
            : 'Импортируйте ссылку — адрес, ключи и настройки заполнятся сами. Необязательные параметры выбирает ядро. Недостающие учётные данные создаются автоматически; существующие сохраняются. Новые данные нужно указать и на удалённом сервере.'
        }
        action={
          type === 'masque' ? undefined : (
            <Button
              disabled={!!jsonError || invalidOptions.length > 0}
              onClick={() =>
                update(
                  autoConfigureOutbound({
                    ...draft,
                    tag: text(draft.tag) || uniqueTag(type, availableTags),
                  }),
                )
              }
            >
              Настроить автоматически
            </Button>
          )
        }
      />
      {type !== 'masque' && (
        <Space wrap style={{ marginBottom: 16 }}>
          <Button
            disabled={!!jsonError || invalidOptions.length > 0}
            onClick={() => update(generateOutboundCredentials(draft, true))}
          >
            Создать недостающие учётные данные
          </Button>
          <Button
            onClick={() =>
              Modal.info({
                title: 'Учётные данные подключения',
                width: 650,
                content: (
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    <p>
                      Новые логины, пароли и общие ключи должны совпадать на клиенте и сервере.
                      Публичный ключ WireGuard/AWG добавьте на сервер как ключ клиента.
                    </p>
                    <Input.TextArea
                      aria-label="Учётные данные подключения"
                      readOnly
                      autoSize={{ minRows: 6, maxRows: 18 }}
                      value={JSON.stringify(outboundCredentialSummary(draft), null, 2)}
                    />
                    <Button
                      onClick={() =>
                        void ClipboardManager.copyText(
                          JSON.stringify(outboundCredentialSummary(draft), null, 2),
                        )
                      }
                    >
                      Скопировать данные
                    </Button>
                  </Space>
                ),
              })
            }
          >
            Показать данные для сервера
          </Button>
        </Space>
      )}
      <Space.Compact style={{ width: '100%', marginBottom: 16 }}>
        <Input
          aria-label="Ссылка для автоматической настройки"
          placeholder="Вставьте ссылку подключения или подписки"
          value={link}
          onChange={(e) => setLink(e.target.value)}
          onPressEnter={() => {
            if (link.trim() && !importing) void importLink();
          }}
        />
        <Button loading={importing} disabled={!link.trim()} onClick={importLink}>
          Заполнить из ссылки
        </Button>
      </Space.Compact>
      <Row gutter={24}>
        <Field label="Протокол">
          <Select
            showSearch
            optionFilterProp="label"
            value={type || undefined}
            options={[...outboundProtocols, ...nativeEndpointTypes].map((protocol) => ({
              value: protocol,
              label: outboundProtocolLabel(protocol),
            }))}
            onChange={(next) => {
              setInvalidOptions([]);
              savedProtocols.current[type] = structuredClone(draft);
              const saved = savedProtocols.current[next];
              const changed = saved ? { ...saved, tag: draft.tag } : changeProtocol(draft, next);
              // Only rename the pristine generated tag of a NEW connection.
              // Editing and manually named/imported profiles keep their names.
              if (!value && autoTagRef.current && text(draft.tag) === autoTagRef.current) {
                const nextTag = uniqueTag(
                  next === 'masque' ? 'warp-masque' : 'proxy',
                  availableTags,
                );
                changed.tag = nextTag;
                autoTagRef.current = nextTag;
              }
              update(changed);
            }}
          />
        </Field>
        <Field label="Название подключения" hint="Используется в маршрутах и группах.">
          {input('tag', false, 'proxy')}
        </Field>
        {remote && (
          <>
            {nativeServerTypes.includes(type) && (
              <Field label="Адрес сервера">
                <Input
                  value={text(draft.server)}
                  onChange={(e) => update(changeServer(draft, e.target.value.trim()))}
                  placeholder="example.com или IP-адрес"
                />
              </Field>
            )}
            {nativeServerTypes.includes(type) &&
              !['mieru', 'openconnect', 'pingtunnel'].includes(type) && (
                <Field label="Порт">
                  <InputNumber
                    style={{ width: '100%' }}
                    value={typeof draft.server_port === 'number' ? draft.server_port : undefined}
                    min={1}
                    max={65535}
                    onChange={(port) => {
                      const next = { ...draft };
                      if (port === null) delete next.server_port;
                      else next.server_port = port;
                      if (type === 'hysteria2') delete next.server_ports;
                      update(next);
                    }}
                  />
                </Field>
              )}
            {['vless', 'vmess', 'tuic'].includes(type) && (
              <Field label="UUID" wide>
                {input('uuid', false, 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx')}
              </Field>
            )}
            {['http', 'socks', 'mieru', 'naive'].includes(type) && (
              <Field label="Логин">{input('username')}</Field>
            )}
            {type === 'naive' && (
              <Field
                label="Одновременные соединения"
                hint="Пусто или 0 — выбор ядра. Увеличение может ослабить маскировку Naive."
              >
                {number('insecure_concurrency', 0)}
              </Field>
            )}
            {type === 'ssh' && (
              <Field label="Пользователь SSH">{input('user', false, 'root')}</Field>
            )}
            {[
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
            ].includes(type) && <Field label="Пароль">{input('password', true)}</Field>}
            {type === 'hysteria' && (
              <>
                <Field label="Авторизация">{input('auth_str', true)}</Field>
                <Field label="Отправка, Мбит/с">{number('up_mbps', 1)}</Field>
                <Field label="Загрузка, Мбит/с">{number('down_mbps', 1)}</Field>
              </>
            )}
            {type === 'snell' && (
              <>
                <Field label="PSK">{input('psk', true)}</Field>
                <Field label="Версия">
                  <Select
                    aria-label="Версия Snell"
                    value={draft.version || 4}
                    options={[4, 6].map((v) => ({ value: v, label: String(v) }))}
                    onChange={(v) => patch('version', v)}
                  />
                </Field>
              </>
            )}
            {type === 'shadowtls' && (
              <Field label="Версия ShadowTLS">
                <Select
                  value={draft.version || 3}
                  options={[1, 2, 3].map((v) => ({ value: v, label: String(v) }))}
                  onChange={(v) => patch('version', v)}
                />
              </Field>
            )}
            {type === 'shadowsocks' && (
              <Field label="Шифрование">
                <Select
                  showSearch
                  value={text(draft.method)}
                  options={options([
                    '2022-blake3-aes-128-gcm',
                    '2022-blake3-aes-256-gcm',
                    '2022-blake3-chacha20-poly1305',
                    'aes-128-gcm',
                    'aes-256-gcm',
                    'chacha20-ietf-poly1305',
                    'none',
                  ])}
                  onChange={(v) => patch('method', v)}
                />
              </Field>
            )}
            {type === 'vmess' && (
              <Field label="Шифрование VMess">
                <Select
                  value={text(draft.security) || 'auto'}
                  options={options(['auto', 'aes-128-gcm', 'chacha20-poly1305', 'none', 'zero'])}
                  onChange={(v) => patch('security', v)}
                />
              </Field>
            )}
            {type === 'vless' && (
              <Field
                label="Шифрование VLESS"
                wide
                hint="Полная строка mlkem768x25519plus из ссылки сервера. Все режимы ядра сохраняются без изменения."
              >
                {input('encryption', false, 'none')}
              </Field>
            )}
            {type === 'vless' && (
              <Field label="Flow" hint="Vision автоматически включает TLS и использует TCP.">
                <Select
                  allowClear
                  value={text(draft.flow) || undefined}
                  options={options(['xtls-rprx-vision'])}
                  onChange={(v) => {
                    if (!v) {
                      patch('flow', undefined);
                      return;
                    }
                    const next = changeSecurity(draft, security === 'reality' ? 'reality' : 'tls');
                    delete next.transport;
                    update({ ...next, flow: v });
                  }}
                />
              </Field>
            )}
            {type === 'hysteria2' && (
              <Field label="Смена портов" hint="Диапазоны через запятую: 443:443,10000:10010">
                <Input
                  value={strings(draft.server_ports).join(',')}
                  onChange={(e) => {
                    const next = { ...draft };
                    if (e.target.value.trim()) {
                      next.server_ports = e.target.value.split(',').map((p) => p.trim());
                      delete next.server_port;
                    } else {
                      delete next.server_ports;
                      next.server_port = 443;
                    }
                    update(next);
                  }}
                />
              </Field>
            )}
          </>
        )}
        {group && (
          <>
            <Field label="Подключения группы" wide>
              <Select
                mode="multiple"
                value={strings(draft.outbounds)}
                options={options(existingTags.filter((t) => t !== draft.tag))}
                onChange={(v) => {
                  const next: ConfigObject = { ...draft, outbounds: v };
                  if (next.default && !v.includes(text(next.default))) delete next.default;
                  update(next);
                }}
              />
            </Field>
            {type === 'selector' && (
              <Field label="Подключение по умолчанию">
                <Select
                  allowClear
                  value={text(draft.default) || undefined}
                  options={options(strings(draft.outbounds))}
                  onChange={(v) => patch('default', v)}
                />
              </Field>
            )}
            {type === 'urltest' && (
              <>
                <Field label="URL проверки">{input('url')}</Field>
                <Field label="Интервал">{input('interval', false, '3m')}</Field>
                <Field label="Допуск, мс">{number('tolerance')}</Field>
              </>
            )}
          </>
        )}
        {!group && !['block', 'bridge'].includes(type) && (
          <Field
            label="Подключиться через"
            wide
            hint="Без выбора подключение идёт напрямую к серверу."
          >
            <Select
              allowClear
              showSearch
              placeholder="Напрямую"
              value={text(draft.detour) || undefined}
              options={options(
                [
                  ...new Set([
                    ...existingTags,
                    ...additional.map((item) => text(item.tag)),
                    text(draft.detour),
                  ]),
                ].filter((t) => t && t !== draft.tag),
              )}
              onChange={(v) => patch('detour', v)}
            />
          </Field>
        )}
        {['masque', 'masque-client'].includes(type) && (
          <Col span={24}>
            <MasqueConnectionFields value={draft} onChange={update} />
          </Col>
        )}
        <ProtocolOptionFields
          key={`${type}-${optionsRevision}`}
          value={draft}
          onChange={patch}
          onValidity={(key, valid) =>
            setInvalidOptions((current) =>
              valid ? current.filter((item) => item !== key) : [...new Set([...current, key])],
            )
          }
        />
      </Row>
    </Form>
  );
  const securityForm = (
    <Form layout="vertical">
      <Row gutter={24}>
        <Field label="Защита подключения">
          <Select
            value={security}
            options={[
              ...(!requiredTlsProtocols.includes(type)
                ? [{ value: 'none', label: 'Без TLS' }]
                : []),
              { value: 'tls', label: 'TLS' },
              ...(realityProtocols.includes(type) ? [{ value: 'reality', label: 'Reality' }] : []),
            ]}
            onChange={(v) => update(changeSecurity(draft, v))}
          />
        </Field>
        {tls.enabled === true && (
          <>
            <Field label="Имя сервера (SNI)" hint="По умолчанию совпадает с адресом сервера.">
              <Input
                value={text(tls.server_name)}
                placeholder={text(draft.server)}
                onChange={(e) => nested('tls', 'server_name', e.target.value)}
              />
            </Field>
            <Field label="ALPN">
              <Select
                mode="tags"
                value={strings(tls.alpn)}
                options={options(['h2', 'http/1.1', 'h3'])}
                onChange={(v) => nested('tls', 'alpn', v)}
              />
            </Field>
            {!['hysteria', 'hysteria2', 'tuic', 'naive'].includes(type) && (
              <Field label="Отпечаток TLS (uTLS)">
                <Select
                  allowClear
                  placeholder="Стандартный TLS"
                  value={object(tls.utls).enabled ? text(object(tls.utls).fingerprint) : undefined}
                  options={options([
                    'chrome',
                    'firefox',
                    'safari',
                    'ios',
                    'android',
                    'edge',
                    'random',
                    'randomized',
                  ])}
                  onChange={(v) => {
                    const next = { ...tls };
                    if (v) next.utls = { enabled: true, fingerprint: v };
                    else if (security === 'reality')
                      next.utls = { enabled: true, fingerprint: 'chrome' };
                    else delete next.utls;
                    update({ ...draft, tls: next });
                  }}
                />
              </Field>
            )}
            {security === 'reality' ? (
              <>
                <Field label="Публичный ключ Reality" wide>
                  <Input
                    value={text(reality.public_key)}
                    onChange={(e) =>
                      nested('tls', 'reality', { ...reality, public_key: e.target.value })
                    }
                  />
                </Field>
                <Field label="Short ID">
                  <Input
                    value={text(reality.short_id)}
                    onChange={(e) =>
                      nested('tls', 'reality', { ...reality, short_id: e.target.value })
                    }
                  />
                </Field>
              </>
            ) : (
              <Field
                label="Пропустить проверку сертификата"
                hint="Для сервера с собственным сертификатом."
              >
                <Switch checked={!!tls.insecure} onChange={(v) => nested('tls', 'insecure', v)} />
              </Field>
            )}
            <Field label="Файл сертификата" wide>
              <Input
                value={text(tls.certificate_path)}
                onChange={(e) => nested('tls', 'certificate_path', e.target.value)}
                placeholder="Необязательно"
              />
            </Field>
          </>
        )}
      </Row>
    </Form>
  );
  const transportForm = (
    <Form layout="vertical">
      <Row gutter={24}>
        <Field label="Транспорт">
          <Select
            value={network}
            options={options([
              'tcp',
              'raw',
              'ws',
              'grpc',
              'http',
              'httpupgrade',
              'xhttp',
              'quic',
              'kcp',
            ])}
            onChange={(v) => update(changeTransport(draft, v))}
          />
        </Field>
        {['ws', 'http', 'httpupgrade', 'xhttp'].includes(network) && (
          <>
            <Field label="Путь">
              <Input
                value={text(transport.path)}
                placeholder="/"
                onChange={(e) => nested('transport', 'path', e.target.value)}
              />
            </Field>
            <Field label="Host">
              <Input
                value={
                  network === 'ws'
                    ? text(object(transport.headers).Host)
                    : network === 'http'
                      ? strings(transport.host).join(',')
                      : text(transport.host)
                }
                onChange={(e) => {
                  if (network === 'ws')
                    nested('transport', 'headers', {
                      ...object(transport.headers),
                      Host: e.target.value,
                    });
                  else
                    nested(
                      'transport',
                      'host',
                      network === 'http'
                        ? e.target.value
                            .split(',')
                            .map((s) => s.trim())
                            .filter(Boolean)
                        : e.target.value,
                    );
                }}
              />
            </Field>
          </>
        )}
        {network === 'grpc' && (
          <Field label="Имя сервиса gRPC">
            <Input
              value={text(transport.service_name)}
              onChange={(e) => nested('transport', 'service_name', e.target.value)}
            />
          </Field>
        )}
        {network === 'kcp' && (
          <>
            <Field label="Seed mKCP">
              <Input
                value={text(transport.seed)}
                onChange={(e) => nested('transport', 'seed', e.target.value)}
              />
            </Field>
            <Field label="Заголовок mKCP">
              <Select
                value={text(transport.header) || 'none'}
                options={options(['none', 'srtp', 'utp', 'wechat', 'dtls', 'wireguard'])}
                onChange={(v) => nested('transport', 'header', v)}
              />
            </Field>
            <Field label="Совместимость с Xray mKCP">
              <Switch
                checked={!!transport.legacy}
                onChange={(v) => nested('transport', 'legacy', v)}
              />
            </Field>
          </>
        )}
        {network === 'xhttp' && (
          <Field label="Режим XHTTP">
            <Select
              value={text(transport.mode) || 'auto'}
              options={options(['auto', 'packet-up', 'stream-up', 'stream-one'])}
              onChange={(v) => nested('transport', 'mode', v)}
            />
          </Field>
        )}
        <Col span={24}>
          <Alert
            type="info"
            showIcon
            title={
              network === 'tcp' ? 'Прямое TCP-подключение' : 'Дополнительные параметры сохраняются'
            }
            description={
              network === 'tcp'
                ? 'TLS и Reality настраиваются в разделе безопасности.'
                : 'Параметры импортированной ссылки доступны в JSON и сохраняются при редактировании формы.'
            }
          />
        </Col>
      </Row>
    </Form>
  );
  const jsonForm = (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Card
        size="small"
        className="outbound-import-card"
        title={
          <Space>
            <ImportOutlined />
            Импорт ссылки
          </Space>
        }
      >
        <p className="outbound-muted">
          Вставьте ссылку, нативный JSON или цепочку подключений. Параметры всех протоколов, TLS,
          транспорта и маскировок импортирует установленное ядро.
        </p>
        <Space.Compact style={{ width: '100%' }}>
          <Input.TextArea
            autoSize={{ minRows: 2, maxRows: 6 }}
            aria-label="Ссылка подключения"
            value={link}
            placeholder="vless://… · vmess://… · ss://… · trojan://…"
            onChange={(e) => {
              setLink(e.target.value);
              setImported(false);
            }}
            onPressEnter={(event) => {
              if ((event.ctrlKey || event.metaKey) && link.trim() && !importing) void importLink();
            }}
            disabled={importing}
          />
          <Button
            type="primary"
            icon={<ImportOutlined />}
            loading={importing}
            disabled={!link.trim()}
            onClick={importLink}
          >
            Импортировать
          </Button>
        </Space.Compact>
        {imported && (
          <Alert
            style={{ marginTop: 12 }}
            type="success"
            showIcon
            title="Импорт выполнен. Форма и JSON обновлены."
          />
        )}
      </Card>
      {additional.length > 0 && (
        <Alert
          type="info"
          showIcon
          title={`Импортировано подключений: ${additional.length + 1}`}
          description="Первое подключение открыто в редакторе. Остальные будут добавлены вместе с ним; связи между ними сохранятся."
        />
      )}
      <div className="outbound-json-toolbar">
        <span className="outbound-muted">JSON и поля формы синхронизируются автоматически</span>
        <Button
          size="small"
          disabled={!!jsonError}
          onClick={() => setExportedLink(nativeConnectionLink(draft))}
        >
          Получить ссылку
        </Button>
        <Button
          size="small"
          disabled={!!jsonError}
          onClick={() => setJson(JSON.stringify(draft, null, 2))}
        >
          Форматировать
        </Button>
      </div>
      {exportedLink && (
        <Input.TextArea
          aria-label="Нативная ссылка"
          readOnly
          autoSize={{ minRows: 2, maxRows: 4 }}
          value={nativeConnectionLink(draft)}
        />
      )}
      <JsonEditor
        value={json}
        minHeight="320px"
        maxHeight="440px"
        readOnly={importing}
        onChange={(raw) => {
          setJson(raw);
          try {
            setDraft(parseOutboundJson(raw));
            autoTagRef.current = '';
            setOptionsRevision((revision) => revision + 1);
            setInvalidOptions([]);
            setJsonError('');
            setError('');
          } catch (e) {
            setJsonError(e instanceof Error ? e.message : String(e));
          }
        }}
      />
      {jsonError && <Alert type="error" showIcon title="Исправьте JSON" description={jsonError} />}
    </Space>
  );
  const tabs = [
    {
      key: 'connection',
      label: (
        <Space>
          <ApiOutlined />
          Подключение
        </Space>
      ),
      children: connection,
    },
    ...(remote && tlsProtocols.includes(type)
      ? [
          {
            key: 'security',
            label: (
              <Space>
                <SafetyCertificateOutlined />
                Безопасность
              </Space>
            ),
            children: securityForm,
          },
        ]
      : []),
    ...(transportProtocols.includes(type)
      ? [
          {
            key: 'transport',
            label: (
              <Space>
                <SwapOutlined />
                Транспорт
              </Space>
            ),
            children: transportForm,
          },
        ]
      : []),
    {
      key: 'json',
      label: (
        <Space>
          <CodeOutlined />
          JSON / импорт
        </Space>
      ),
      children: jsonForm,
    },
  ];
  return (
    <Modal
      open={open}
      width={880}
      className="outbound-editor-modal"
      title={
        <div className="outbound-modal-title">
          <div className="outbound-title-icon">
            <ApiOutlined />
          </div>
          <div>
            <div>{value ? 'Изменить подключение' : 'Новый outbound'}</div>
            <div className="outbound-muted">Настройте вручную или импортируйте готовую ссылку</div>
          </div>
        </div>
      }
      destroyOnHidden
      onCancel={() => {
        session.current++;
        onCancel();
      }}
      mask={{ closable: !importing }}
      cancelButtonProps={{ disabled: importing }}
      okButtonProps={{ disabled: importing || !!jsonError || invalidOptions.length > 0 }}
      okText={value ? 'Сохранить изменения' : 'Добавить подключение'}
      cancelText="Отмена"
      onOk={() => {
        if (invalidOptions.length > 0 || jsonError || importing) return;
        const batchTags = additional.map((item) => text(item.tag));
        const errors = outboundErrors(draft, [...availableTags, ...batchTags]);
        for (const item of additional)
          errors.push(
            ...outboundErrors(item, [
              ...availableTags,
              text(draft.tag),
              ...batchTags.filter((tag) => tag !== item.tag),
            ]),
          );
        if (
          object(object(draft.tls).reality).enabled &&
          !text(object(object(draft.tls).reality).public_key)
        )
          errors.push('Укажите публичный ключ Reality.');
        if (errors.length) {
          setError(errors.join(' '));
          return;
        }
        onSave({ ...draft, tag: text(draft.tag).trim() }, additional);
      }}
    >
      <div className="outbound-summary">
        <Space wrap>
          <Tag color="blue">{type ? outboundProtocolLabel(type) : 'Протокол'}</Tag>
          <span>
            {text(draft.server) ||
              (type === 'masque'
                ? 'Cloudflare WARP · регистрация в sing-box'
                : group
                  ? 'Группа подключений'
                  : remote
                    ? 'Укажите сервер'
                    : 'Локальное подключение')}
            {draft.server && draft.server_port ? `:${draft.server_port}` : ''}
          </span>
        </Space>
        <Space>
          <span>
            {type === 'masque'
              ? draft.use_http2 ? 'HTTP/2 · TCP/TLS' : 'HTTP/3 · QUIC/TLS'
              : security === 'reality' ? 'Reality' : security === 'tls' ? 'TLS' : 'Без TLS'}
          </span>
          {transportProtocols.includes(type) && <Tag>{network.toUpperCase()}</Tag>}
          {['hysteria', 'hysteria2', 'tuic'].includes(type) && <Tag>QUIC</Tag>}
        </Space>
      </div>
      {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 16 }} />}
      <Tabs
        activeKey={tabs.some((t) => t.key === tab) ? tab : 'connection'}
        onChange={(next) => {
          if (!jsonError && !importing && invalidOptions.length === 0) setTab(next);
        }}
        items={tabs}
      />
      {isNativeEndpoint(draft) && (
        <Alert
          type="info"
          showIcon
          title="Туннель будет сохранён в Endpoints"
          description="Параметры туннеля доступны в дополнительных настройках и JSON. Он будет доступен как исходящее подключение в маршрутах."
        />
      )}
    </Modal>
  );
}
