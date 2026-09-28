import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Divider,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';

import { HttpUtil } from '@/utils';

type FirewallProtocol = 'tcp' | 'udp' | 'both';

interface FirewallRule {
  port: number;
  protocol: FirewallProtocol;
  label?: string;
  source?: string;
}

interface FirewallStatus {
  available: boolean;
  enabled: boolean;
  autoSync: boolean;
  backend: string;
  external: boolean;
  rules: FirewallRule[];
  manualRules: FirewallRule[];
  backendNotice?: string;
}

interface FirewallButtonProps {
  compact?: boolean;
}

const strings = {
  ru: {
    button: 'Файрволл',
    title: 'Управление файрволлом',
    enabled: 'Файрволл x-ui',
    enabledDesc:
      'Ограничивает входящие подключения и автоматически оставляет доступными только нужные панели порты.',
    auto: 'Автоматическая синхронизация',
    autoDesc:
      'При создании, изменении, включении или удалении подключения набор открытых портов обновляется автоматически.',
    backend: 'Backend',
    active: 'Включён',
    inactive: 'Выключен',
    sync: 'Синхронизировать сейчас',
    automatic: 'Автоматические правила',
    manual: 'Дополнительные порты',
    add: 'Добавить порт',
    port: 'Порт',
    protocol: 'Протокол',
    label: 'Описание',
    source: 'Источник',
    actions: 'Действия',
    remove: 'Удалить',
    confirmRemove: 'Удалить это правило?',
    noBackend:
      'Не найден поддерживаемый firewall backend. Установите nftables/iptables, UFW или firewalld.',
    safety:
      'Порт панели, порт подписок и SSH добавляются автоматически, чтобы включение файрволла не заблокировало доступ к серверу.',
    external:
      'Обнаружен системный firewall. x-ui управляет только своими правилами и не удаляет правила администратора.',
    loadError: 'Не удалось получить состояние файрволла',
    requestError: 'Операция с файрволлом не выполнена',
    invalidPort: 'Укажите порт от 1 до 65535.',
    empty: 'Нет правил',
    close: 'Закрыть',
  },
  en: {
    button: 'Firewall',
    title: 'Firewall manager',
    enabled: 'x-ui firewall',
    enabledDesc:
      'Restricts inbound traffic while keeping the ports required by the panel reachable.',
    auto: 'Automatic synchronization',
    autoDesc:
      'Creating, editing, enabling or deleting an inbound automatically updates the allowed ports.',
    backend: 'Backend',
    active: 'Enabled',
    inactive: 'Disabled',
    sync: 'Sync now',
    automatic: 'Automatic rules',
    manual: 'Additional ports',
    add: 'Add port',
    port: 'Port',
    protocol: 'Protocol',
    label: 'Description',
    source: 'Source',
    actions: 'Actions',
    remove: 'Delete',
    confirmRemove: 'Delete this rule?',
    noBackend:
      'No supported firewall backend was found. Install nftables/iptables, UFW or firewalld.',
    safety:
      'The panel, subscription and SSH ports are added automatically to avoid locking you out of the server.',
    external:
      'A system firewall is active. x-ui manages only its own rules and does not remove administrator rules.',
    loadError: 'Failed to load firewall state',
    requestError: 'Firewall operation failed',
    invalidPort: 'Enter a port between 1 and 65535.',
    empty: 'No rules',
    close: 'Close',
  },
};

function errorText(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function FirewallButton({ compact = false }: FirewallButtonProps) {
  const { i18n } = useTranslation();
  const text = i18n.resolvedLanguage?.toLowerCase().startsWith('ru') ? strings.ru : strings.en;
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<FirewallStatus | null>(null);
  const [error, setError] = useState('');
  const [port, setPort] = useState<number | null>(null);
  const [protocol, setProtocol] = useState<FirewallProtocol>('tcp');
  const [label, setLabel] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const msg = await HttpUtil.get('/panel/api/firewall/status', undefined, { silent: true });
      if (!msg?.success || !msg.obj) throw new Error(msg?.msg || text.loadError);
      setStatus(msg.obj as FirewallStatus);
    } catch (e) {
      setError(errorText(e, text.loadError));
    } finally {
      setLoading(false);
    }
  }, [text.loadError]);

  const mutate = useCallback(
    async (url: string, data?: Record<string, unknown>) => {
      setBusy(true);
      setError('');
      try {
        const msg = await HttpUtil.post(url, data, { silent: true });
        if (!msg?.success) throw new Error(msg?.msg || text.requestError);
        if (msg.obj) setStatus(msg.obj as FirewallStatus);
        else await load();
        return true;
      } catch (e) {
        setError(errorText(e, text.requestError));
        return false;
      } finally {
        setBusy(false);
      }
    },
    [load, text.requestError],
  );

  const automaticRules = useMemo(
    () => (status?.rules || []).filter((rule) => rule.source !== 'manual'),
    [status?.rules],
  );

  const columns = [
    {
      title: text.port,
      dataIndex: 'port',
      width: 90,
    },
    {
      title: text.protocol,
      dataIndex: 'protocol',
      width: 100,
      render: (value: FirewallProtocol) => <Tag>{value.toUpperCase()}</Tag>,
    },
    {
      title: text.label,
      dataIndex: 'label',
      ellipsis: true,
    },
    {
      title: text.source,
      dataIndex: 'source',
      width: 120,
      render: (value: string) => <Typography.Text type="secondary">{value || '—'}</Typography.Text>,
    },
  ];

  const manualColumns = [
    ...columns.slice(0, 3),
    {
      title: text.actions,
      key: 'actions',
      width: 100,
      render: (_: unknown, rule: FirewallRule) => (
        <Popconfirm
          title={text.confirmRemove}
          okText={text.remove}
          onConfirm={() =>
            void mutate('/panel/api/firewall/manual/delete', {
              port: rule.port,
              protocol: rule.protocol,
            })
          }
        >
          <Button danger size="small" disabled={busy}>
            {text.remove}
          </Button>
        </Popconfirm>
      ),
    },
  ];

  async function addManualRule() {
    if (!port || port < 1 || port > 65535) {
      setError(text.invalidPort);
      return;
    }
    const ok = await mutate('/panel/api/firewall/manual/add', { port, protocol, label });
    if (ok) {
      setPort(null);
      setLabel('');
    }
  }

  function openManager() {
    setOpen(true);
    void load();
  }

  return (
    <>
      <Button block={compact} onClick={openManager}>
        {text.button}
      </Button>
      <Modal
        open={open}
        title={text.title}
        width={860}
        onCancel={() => setOpen(false)}
        footer={<Button onClick={() => setOpen(false)}>{text.close}</Button>}
        destroyOnHidden
      >
        <Spin spinning={loading || busy}>
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            {error ? <Alert type="error" showIcon message={error} /> : null}
            {status && !status.available ? (
              <Alert type="warning" showIcon message={text.noBackend} />
            ) : null}
            <Alert type="info" showIcon message={text.safety} />
            {status?.external ? <Alert type="warning" showIcon message={text.external} /> : null}

            <Space wrap size="large">
              <Space>
                <Switch
                  checked={Boolean(status?.enabled)}
                  disabled={!status?.available || busy}
                  onChange={(enabled) => void mutate('/panel/api/firewall/enable', { enabled })}
                />
                <div>
                  <Typography.Text strong>{text.enabled}</Typography.Text>
                  <br />
                  <Typography.Text type="secondary">{text.enabledDesc}</Typography.Text>
                </div>
              </Space>
            </Space>

            <Space wrap size="large">
              <Space>
                <Switch
                  checked={status?.autoSync ?? true}
                  disabled={busy}
                  onChange={(autoSync) => void mutate('/panel/api/firewall/autoSync', { autoSync })}
                />
                <div>
                  <Typography.Text strong>{text.auto}</Typography.Text>
                  <br />
                  <Typography.Text type="secondary">{text.autoDesc}</Typography.Text>
                </div>
              </Space>
            </Space>

            <Space wrap>
              <Typography.Text>
                {text.backend}: <Tag>{status?.backend || '—'}</Tag>
              </Typography.Text>
              <Tag color={status?.enabled ? 'success' : 'default'}>
                {status?.enabled ? text.active : text.inactive}
              </Tag>
              <Button
                disabled={!status?.enabled || busy}
                onClick={() => void mutate('/panel/api/firewall/sync')}
              >
                {text.sync}
              </Button>
            </Space>

            <Divider orientation="left">{text.automatic}</Divider>
            <Table<FirewallRule>
              size="small"
              rowKey={(rule) => `${rule.source}-${rule.port}-${rule.protocol}`}
              columns={columns}
              dataSource={automaticRules}
              pagination={false}
              locale={{ emptyText: text.empty }}
              scroll={{ x: 520 }}
            />

            <Divider orientation="left">{text.manual}</Divider>
            <Space wrap>
              <InputNumber
                min={1}
                max={65535}
                value={port}
                placeholder={text.port}
                onChange={(value) => setPort(value == null ? null : Number(value))}
              />
              <Select<FirewallProtocol>
                value={protocol}
                style={{ width: 120 }}
                options={[
                  { value: 'tcp', label: 'TCP' },
                  { value: 'udp', label: 'UDP' },
                  { value: 'both', label: 'TCP + UDP' },
                ]}
                onChange={setProtocol}
              />
              <Input
                value={label}
                style={{ width: 240 }}
                maxLength={120}
                placeholder={text.label}
                onChange={(e) => setLabel(e.target.value)}
                onPressEnter={() => void addManualRule()}
              />
              <Button type="primary" disabled={busy} onClick={() => void addManualRule()}>
                {text.add}
              </Button>
            </Space>
            <Table<FirewallRule>
              size="small"
              rowKey={(rule) => `manual-${rule.port}-${rule.protocol}`}
              columns={manualColumns}
              dataSource={status?.manualRules || []}
              pagination={false}
              locale={{ emptyText: text.empty }}
              scroll={{ x: 520 }}
            />
          </Space>
        </Spin>
      </Modal>
    </>
  );
}
