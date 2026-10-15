import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Popconfirm,
  Space,
  Switch,
  Table,
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
    <Drawer open={open} onClose={onClose} title="Подписки для каскада" size={760} zIndex={1100}>
      <Space orientation="vertical" size={16} style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title="Добавьте подписку и выберите её в поле «Подключиться через»"
          description="Тег sub-auto-ID автоматически выбирает лучшее доступное подключение и сохраняет его, пока оно работает. Кнопка «Пинг» измеряет задержку через применённый балансировщик. Список подключений обновляется по расписанию; также можно выбрать отдельный узел подписки. UDP-цепочка требует поддержки UDP у промежуточного узла."
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
                  <Typography.Text type="secondary">
                    {row.outboundCount} подключений{row.enabled ? '' : ' · отключена'}
                  </Typography.Text>
                  {row.supportedCount !== undefined && (
                    <Typography.Text type="secondary">
                      Применимо: {row.supportedCount} из {row.outboundCount}
                    </Typography.Text>
                  )}
                  {row.selectedTag && <Typography.Text>Выбран: {row.selectedTag}</Typography.Text>}
                  {row.appliedTag && (
                    <Typography.Text type="secondary">Применён: {row.appliedTag}</Typography.Text>
                  )}
                  {row.lastWarning && (
                    <Typography.Text type="warning">{row.lastWarning}</Typography.Text>
                  )}
                  {row.applyError && (
                    <Typography.Text type="warning">Не применено: {row.applyError}</Typography.Text>
                  )}
                  {(row.lastError || row.probeError) && (
                    <Typography.Text type="danger">
                      {row.lastError || row.probeError}
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
                    Обновить
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
