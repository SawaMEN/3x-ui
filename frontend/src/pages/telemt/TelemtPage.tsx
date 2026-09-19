import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Form,
  Input,
  InputNumber,
  Layout,
  Row,
  Space,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  PlayCircleOutlined,
  StopOutlined,
  SyncOutlined,
  ApiOutlined,
  SettingOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { HttpUtil, RandomUtil } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import './TelemtPage.css';

type Status = {
  installed: boolean;
  active: boolean;
  enabled: boolean;
  configured: boolean;
  version: string;
  latestVersion: string;
  updateAvailable: boolean;
  mekoEnabled: boolean;
};
type Config = {
  enabled: boolean;
  port: number;
  secret: string;
  ipv4: boolean;
  ipv6: boolean;
  fastMode: boolean;
  classic: boolean;
  secure: boolean;
  tls: boolean;
  sni: string;
  upstreamType: string;
};
type Proxy = {
  name: string;
  secret: string;
  host: string;
  port: number;
  tls: boolean;
  link: string;
};
type CreateForm = { name: string };

const defaults: Config = {
  enabled: false,
  port: 8443,
  secret: '',
  ipv4: true,
  ipv6: true,
  fastMode: true,
  classic: false,
  secure: false,
  tls: true,
  sni: 'petrovich.ru',
  upstreamType: 'direct',
};
const jsonOptions = { headers: { 'Content-Type': 'application/json' } };

export default function TelemtPage() {
  const { antdThemeConfig } = useTheme();
  const [form] = Form.useForm<Config>();
  const [createForm] = Form.useForm<CreateForm>();
  const [status, setStatus] = useState<Status>({
    installed: false,
    active: false,
    enabled: false,
    configured: false,
    version: '',
    latestVersion: '',
    updateAvailable: false,
    mekoEnabled: false,
  });
  const [loading, setLoading] = useState(false);
  const [proxies, setProxies] = useState<Proxy[]>([]);
  const refreshInFlight = useRef<Promise<void> | null>(null);

  const refresh = useCallback(async () => {
    if (refreshInFlight.current) return refreshInFlight.current;
    refreshInFlight.current = (async () => {
      const [s, c] = await Promise.all([
        HttpUtil.get<Status>('/panel/api/telemt/status'),
        HttpUtil.get<Config>('/panel/api/telemt/config'),
      ]);
      if (s?.success && s.obj) setStatus(s.obj);
      if (c?.success && c.obj) form.setFieldsValue({ ...defaults, ...c.obj });
      const p = await HttpUtil.get<Proxy[]>('/panel/api/telemt/proxy');
      if (p?.success && Array.isArray(p.obj)) setProxies(p.obj);
    })();
    try {
      await refreshInFlight.current;
    } finally {
      refreshInFlight.current = null;
    }
  }, [form]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 10000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const generateSecret = () => {
    const secret = RandomUtil.randomSeq(32, { type: 'hex' });
    form.setFieldValue('secret', secret);
    message.success('Новый 32-символьный hex-секрет сгенерирован');
  };

  const save = async (v: Config) => {
    setLoading(true);
    try {
      const r = await HttpUtil.post('/panel/api/telemt/config', v, jsonOptions);
      if (r?.success) {
        message.success('Конфигурация Telemt сохранена');
        await refresh();
      } else message.error(r?.msg || 'Не удалось сохранить конфигурацию');
    } finally {
      setLoading(false);
    }
  };

  const createProxy = async (v: CreateForm) => {
    setLoading(true);
    try {
      const r = await HttpUtil.post<Proxy>('/panel/api/telemt/proxy', v, jsonOptions);
      if (r?.success && r.obj) {
        message.success('Прокси создан');
        await refresh();
        await action(status.active ? 'restart' : 'start');
      } else message.error(r?.msg || 'Не удалось создать прокси');
    } finally {
      setLoading(false);
    }
  };

  const action = async (
    a:
      | 'start'
      | 'stop'
      | 'restart'
      | 'enable'
      | 'disable'
      | 'update'
      | 'meko-enable'
      | 'meko-disable',
  ) => {
    setLoading(true);
    try {
      const r = await HttpUtil.post('/panel/api/telemt/action', { action: a }, jsonOptions);
      if (r?.success) {
        message.success('Команда выполнена');
        await refresh();
      } else message.error(r?.msg || 'Команда не выполнена');
    } finally {
      setLoading(false);
    }
  };

  const deleteProxy = async (name: string) => {
    setLoading(true);
    try {
      const r = await HttpUtil.delete('/panel/api/telemt/proxy/' + encodeURIComponent(name));
      if (r?.success) {
        message.success('Ссылка удалена');
        await refresh();
      } else message.error(r?.msg || 'Не удалось удалить ссылку');
    } finally {
      setLoading(false);
    }
  };

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className="page-layout telemt-page">
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="telemt-shell">
              <div className="telemt-header">
                <div>
                  <Typography.Title level={2} className="telemt-title">
                    Telemt
                  </Typography.Title>
                  <Typography.Text type="secondary">
                    MTProto-прокси в составе панели 3X-UI
                  </Typography.Text>
                </div>
                <Button htmlType="button" icon={<ReloadOutlined />} onClick={refresh}>
                  Обновить
                </Button>
              </div>
              {!status.installed && (
                <Alert
                  type="warning"
                  showIcon
                  message="Бинарник Telemt не установлен"
                  description="Установите актуальную сборку проекта — бинарник и systemd-служба будут установлены автоматически."
                />
              )}
              <Row gutter={[16, 16]} className="telemt-status-grid">
                <Col xs={24} sm={12} lg={6}>
                  <Card className="telemt-status-card">
                    <Typography.Text type="secondary">Версия бинарника</Typography.Text>
                    <Typography.Title level={4}>{status.version || '—'}</Typography.Title>
                    {status.updateAvailable && (
                      <Typography.Text type="warning">
                        Доступна {status.latestVersion}
                      </Typography.Text>
                    )}
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card className="telemt-status-card">
                    <Typography.Text type="secondary">Бинарник</Typography.Text>
                    <Typography.Title level={4}>
                      <Tag color={status.installed ? 'green' : 'red'}>
                        {status.installed ? 'Установлен' : 'Не установлен'}
                      </Tag>
                    </Typography.Title>
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card className="telemt-status-card">
                    <Typography.Text type="secondary">Сервис</Typography.Text>
                    <Typography.Title level={4}>
                      <Tag color={status.active ? 'green' : 'default'}>
                        {status.active ? 'Запущен' : 'Остановлен'}
                      </Tag>
                    </Typography.Title>
                  </Card>
                </Col>
                <Col xs={24} sm={12} lg={6}>
                  <Card className="telemt-status-card">
                    <Typography.Text type="secondary">Автозапуск</Typography.Text>
                    <Typography.Title level={4}>
                      <Tag color={status.enabled ? 'green' : 'default'}>
                        {status.enabled ? 'Включён' : 'Выключен'}
                      </Tag>
                    </Typography.Title>
                  </Card>
                </Col>
              </Row>
              <Card
                title={
                  <Space>
                    <ApiOutlined /> Управление прокси
                  </Space>
                }
                className="telemt-card"
              >
                <Form form={createForm} layout="vertical" onFinish={createProxy}>
                  <Form.Item
                    name="name"
                    label="Название прокси"
                    rules={[{ required: true, message: 'Введите название' }]}
                  >
                    <Input placeholder="Telegram Proxy" />
                  </Form.Item>
                  <Typography.Text type="secondary">
                    Публичный адрес определяется автоматически по домену или IP, с которого открыта
                    панель.
                  </Typography.Text>
                  <div style={{ marginTop: 16 }}>
                    <Button
                      type="primary"
                      htmlType="submit"
                      icon={<PlusOutlined />}
                      loading={loading}
                      disabled={!status.installed}
                    >
                      Создать прокси
                    </Button>
                  </div>
                </Form>
                {proxies.length > 0 && (
                  <div className="telemt-proxy-list">
                    {proxies.map((item) => (
                      <div className="telemt-proxy-item" key={item.name}>
                        <div>
                          <Typography.Text strong>{item.name}</Typography.Text>
                          <Typography.Paragraph
                            copyable={{ text: item.link }}
                            ellipsis={{ rows: 1 }}
                            code
                            style={{ margin: '4px 0 0' }}
                          >
                            {item.link}
                          </Typography.Paragraph>
                        </div>
                        <Space wrap>
                          <Tag>
                            {item.host}:{item.port}
                          </Tag>
                          <Button
                            htmlType="button"
                            icon={<CopyOutlined />}
                            onClick={() => navigator.clipboard.writeText(item.link)}
                          >
                            Копировать
                          </Button>
                          <Button
                            htmlType="button"
                            danger
                            icon={<DeleteOutlined />}
                            loading={loading}
                            onClick={() => void deleteProxy(item.name)}
                          >
                            Удалить
                          </Button>
                        </Space>
                      </div>
                    ))}
                  </div>
                )}
              </Card>
              <Card
                title={
                  <Space>
                    <SafetyCertificateOutlined /> MEKO V3 fix
                  </Space>
                }
                className="telemt-card"
              >
                <Alert
                  type="info"
                  showIcon
                  message="Фикс by MEKO для Telemt"
                  description="V3 использует u32 fingerprint для распознавания iOS-клиентов и ограничивает новые TCP SYN для остальных клиентов. Порт берётся автоматически из настройки Telemt."
                />
                <Space wrap style={{ marginTop: 16 }}>
                  <Tag color={status.mekoEnabled ? 'green' : 'default'}>
                    {status.mekoEnabled ? 'MEKO включён' : 'MEKO выключен'}
                  </Tag>
                  <Button
                    htmlType="button"
                    type={status.mekoEnabled ? 'default' : 'primary'}
                    icon={<SafetyCertificateOutlined />}
                    onClick={() => action(status.mekoEnabled ? 'meko-disable' : 'meko-enable')}
                    disabled={!status.installed || !status.active}
                    loading={loading}
                  >
                    {status.mekoEnabled ? 'Выключить MEKO V3' : 'Включить MEKO V3'}
                  </Button>
                </Space>
                <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
                  Параметры V3 фиксированы в системном сервисе: iOS fingerprint — без лимита,
                  остальные клиенты — до 54 новых SYN в минуту с burst 1, затем TCP reset.
                </Typography.Paragraph>
              </Card>
              <Card
                title={
                  <Space>
                    <SettingOutlined /> Конфигурация Telemt
                  </Space>
                }
                className="telemt-card"
              >
                <Form form={form} layout="vertical" onFinish={save} initialValues={defaults}>
                  <Row gutter={16}>
                    <Col xs={24} md={8}>
                      <Form.Item
                        name="port"
                        label="Порт"
                        rules={[{ required: true }, { type: 'number', min: 1, max: 65535 }]}
                      >
                        <InputNumber style={{ width: '100%' }} />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={16}>
                      <Form.Item label="Секрет" required>
                        <Space.Compact block>
                          <Form.Item
                            name="secret"
                            noStyle
                            rules={[
                              { required: true },
                              {
                                pattern: /^[0-9a-fA-F]{32}$/,
                                message: 'Нужно ровно 32 hex-символа',
                              },
                            ]}
                          >
                            <Input placeholder="0123456789abcdef0123456789abcdef" />
                          </Form.Item>
                          <Button
                            htmlType="button"
                            icon={<SafetyCertificateOutlined />}
                            onClick={generateSecret}
                          >
                            gen
                          </Button>
                        </Space.Compact>
                      </Form.Item>
                    </Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={12}>
                      <Form.Item name="sni" label="SNI" rules={[{ required: true }]}>
                        <Input placeholder="petrovich.ru" />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={12}>
                      <Form.Item name="tls" label="Fake-TLS (ee)" valuePropName="checked">
                        <Switch checkedChildren="Включён" unCheckedChildren="Выключен" />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={8}>
                      <Form.Item name="ipv4" label="IPv4" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                      <Form.Item name="ipv6" label="IPv6" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                    <Col xs={24} md={8}>
                      <Form.Item name="fastMode" label="Fast mode" valuePropName="checked">
                        <Switch />
                      </Form.Item>
                    </Col>
                  </Row>
                  <Button type="primary" htmlType="submit" loading={loading}>
                    Сохранить конфигурацию
                  </Button>
                </Form>
              </Card>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
