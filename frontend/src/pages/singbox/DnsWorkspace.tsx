import { Alert, Button, Card, Collapse, Space, Tag, Typography } from 'antd';

import DNSTTSettings from './DNSTTSettings';
import NativeDnsRules from './NativeDnsRules';
import NativeDnsServers from './NativeDnsServers';
import OxiDNSSettings from './OxiDNSSettings';
import SimpleSettings from './SimpleSettings';
import { applyDnsPreset, currentDnsPreset, object, records } from './simple-settings';
import type { ConfigObject } from './simple-settings';

type Props = {
  config: ConfigObject;
  onChange: (value: ConfigObject) => void;
  subscriptionTags?: string[];
};

export default function DnsWorkspace({ config, onChange, subscriptionTags = [] }: Props) {
  const dns = object(config.dns);
  const servers = records(dns.servers);
  const rules = records(dns.rules);
  const finalTag = String(dns.final || '');
  const resolvedFinal = servers.some((server) => server.tag === finalTag);
  const preset = currentDnsPreset(config);
  const missingFinal = Boolean(finalTag && !resolvedFinal);
  const noServers = servers.length === 0;

  return (
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      <Card className="singbox-item-card">
        <div className="singbox-section-header">
          <div>
            <div className="singbox-section-title">DNS sing-box</div>
            <div className="singbox-section-description">
              Настройте DNS в одном месте. Готовый профиль автоматически добавляет сервер,
              проверку TLS и DNS по умолчанию, сохраняя ваши правила.
            </div>
          </div>
          <Space wrap>
            <Tag color={noServers || missingFinal ? 'warning' : 'success'}>
              {noServers ? 'Не настроен' : missingFinal ? 'Проверьте конфигурацию' : 'Настроен'}
            </Tag>
            <Tag>{servers.length} серверов</Tag>
            <Tag>{rules.length} правил</Tag>
          </Space>
        </div>
        {(noServers || missingFinal) && (
          <Alert
            type="warning"
            showIcon
            style={{ marginBottom: 16 }}
            title={noServers ? 'DNS-сервер не выбран' : 'DNS по умолчанию отсутствует среди серверов'}
            description="Можно автоматически создать безопасный профиль. Изменения вступят в силу после сохранения настроек ядра."
            action={
              <Button
                size="small"
                type="primary"
                onClick={() => onChange(applyDnsPreset(config, 'quad9'))}
              >
                Настроить Quad9
              </Button>
            }
          />
        )}
        {preset === 'custom' && !noServers && !missingFinal && (
          <Typography.Paragraph type="secondary">
            Используется пользовательская конфигурация. Готовые профили не удаляют существующие
            серверы и правила DNS.
          </Typography.Paragraph>
        )}
      </Card>

      <SimpleSettings config={config} onChange={onChange} />

      <Collapse
        className="singbox-advanced-groups"
        items={[
          {
            key: 'servers',
            label: <>DNS-серверы <Tag>{servers.length}</Tag></>,
            children: (
              <NativeDnsServers
                config={config}
                onChange={onChange}
                subscriptionTags={subscriptionTags}
              />
            ),
          },
          {
            key: 'rules',
            label: <>Правила DNS <Tag>{rules.length}</Tag></>,
            children: <NativeDnsRules config={config} onChange={onChange} />,
          },
          {
            key: 'components',
            label: 'Дополнительные компоненты — OxiDNS и DNSTT',
            children: (
              <Space orientation="vertical" size={12} style={{ width: '100%' }}>
                <Alert
                  type="info"
                  showIcon
                  title="Независимые DNS-компоненты"
                  description="Параметры запуска и клиенты доступны только после установки соответствующего компонента. Установка выполняется через «Настройки → Системные пакеты и ядра»."
                />
                <OxiDNSSettings />
                <DNSTTSettings />
              </Space>
            ),
          },
        ]}
      />
      <Typography.Text type="secondary">
        Изменения настроек DNS sing-box применяются кнопкой «Сохранить» вверху страницы.
        Дополнительные службы управляются отдельно.
      </Typography.Text>
    </Space>
  );
}
