import { Alert, Card, Col, Row, Select, Switch } from 'antd';
import { applyDnsPreset, currentDnsPreset, object, setDnsCache } from './simple-settings';
import type { ConfigObject, DnsPreset } from './simple-settings';

export default function SimpleSettings({
  config,
  onChange,
}: {
  config: ConfigObject;
  onChange: (config: ConfigObject) => void;
}) {
  const dns = object(config.dns);
  const preset = currentDnsPreset(config);
  return (
    <Card title="Определение адресов сайтов (DNS)">
      <p className="singbox-description">
        Выберите готовый сервис: адрес, порт и проверка TLS настроятся автоматически. Системный DNS
        использует настройки вашего сервера.
      </p>
      <Row gutter={[16, 20]}>
        <Col xs={24}>
          <label className="singbox-field-label" htmlFor="singbox-simple-dns">
            DNS-сервис
          </label>
          <Select
            id="singbox-simple-dns"
            style={{ width: '100%' }}
            value={preset}
            options={[
              { value: 'system', label: 'Системный — настройки сервера' },
              { value: 'cloudflare', label: 'Cloudflare — зашифрованный DNS' },
              {
                value: 'quad9',
                label: 'Quad9 — зашифрованный DNS с защитой от вредоносных доменов',
              },
              ...(preset === 'custom'
                ? [{ value: 'custom', label: 'Ваши текущие настройки', disabled: true }]
                : []),
            ]}
            onChange={(value: DnsPreset | 'custom') => {
              if (value !== 'custom') onChange(applyDnsPreset(config, value));
            }}
          />
        </Col>
        <Col xs={24} md={16}>
          <label className="singbox-field-label" htmlFor="singbox-simple-strategy">
            Выбор IP-адреса
          </label>
          <Select
            id="singbox-simple-strategy"
            style={{ width: '100%' }}
            value={String(dns.strategy || '')}
            options={[
              { value: '', label: 'Автоматически — выбор sing-box' },
              { value: 'prefer_ipv4', label: 'Сначала IPv4' },
              { value: 'prefer_ipv6', label: 'Сначала IPv6' },
              { value: 'ipv4_only', label: 'Только IPv4 — если IPv6 не работает' },
              { value: 'ipv6_only', label: 'Только IPv6' },
            ]}
            onChange={(strategy) => {
              const next = { ...dns };
              if (strategy) next.strategy = strategy;
              else delete next.strategy;
              onChange({ ...config, dns: next });
            }}
          />
        </Col>
        <Col xs={24} md={8}>
          <div className="singbox-switch-setting">
            <div>
              <span>Кэш DNS</span>
              <p className="singbox-description">Ускоряет повторные запросы</p>
            </div>
            <Switch
              aria-label="Кэш DNS"
              checked={dns.disable_cache !== true}
              onChange={(enabled) => onChange(setDnsCache(config, enabled))}
            />
          </div>
        </Col>
      </Row>
      <p className="singbox-description">
        Готовые HTTPS-сервисы подключаются напрямую с этого сервера. Правила DNS и отдельные серверы
        сохраняются; правила имеют приоритет над выбранным сервисом.
      </p>
      {preset === 'custom' && (
        <Alert
          type="info"
          showIcon
          title="Используется ваша конфигурация DNS"
          description="Она сохранится, пока вы не выберете готовый сервис. Для редактирования откройте вкладку «DNS-серверы»."
        />
      )}
    </Card>
  );
}
