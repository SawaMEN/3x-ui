import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  EditOutlined,
  KeyOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { HttpUtil } from '@/utils';

type Provider = 'jitsi' | 'telemost' | 'wbstream';
type Transport = 'datachannel' | 'vp8channel' | 'seichannel' | 'videochannel';
type ClientQuota = {
  speed_mbps?: number;
  traffic_gb?: number;
  used_bytes?: number;
  used_gb?: number;
  expires_at?: string;
};
type ClientInput = {
  client_id: string;
  carrier: Provider;
  transport: Transport;
  room_id: string;
  key: string;
  dns: string;
  name: string;
  quota?: ClientQuota;
};
type Location = {
  name: string;
  carrier: Provider;
  endpoint: { room_id: string; key: string };
  transport: { type?: string; Type?: string; payload?: Record<string,string> };
  dns: string;
};
type Client = { id: string; locations: Location[]; quota?: ClientQuota; link: string };
type ApiMsg<T> = { success?: boolean; msg?: string; obj?: T };

const transportOptions: Record<Provider, { value: Transport; label: string }[]> = {
  jitsi: [
    { value: 'datachannel', label: 'DataChannel (рекомендуется)' },
    { value: 'vp8channel', label: 'VP8' },
    { value: 'seichannel', label: 'H264 SEI' },
    { value: 'videochannel', label: 'VideoChannel (экспериментально)' },
  ],
  telemost: [
    { value: 'vp8channel', label: 'VP8 (рекомендуется)' },
    { value: 'videochannel', label: 'VideoChannel (медленно)' },
  ],
  wbstream: [
    { value: 'vp8channel', label: 'VP8 (рекомендуется)' },
    { value: 'seichannel', label: 'H264 SEI' },
    { value: 'videochannel', label: 'VideoChannel (экспериментально)' },
  ],
};

export default function OlcRTCClientsModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [toast, holder] = message.useMessage();
  const [form] = Form.useForm<ClientInput>();
  const [provider, setProvider] = useState<Provider>('jitsi');
  const [clients, setClients] = useState<Client[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [working, setWorking] = useState<string | null>(null);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [error, setError] = useState('');
  const load = useCallback(async () => {
    setLoading(true);
    try {
      const result = (await HttpUtil.get('/panel/api/olcrtc/clients')) as ApiMsg<Client[]>;
      if (!result.success) throw new Error(result.msg || 'Не удалось загрузить подключения');
      setClients(result.obj || []);
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!open) return;
    const timer = window.setTimeout(() => {
      form.resetFields();
      setProvider('jitsi');
      setEditingID(null);
      void load();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open, form, load]);

  const save = async (values: ClientInput) => {
    setSaving(true);
    try {
      const url = editingID
        ? `/panel/api/olcrtc/clients/${encodeURIComponent(editingID)}/update`
        : '/panel/api/olcrtc/clients';
      const result = (await HttpUtil.post(url, values)) as ApiMsg<ClientInput>;
      if (!result.success) throw new Error(result.msg || 'Не удалось сохранить подключение');
      toast.success(
        editingID
          ? 'Настройки подключения обновлены.'
          : 'Подключение создано. Скопируйте ссылку клиента.',
      );
      form.resetFields();
      setProvider('jitsi');
      setEditingID(null);
      await load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const edit = (client: Client) => {
    if (client.locations.length !== 1) return;
    const location = client.locations[0];
    const carrier = location.carrier;
    const transport = (location.transport?.type ||
      location.transport?.Type ||
      'datachannel') as Transport;
    setEditingID(client.id);
    setProvider(carrier);
    form.setFieldsValue({
      client_id: client.id,
      carrier,
      transport,
      room_id: location.endpoint.room_id,
      key: location.endpoint.key,
      dns: location.dns,
      name: location.name || 'Main',
      quota: {
        speed_mbps: client.quota?.speed_mbps ?? 0,
        traffic_gb: client.quota?.traffic_gb ?? 0,
        expires_at: client.quota?.expires_at ?? '',
      },
    });
    document
      .getElementById('olcrtc-client-editor')
      ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  };
  const cancelEdit = () => {
    setEditingID(null);
    setProvider('jitsi');
    form.resetFields();
  };

  const act = async (id: string, action: 'delete' | 'rotate-key') => {
    setWorking(id);
    try {
      const result = (await HttpUtil.post(
        `/panel/api/olcrtc/clients/${encodeURIComponent(id)}/${action}`,
      )) as ApiMsg<unknown>;
      if (!result.success) throw new Error(result.msg || 'Не удалось изменить подключение');
      await load();
      if (editingID === id) cancelEdit();
      toast.success(
        action === 'delete'
          ? 'Подключение удалено.'
          : 'Ключ обновлён. Старые ссылки больше не работают.',
      );
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e));
    } finally {
      setWorking(null);
    }
  };

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={<Button onClick={onClose}>{t('close', { defaultValue: 'Закрыть' })}</Button>}
      title="OlcRTC — подключения"
      width={760}
      destroyOnHidden
    >
      {holder}
      <Typography.Paragraph type="secondary">
        OlcRTC запускается отдельной службой, а не как входящее подключение sing-box. Создайте
        клиента, выберите сервис видеосвязи и скопируйте готовую ссылку в совместимый клиент OlcRTC.
      </Typography.Paragraph>
      {error && <Alert type="error" showIcon title={error} style={{ marginBottom: 12 }} />}
      <Card
        size="small"
        title="Клиенты"
        extra={
          <Button
            size="small"
            icon={<ReloadOutlined />}
            loading={loading}
            onClick={() => void load()}
          >
            Обновить
          </Button>
        }
        style={{ marginBottom: 16 }}
      >
        {clients.length === 0 ? (
          <Typography.Text type="secondary">Подключений пока нет.</Typography.Text>
        ) : (
          <Space direction="vertical" style={{ width: '100%' }}>
            {clients.map((client) => (
              <Card key={client.id} size="small">
                <Space wrap style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Space wrap>
                    <Typography.Text strong>{client.id}</Typography.Text>
                    {client.locations[0] && (
                      <Tag>
                        {client.locations[0].carrier} /{' '}
                        {client.locations[0].transport?.type ||
                          client.locations[0].transport?.Type ||
                          'datachannel'}
                      </Tag>
                    )}
                  </Space>
                  <Space wrap>
                    <Button
                      size="small"
                      icon={<EditOutlined />}
                      disabled={
                        client.locations.length !== 1 ||
                        Object.keys(client.locations[0]?.transport?.payload || {}).length > 0
                      }
                      onClick={() => edit(client)}
                    >
                      Изменить
                    </Button>
                    <Button
                      size="small"
                      icon={<CopyOutlined />}
                      disabled={!client.link}
                      onClick={() => {
                        void navigator.clipboard.writeText(client.link).then(
                          () => toast.success('Ссылка скопирована.'),
                          () => toast.error('Браузер запретил копирование.'),
                        );
                      }}
                    >
                      Ссылка
                    </Button>
                    <Popconfirm
                      title="Изменить ключ?"
                      description="Старые подключения перестанут работать."
                      onConfirm={() => void act(client.id, 'rotate-key')}
                    >
                      <Button size="small" icon={<KeyOutlined />} loading={working === client.id} />
                    </Popconfirm>
                    <Popconfirm
                      title={`Удалить клиента ${client.id}?`}
                      onConfirm={() => void act(client.id, 'delete')}
                    >
                      <Button
                        danger
                        size="small"
                        icon={<DeleteOutlined />}
                        disabled={working !== null}
                      />
                    </Popconfirm>
                  </Space>
                </Space>
                <Space wrap>
                  {typeof client.quota?.speed_mbps === 'number' && client.quota.speed_mbps > 0 && (
                    <Tag>До {client.quota.speed_mbps} Мбит/с</Tag>
                  )}
                  {typeof client.quota?.traffic_gb === 'number' && client.quota.traffic_gb > 0 && (
                    <Tag>
                      Трафик: {((client.quota.used_bytes || 0) / 1073741824).toFixed(2)} / {client.quota.traffic_gb} ГБ
                    </Tag>
                  )}
                  {client.quota?.expires_at && <Tag>До {client.quota.expires_at}</Tag>}
                </Space>
                {client.locations.length > 1 && (
                  <Typography.Text type="secondary">
                    Локаций: {client.locations.length}. Ссылка выше относится к первой.
                  </Typography.Text>
                )}
              </Card>
            ))}
          </Space>
        )}
      </Card>
      <Card
        id="olcrtc-client-editor"
        size="small"
        title={editingID ? `Редактирование: ${editingID}` : 'Новое подключение'}
      >
        <Form<ClientInput>
          form={form}
          layout="vertical"
          onFinish={(values) => void save(values)}
          initialValues={{
            client_id: '',
            carrier: 'jitsi',
            transport: 'datachannel',
            room_id: '',
            key: '',
            dns: '1.1.1.1:53',
            name: 'Main',
            quota: { speed_mbps: 0, traffic_gb: 0, expires_at: '' },
          }}
        >
          <Form.Item name="client_id" label="Имя клиента (необязательно)">
            <Input
              maxLength={64}
              placeholder="Автоматически"
              autoComplete="off"
              disabled={editingID !== null}
            />
          </Form.Item>
          <Space wrap style={{ display: 'flex', alignItems: 'start' }}>
            <Form.Item name="carrier" label="Сервис видеосвязи" rules={[{ required: true }]}>
              <Select
                style={{ minWidth: 190 }}
                options={[
                  { label: 'Jitsi Meet', value: 'jitsi' },
                  { label: 'Яндекс Телемост', value: 'telemost' },
                  { label: 'WB Stream', value: 'wbstream' },
                ]}
                onChange={(next: Provider) => {
                  setProvider(next);
                  form.setFieldsValue({
                    transport: next === 'jitsi' ? 'datachannel' : 'vp8channel',
                    room_id: '',
                  });
                }}
              />
            </Form.Item>
            <Form.Item name="transport" label="Транспорт" rules={[{ required: true }]}>
              <Select style={{ minWidth: 190 }} options={transportOptions[provider]} />
            </Form.Item>
          </Space>
          <Form.Item
            name="room_id"
            label={provider === 'jitsi' ? 'Адрес комнаты (необязательно)' : 'ID комнаты'}
            extra={
              provider === 'jitsi'
                ? 'Оставьте пустым: безопасный случайный идентификатор комнаты Jitsi будет создан автоматически.'
                : 'Сначала создайте комнату у выбранного сервиса и вставьте её ID.'
            }
            rules={
              provider === 'jitsi'
                ? []
                : [{ required: true, message: 'Укажите ID существующей комнаты' }]
            }
          >
            <Input
              autoComplete="off"
              placeholder={provider === 'jitsi' ? 'https://meet.jit.si/...' : 'ID комнаты'}
            />
          </Form.Item>
          <Form.Item
            name="key"
            label="Ключ шифрования (необязательно)"
            extra="Случайный 256-битный ключ будет создан автоматически."
          >
            <Input.Password autoComplete="new-password" placeholder="Автоматически" />
          </Form.Item>
          <Form.Item name="dns" label="DNS сервера">
            <Input placeholder="1.1.1.1:53" />
          </Form.Item>
          <Typography.Text strong>Лимиты подключения (необязательно)</Typography.Text>
          <Typography.Paragraph type="secondary">
            Нулевые значения означают отсутствие ограничения. Уже использованный трафик
            сохраняется при изменении лимитов.
          </Typography.Paragraph>
          <Space wrap>
            <Form.Item name={['quota', 'speed_mbps']} label="Скорость, Мбит/с">
              <InputNumber min={0} max={1000000} placeholder="Без ограничений" />
            </Form.Item>
            <Form.Item name={['quota', 'traffic_gb']} label="Трафик, ГБ">
              <InputNumber min={0} max={1000000} placeholder="Без ограничений" />
            </Form.Item>
            <Form.Item name={['quota', 'expires_at']} label="Действует до">
              <Input placeholder="ГГГГ-ММ-ДД" />
            </Form.Item>
          </Space>
          <Space wrap>
            <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={saving}>
              {editingID ? 'Сохранить настройки' : 'Создать подключение'}
            </Button>
            {editingID && <Button onClick={cancelEdit}>Отмена</Button>}
          </Space>
        </Form>
      </Card>
      <Alert
        style={{ marginTop: 12 }}
        type="info"
        showIcon
        title="Совместимый клиент обязателен"
        description="OlcRTC использует отдельный клиентский протокол и формат olcrtc://. Обычные VLESS/VMess-конфигурации с ним не совместимы."
      />
    </Modal>
  );
}
