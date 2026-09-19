import { useEffect, useState } from 'react';
import { Alert, Button, Card, Col, ConfigProvider, Form, Input, InputNumber, Layout, Row, Space, Switch, Tag, Typography, message } from 'antd';
import { CopyOutlined, PlusOutlined, ReloadOutlined, PlayCircleOutlined, StopOutlined, SyncOutlined, ApiOutlined, SettingOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { useNavigate } from 'react-router';
import { HttpUtil, RandomUtil } from '@/utils';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import './TelemtPage.css';

type Status = { installed: boolean; active: boolean; enabled: boolean; configured: boolean; version: string; latestVersion: string; updateAvailable: boolean };
type Config = { enabled: boolean; port: number; secret: string; ipv4: boolean; ipv6: boolean; fastMode: boolean; classic: boolean; secure: boolean; tls: boolean; sni: string; upstreamType: string };
type Proxy = { name: string; secret: string; host: string; port: number; tls: boolean; link: string };
type CreateForm = { name: string; host: string };

const defaults: Config = { enabled: false, port: 8443, secret: '', ipv4: true, ipv6: true, fastMode: true, classic: false, secure: false, tls: true, sni: 'petrovich.ru', upstreamType: 'direct' };
const jsonOptions = { headers: { 'Content-Type': 'application/json' } };

export default function TelemtPage() {
  const { antdThemeConfig } = useTheme();
  const navigate = useNavigate();
  const [form] = Form.useForm<Config>();
  const [createForm] = Form.useForm<CreateForm>();
  const [status, setStatus] = useState<Status>({ installed: false, active: false, enabled: false, configured: false, version: '' });
  const [loading, setLoading] = useState(false);
  const [proxy, setProxy] = useState<Proxy | null>(null);

  const refresh = async () => {
    const [s, c] = await Promise.all([HttpUtil.get<Status>('/panel/api/telemt/status'), HttpUtil.get<Config>('/panel/api/telemt/config')]);
    if (s?.success && s.obj) setStatus(s.obj);
    if (c?.success && c.obj) form.setFieldsValue({ ...defaults, ...c.obj });
  };

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 60000);
    return () => window.clearInterval(timer);
  }, []);

  const generateSecret = () => {
    const secret = RandomUtil.randomSeq(32, { type: 'hex' });
    form.setFieldValue('secret', secret);
    message.success('Новый 32-символьный hex-секрет сгенерирован');
  };

  const save = async (v: Config) => {
    setLoading(true);
    try {
      const r = await HttpUtil.post('/panel/api/telemt/config', v, jsonOptions);
      if (r?.success) { message.success('Конфигурация Telemt сохранена'); await refresh(); }
      else message.error(r?.msg || 'Не удалось сохранить конфигурацию');
    } finally { setLoading(false); }
  };

  const createProxy = async (v: CreateForm) => {
    setLoading(true);
    try {
      const r = await HttpUtil.post<Proxy>('/panel/api/telemt/proxy', v, jsonOptions);
      if (r?.success && r.obj) {
        setProxy(r.obj);
        message.success('Прокси создан');
        await refresh();
        await action(status.active ? 'restart' : 'start');
      } else message.error(r?.msg || 'Не удалось создать прокси');
    } finally { setLoading(false); }
  };

  const action = async (a: 'start' | 'stop' | 'restart' | 'enable' | 'disable' | 'update') => {
    setLoading(true);
    try {
      const r = await HttpUtil.post('/panel/api/telemt/action', { action: a }, jsonOptions);
      if (r?.success) { message.success('Команда выполнена'); await refresh(); }
      else message.error(r?.msg || 'Команда не выполнена');
    } finally { setLoading(false); }
  };

  const copyLink = async () => {
    if (!proxy) return;
    await navigator.clipboard.writeText(proxy.link);
    message.success('Ссылка скопирована');
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
                  <Typography.Title level={2} className="telemt-title">Telemt</Typography.Title>
                  <Typography.Text type="secondary">MTProto-прокси в составе панели 3X-UI</Typography.Text>
                </div>
                <Button htmlType="button" icon={<ReloadOutlined />} onClick={refresh}>Обновить</Button>
              </div>

              {!status.installed && (
                <Alert type="warning" showIcon message="Бинарник Telemt не установлен" description="Установите актуальную сборку проекта — бинарник и systemd-служба будут установлены автоматически." />
              )}

              <Row gutter={[16, 16]} className="telemt-status-grid">
                <Col xs={24} sm={12} lg={6}><Card className="telemt-status-card"><Typography.Text type="secondary">Версия бинарника</Typography.Text><Typography.Title level={4}>{status.version || '—'}</Typography.Title>{status.updateAvailable && <Typography.Text type="warning">Доступна {status.latestVersion}</Typography.Text>}</Card></Col>
                <Col xs={24} sm={12} lg={6}><Card className="telemt-status-card"><Typography.Text type="secondary">Бинарник</Typography.Text><Typography.Title level={4}><Tag color={status.installed ? 'green' : 'red'}>{status.installed ? 'Установлен' : 'Не установлен'}</Tag></Typography.Title></Card></Col>
                <Col xs={24} sm={12} lg={6}><Card className="telemt-status-card"><Typography.Text type="secondary">Сервис</Typography.Text><Typography.Title level={4}><Tag color={status.active ? 'green' : 'default'}>{status.active ? 'Запущен' : 'Остановлен'}</Tag></Typography.Title></Card></Col>
                <Col xs={24} sm={12} lg={6}><Card className="telemt-status-card"><Typography.Text type="secondary">Автозапуск</Typography.Text><Typography.Title level={4}><Tag color={status.enabled ? 'green' : 'default'}>{status.enabled ? 'Включён' : 'Выключен'}</Tag></Typography.Title></Card></Col>
              </Row>

              <Card title={<Space><ApiOutlined /> Управление прокси</Space>} className="telemt-card">
                <Form form={createForm} layout="vertical" onFinish={createProxy}>
                  <Row gutter={16}>
                    <Col xs={24} md={8}><Form.Item name="name" label="Название прокси" rules={[{ required: true, message: 'Введите название' }]}><Input placeholder="Telegram Proxy" /></Form.Item></Col>
                    <Col xs={24} md={16}><Form.Item name="host" label="Публичный адрес" rules={[{ required: true, message: 'Введите IP или домен' }]}><Input placeholder="proxy.example.com или IP-адрес" /></Form.Item></Col>
                  </Row>
                  <Button type="primary" htmlType="submit" icon={<PlusOutlined />} loading={loading} disabled={!status.installed}>Создать прокси</Button>
                </Form>
                {proxy && <div className="telemt-result"><Typography.Text strong>{proxy.name}</Typography.Text><Typography.Paragraph copyable={{ text: proxy.link }} code>{proxy.link}</Typography.Paragraph><Space><Button htmlType="button" icon={<CopyOutlined />} onClick={copyLink}>Копировать ссылку</Button><Tag color={proxy.tls ? 'green' : 'default'}>{proxy.tls ? 'TLS' : 'Classic'}</Tag></Space></div>}
              </Card>

              <Card title={<Space><SettingOutlined /> Конфигурация Telemt</Space>} className="telemt-card">
                <Form form={form} layout="vertical" onFinish={save} initialValues={defaults}>
                  <Row gutter={16}>
                    <Col xs={24} md={8}><Form.Item name="port" label="Порт" rules={[{ required: true }, { type: 'number', min: 1, max: 65535 }]}><InputNumber style={{ width: '100%' }} /></Form.Item></Col>
                    <Col xs={24} md={16}>
                      <Form.Item name="secret" label="Секрет" rules={[{ required: true }, { pattern: /^[0-9a-fA-F]{32}$/, message: 'Нужно ровно 32 hex-символа' }]}>
                        <Space.Compact block>
                          <Input placeholder="0123456789abcdef0123456789abcdef" />
                          <Button htmlType="button" icon={<SafetyCertificateOutlined />} onClick={generateSecret}>gen</Button>
                        </Space.Compact>
                      </Form.Item>
                    </Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={12}><Form.Item name="sni" label="SNI" tooltip="Домен, который используется как tls_domain для Fake-TLS"><Input placeholder="www.example.com" /></Form.Item></Col>
                    <Col xs={24} md={12}><Form.Item name="tls" label="Fake-TLS (ee)" valuePropName="checked"><Switch onChange={(checked) => { if (checked && !form.getFieldValue("sni")) form.setFieldValue("sni", "petrovich.ru"); }} /></Form.Item></Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={8}><Form.Item name="ipv4" label="Разрешить IPv4" valuePropName="checked"><Switch /></Form.Item></Col>
                    <Col xs={24} md={8}><Form.Item name="ipv6" label="Разрешить IPv6" valuePropName="checked"><Switch /></Form.Item></Col>
                    <Col xs={24} md={8}><Form.Item name="fastMode" label="Fast mode" valuePropName="checked"><Switch /></Form.Item></Col>
                  </Row>
                  <Row gutter={16}>
                    <Col xs={24} md={8}><Form.Item name="classic" label="Classic" valuePropName="checked"><Switch /></Form.Item></Col>
                    <Col xs={24} md={8}><Form.Item name="secure" label="Secure" valuePropName="checked"><Switch /></Form.Item></Col>
                  </Row>
                  <Space wrap>
                    <Button type="primary" htmlType="submit" loading={loading}>Сохранить</Button>
                    <Button htmlType="button" icon={<PlayCircleOutlined />} onClick={() => action('start')} disabled={!status.installed}>Запустить</Button>
                    <Button htmlType="button" icon={<StopOutlined />} onClick={() => action('stop')} disabled={!status.active}>Остановить</Button>
                    <Button htmlType="button" icon={<SyncOutlined />} onClick={() => action('restart')} disabled={!status.installed}>Перезапустить</Button>
                    <Button htmlType="button" onClick={() => action(status.enabled ? 'disable' : 'enable')}>{status.enabled ? 'Отключить автозапуск' : 'Включить автозапуск'}</Button>
                    <Button htmlType="button" icon={<SyncOutlined />} onClick={() => action('update')} loading={loading} disabled={!status.installed || !status.updateAvailable}>Обновить Telemt</Button>
                  </Space>
                </Form>
              </Card>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
