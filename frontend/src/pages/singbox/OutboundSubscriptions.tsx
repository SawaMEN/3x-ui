import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Collapse,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Space,
  Switch,
  Table,
  Tag,
  Typography,
} from 'antd';
import OutboundPingButton from './OutboundPingButton';
import { HttpUtil } from '@/utils';

type Subscription = {
  id: number;
  remark: string;
  url: string;
  enabled: boolean;
  autoBalance: boolean;
  outboundCount: number;
  lastError?: string;
  lastWarning?: string;
  supportedCount?: number;
  selectedTag?: string;
  appliedTag?: string;
  applyError?: string;
  probeResults?: string;
  probeError?: string;
};


type WarningGroup = { label: string; issues: string[] };

function warningCategory(message: string): string {
  const value = message.toLowerCase();
  if (value.includes('grpc authority')) return 'gRPC: несовместимый authority';
  if (value.includes('tcp header "http"')) return 'TCP: HTTP-заголовок Xray';
  if (value.includes('hysteria') && value.includes('auth')) return 'Hysteria: параметры авторизации';
  if (value.includes('quic transport')) return 'QUIC: несовместимый транспорт';
  if (value.includes('xhttp')) return 'XHTTP: не поддерживается';
  if (value.includes('cascade dependency')) return 'Зависимости подключений';
  if (value.includes('не поддерживается')) return 'Протокол не поддерживается';
  return 'Другие ограничения';
}

function SubscriptionWarning({ warning }: { warning: string }) {
  const marker = /(\d+) подключений не поддерживаются текущим ядром ([^;]+); сохранены в подписке, но не применены:\s*/.exec(warning);
  if (!marker || marker.index === undefined) {
    return warning.length < 180 ? (
      <Typography.Text type="warning">{warning}</Typography.Text>
    ) : (
      <Collapse
        ghost
        size="small"
        items={[{ key: 'warning', label: 'Предупреждения подписки', children: <Typography.Text style={{ overflowWrap: 'anywhere' }}>{warning}</Typography.Text> }]}
      />
    );
  }
  const prefix = warning.slice(0, marker.index).trim().replace(/;$/, '');
  const details = warning.slice(marker.index + marker[0].length).split(/;\s*(?=[^;\n]+:\s)/).filter(Boolean);
  const grouped = new Map<string, WarningGroup>();
  for (const detail of details) {
    const label = warningCategory(detail);
    if (!grouped.has(label)) grouped.set(label, { label, issues: [] });
    grouped.get(label)?.issues.push(detail);
  }
  return (
    <Space orientation="vertical" size={4} style={{ maxWidth: '100%', width: '100%' }}>
      <Typography.Text type="warning">
        {marker[1]} подключений сохранены, но не работают с ядром {marker[2]}
      </Typography.Text>
      <Space size={[4, 4]} wrap>
        {Array.from(grouped.values()).map((group) => (
          <Tag key={group.label} color="orange">{group.label}: {group.issues.length}</Tag>
        ))}
      </Space>
      {prefix && (prefix.length > 180 ? (
        <Collapse
          ghost
          size="small"
          items={[{ key: 'parser', label: 'Другие предупреждения импорта', children: <Typography.Text style={{ overflowWrap: 'anywhere' }}>{prefix}</Typography.Text> }]}
        />
      ) : <Typography.Text type="secondary">{prefix}</Typography.Text>)}
      <Collapse
        ghost
        size="small"
        items={[{
          key: 'details',
          label: 'Показать причины по подключениям',
          children: (
            <Space orientation="vertical" size={6} style={{ width: '100%' }}>
              {details.map((detail, i) => (
                <Typography.Text key={i} type="secondary" copyable style={{ overflowWrap: 'anywhere' }}>
                  {detail}
                </Typography.Text>
              ))}
            </Space>
          ),
        }]}
      />
    </Space>
  );
}

function subscriptionStatusMessage(message: string): string {
  if (message.includes('ещё не загружен в ядро')) {
    return 'Изменения ожидают перезапуска sing-box';
  }
  if (message.includes('No healthy outbound')) {
    return 'Нет доступных узлов: автоматический маршрут временно заблокирован';
  }
  if (message.includes('core is stopped')) {
    return 'Ядро остановлено — выбор применится после запуска';
  }
  return message;
}

export default function OutboundSubscriptions({
  open,
  initialURL,
  onClose,
  onTagsChanged,
}: {
  open: boolean;
  initialURL: string;
  onClose: () => void;
  onTagsChanged: (tags: string[]) => void;
}) {
  const [rows, setRows] = useState<Subscription[]>([]);
  const [url, setURL] = useState('');
  const [remark, setRemark] = useState('');
  const [interval, setInterval] = useState(10);
  const [allowPrivate, setAllowPrivate] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    const result = await HttpUtil.get<Subscription[]>('/panel/api/xray/outbound-subs', undefined, {
      silent: true,
    });
    if (!result.success) throw new Error(result.msg || 'Не удалось загрузить подписки');
    setRows(result.obj || []);
  }, []);
  useEffect(() => {
    if (!open) return;
    const timer = window.setTimeout(() => {
      setURL(initialURL);
      setError('');
      void load().catch((e: Error) => setError(e.message));
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open, initialURL, load]);
  async function syncTags() {
    const result = await HttpUtil.post<{ subscriptionOutboundTags?: string[] }>(
      '/panel/api/xray/',
      undefined,
      { silent: true },
    );
    if (!result.success) throw new Error(result.msg || 'Не удалось обновить подключения');
    onTagsChanged(result.obj?.subscriptionOutboundTags || []);
  }
  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      try {
        await load();
        await syncTags();
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
      setBusy(false);
    }
  }
  async function refresh(id: number) {
    const result = await HttpUtil.post(`/panel/api/xray/outbound-subs/${id}/refresh`, undefined, {
      silent: true,
    });
    if (!result.success) throw new Error(result.msg || 'Не удалось обновить подписку');
  }
  async function add() {
    const result = await HttpUtil.post<Subscription>(
      '/panel/api/xray/outbound-subs',
      {
        url: url.trim(),
        remark: remark.trim(),
        enabled: true,
        prepend: false,
        updateInterval: interval * 60,
        allowPrivate,
        autoBalance: true,
        balanceMode: 'latency',
        probeInterval: 300,
        switchInterval: 900,
        healthInterval: 30,
        tolerance: 50,
      },
      { silent: true },
    );
    if (!result.success || !result.obj)
      throw new Error(result.msg || 'Не удалось добавить подписку');
    // A failed first fetch remains a visible saved row for retry, without
    // duplicate subscriptions created by a second click on Add.
    setURL('');
    setRemark('');
    await refresh(result.obj.id);
  }
  return (
    <Drawer
      destroyOnHidden
      open={open}
      onClose={onClose}
      title="Подписки для каскада"
      size={760}
      zIndex={1100}
    >
      <Space orientation="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title="Добавьте подписку и выберите её в поле «Подключиться через»"
          description="Рабочие подключения выбираются автоматически. Несовместимые профили сохраняются для других версий ядра; подробности скрыты до раскрытия. Обновление также применяет изменения к запущенному sing-box."
        />
        {error && <Alert type="error" showIcon title={error} />}
        <Form layout="vertical">
          <Form.Item label="URL подписки" htmlFor="cascade-subscription-url" required>
            <Input
              id="cascade-subscription-url"
              value={url}
              onChange={(e) => setURL(e.target.value)}
              placeholder="https://example.com/sub/..."
            />
          </Form.Item>
          <Form.Item label="Название">
            <Input value={remark} onChange={(e) => setRemark(e.target.value)} />
          </Form.Item>
          <Form.Item label="Обновление, минуты">
            <InputNumber
              min={1}
              max={1440}
              value={interval}
              onChange={(v) => setInterval(v ?? 10)}
            />
          </Form.Item>
          <Form.Item label="Разрешить адрес подписки в локальной сети">
            <Switch checked={allowPrivate} onChange={setAllowPrivate} />
          </Form.Item>
          <Button
            type="primary"
            loading={busy}
            disabled={!url.trim()}
            onClick={() => void run(add)}
          >
            Добавить и загрузить
          </Button>
        </Form>
        <Table<Subscription>
          rowKey="id"
          dataSource={rows}
          pagination={false}
          size="small"
          columns={[
            {
              title: 'Подписка',
              render: (_, row) => (
                <Space orientation="vertical" size={0}>
                  <Typography.Text strong>{row.remark || `Подписка ${row.id}`}</Typography.Text>
                  <Typography.Text code>
                    {row.autoBalance ? `sub-auto-${row.id}` : `${row.outboundCount} узлов`}
                  </Typography.Text>
                  <Space size={4} wrap>
                    {row.supportedCount !== undefined ? (
                      <>
                        <Tag color="green">Применимо: {row.supportedCount} из {row.outboundCount}</Tag>
                        {row.outboundCount > row.supportedCount && (
                          <Tag color="orange">Недоступно: {row.outboundCount - row.supportedCount}</Tag>
                        )}
                      </>
                    ) : (
                      <Tag>{row.outboundCount} подключений</Tag>
                    )}
                    {!row.enabled && <Tag>Отключена</Tag>}
                  </Space>
                  {row.selectedTag && <Typography.Text type="secondary">Выбран: {row.selectedTag}</Typography.Text>}
                  {row.appliedTag && row.appliedTag !== row.selectedTag && (
                    <Typography.Text type="secondary">Применён: {row.appliedTag}</Typography.Text>
                  )}
                  {row.lastWarning && <SubscriptionWarning warning={row.lastWarning} />}
                  {row.applyError && (
                    <Typography.Text type="warning" title={row.applyError}>
                      {subscriptionStatusMessage(row.applyError)}
                    </Typography.Text>
                  )}
                  {row.probeError && (
                    <Typography.Text type="warning" title={row.probeError}>
                      {subscriptionStatusMessage(row.probeError)}
                    </Typography.Text>
                  )}
                  {row.lastError && (
                    <Typography.Text type="danger" style={{ overflowWrap: 'anywhere' }}>
                      {row.lastError}
                    </Typography.Text>
                  )}
                </Space>
              ),
            },
            {
              title: 'Действия',
              render: (_, row) => (
                <Space wrap>
                  <Button disabled={busy} onClick={() => void run(() => refresh(row.id))}>
                    Обновить и применить
                  </Button>
                  <Button
                    disabled={busy || !row.enabled || !row.autoBalance || !row.outboundCount}
                    onClick={() =>
                      void run(async () => {
                        const reply = await HttpUtil.post(
                          `/panel/api/xray/outbound-subs/${row.id}/probe`,
                          undefined,
                          { silent: true },
                        );
                        if (!reply.success)
                          throw new Error(reply.msg || 'Не удалось проверить подключения подписки');
                      })
                    }
                  >
                    Проверить и выбрать лучший
                  </Button>
                  <OutboundPingButton
                    key={`${row.id}-${row.appliedTag || ''}`}
                    tag={`sub-auto-${row.id}`}
                    disabled={
                      busy ||
                      !row.enabled ||
                      !row.autoBalance ||
                      !row.outboundCount ||
                      !row.appliedTag
                    }
                  />
                  <Popconfirm
                    title="Удалить подписку? Ссылки на её теги в каскаде нужно заменить."
                    onConfirm={() =>
                      run(async () => {
                        const result = await HttpUtil.post(
                          `/panel/api/xray/outbound-subs/${row.id}/del`,
                          undefined,
                          { silent: true },
                        );
                        if (!result.success)
                          throw new Error(result.msg || 'Не удалось удалить подписку');
                      })
                    }
                  >
                    <Button danger disabled={busy}>
                      Удалить
                    </Button>
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Space>
    </Drawer>
  );
}
