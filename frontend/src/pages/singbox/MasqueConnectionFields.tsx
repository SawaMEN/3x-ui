import { Alert, Col, Form, Input, InputNumber, Row, Select, Switch } from 'antd';
import { object, type ConfigObject } from './simple-settings';
import { MASQUE_PATH } from './masque-settings';

export default function MasqueConnectionFields({
  value,
  onChange,
}: {
  value: ConfigObject;
  onChange: (next: ConfigObject) => void;
}) {
  const warp = value.type === 'masque',
    tls = object(value.tls),
    profile = object(value.profile);
  const patch = (key: string, next: unknown) => onChange({ ...value, [key]: next });
  const text = (key: string, secret = false, placeholder?: string) => {
    const props = {
      value: typeof value[key] === 'string' ? (value[key] as string) : '',
      placeholder,
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => patch(key, e.target.value),
    };
    return secret ? <Input.Password {...props} /> : <Input {...props} />;
  };
  return (
    <>
      <Alert
        type="info"
        showIcon
        style={{ marginBottom: 12 }}
        description={
          warp
            ? 'WARP через MASQUE: ядро автоматически зарегистрирует отдельное устройство при запуске и сохранит профиль в кэше. Существующий WireGuard WARP не изменяется. После сохранения выберите это подключение в маршрутизации.'
            : 'CONNECT-IP к вашему MASQUE-серверу. Параметры пользователя доступны в JSON-подписке sing-box. TLS включён по умолчанию; HTTP/3 автоматически переключается на HTTP/2 при необходимости.'
        }
      />
      <Row gutter={[12, 0]}>
        {warp ? (
          <>
            <Col xs={24} sm={12}>
              <Form.Item label="Транспорт WARP">
                <Select
                  value={value.use_http2 ? 2 : 3}
                  options={[
                    { value: 3, label: 'HTTP/3 — QUIC / UDP' },
                    { value: 2, label: 'HTTP/2 — TLS / TCP' },
                  ]}
                  onChange={(next) => patch('use_http2', next === 2)}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Подключаться по IPv6">
                <Switch checked={!!value.use_ipv6} onChange={(next) => patch('use_ipv6', next)} />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item label="Регистрация через другое подключение">
                <Input
                  value={String(profile.detour || '')}
                  placeholder="Необязательно: tag прокси для Cloudflare API"
                  onChange={(e) => patch('profile', { ...profile, detour: e.target.value })}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Задержка переподключения">
                {text('reconnect_delay', false, '5s')}
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="UDP keepalive">
                {text('udp_keepalive_period', false, '30s')}
              </Form.Item>
            </Col>
          </>
        ) : (
          <>
            <Col xs={24} sm={12}>
              <Form.Item label="Имя пользователя">{text('username')}</Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Пароль">{text('password', true)}</Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item label="Путь CONNECT-IP">{text('path', false, MASQUE_PATH)}</Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Версия HTTP">
                <Select
                  value={Number(value.version) || 3}
                  options={[
                    { value: 3, label: 'HTTP/3 (QUIC)' },
                    { value: 2, label: 'HTTP/2' },
                    { value: 1, label: 'HTTP/1.1' },
                  ]}
                  onChange={(next) => patch('version', next)}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Автоматический переход на HTTP/2">
                <Switch
                  checked={!value.disable_version_fallback}
                  onChange={(next) => patch('disable_version_fallback', !next)}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="MTU">
                <InputNumber
                  style={{ width: '100%' }}
                  min={1280}
                  max={65535}
                  value={Number(value.mtu) || 1280}
                  onChange={(next) => patch('mtu', next || 1280)}
                />
              </Form.Item>
            </Col>
            <Col xs={24} sm={12}>
              <Form.Item label="Подключение по требованию">
                <Switch checked={!!value.on_demand} onChange={(next) => patch('on_demand', next)} />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item label="Объявляемые маршруты">
                <Select
                  mode="tags"
                  tokenSeparators={[',', ' ']}
                  value={
                    Array.isArray(value.advertise_routes)
                      ? (value.advertise_routes as string[])
                      : []
                  }
                  onChange={(next) => patch('advertise_routes', next)}
                />
              </Form.Item>
            </Col>
            <Col xs={24}>
              <Form.Item label="SNI">
                <Input
                  value={String(tls.server_name || '')}
                  placeholder={String(value.server || '')}
                  onChange={(e) =>
                    patch('tls', { ...tls, enabled: true, server_name: e.target.value })
                  }
                />
              </Form.Item>
            </Col>
          </>
        )}
        <Col xs={24} sm={12}>
          <Form.Item label="UDP timeout">{text('udp_timeout', false, '5m')}</Form.Item>
        </Col>
      </Row>
    </>
  );
}
