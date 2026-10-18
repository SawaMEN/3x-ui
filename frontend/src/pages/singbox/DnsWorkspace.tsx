import { useEffect, useState } from 'react';
import { Alert, Button, Card, Collapse, Space, Tag, Typography } from 'antd';

import DNSTTSettings from './DNSTTSettings';
import NativeDnsRules from './NativeDnsRules';
import NativeDnsServers from './NativeDnsServers';
import OxiDNSSettings from './OxiDNSSettings';
import SimpleSettings from './SimpleSettings';
import { applyDnsPreset, currentDnsPreset, object, records } from './simple-settings';
import type { ConfigObject } from './simple-settings';
import { HttpUtil } from '@/utils';

type InstalledResponse = { success?: boolean; obj?: { installed?: boolean } };
type ServiceAvailability = {
  checked: boolean;
  oxidns: boolean;
  dnstt: boolean;
  error: boolean;
};


type Props = {
  config: ConfigObject;
  onChange: (value: ConfigObject) => void;
  subscriptionTags?: string[];
};

export default function DnsWorkspace({ config, onChange, subscriptionTags = [] }: Props) {
  const [services, setServices] = useState<ServiceAvailability>({
    checked: false,
    oxidns: false,
    dnstt: false,
    error: false,
  });
  useEffect(() => {
    let active = true;
    const refresh = async () => {
      const results = await Promise.allSettled([
        HttpUtil.get('/panel/api/oxidns/status', undefined, { silent: true }) as Promise<InstalledResponse>,
        HttpUtil.get('/panel/api/dnstt/status', undefined, { silent: true }) as Promise<InstalledResponse>,
      ]);
      if (!active) return;
      const installed = (index: number) => {
        const result = results[index];
        return result.status === 'fulfilled' &&
          result.value.success === true &&
          result.value.obj?.installed === true;
      };
      setServices({
        checked: true,
        oxidns: installed(0),
        dnstt: installed(1),
        error: results.some(
          (result) => result.status === 'rejected' || result.value.success !== true,
        ),
      });
    };
    void refresh();
    window.addEventListener('oxidns:changed', refresh);
    window.addEventListener('dnstt:changed', refresh);
    return () => {
      active = false;
      window.removeEventListener('oxidns:changed', refresh);
      window.removeEventListener('dnstt:changed', refresh);
    };
  }, []);
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
              Настройте DNS в одном месте. Готовый профиль автоматически добавляет сервер, проверку
              TLS и DNS по умолчанию, сохраняя ваши правила.
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
            title={
              noServers ? 'DNS-сервер не выбран' : 'DNS по умолчанию отсутствует среди серверов'
            }
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
            label: (
              <>
                DNS-серверы <Tag>{servers.length}</Tag>
              </>
            ),
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
            label: (
              <>
                Правила DNS <Tag>{rules.length}</Tag>
              </>
            ),
            children: <NativeDnsRules config={config} onChange={onChange} />,
          },
          ...(services.oxidns || services.dnstt
            ? [
                {
                  key: 'components',
                  label: 'Установленные DNS-компоненты',
                  children: (
                    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
                      {services.oxidns && <OxiDNSSettings />}
                      {services.dnstt && <DNSTTSettings />}
                    </Space>
                  ),
                },
              ]
            : []),
        ]}
      />
      {services.checked && services.error && (
        <Alert
          type="warning"
          showIcon
          title="Не удалось проверить установку всех DNS-компонентов"
          description="Настройки компонентов со сбойной проверкой скрыты. Обновите страницу после восстановления соединения с панелью."
        />
      )}
      {services.checked && !services.error && !services.oxidns && !services.dnstt && (
        <Typography.Text type="secondary">
          Дополнительные DNS-компоненты не установлены. Установка доступна в разделе
          «Настройки → Системные пакеты и ядра».
        </Typography.Text>
      )}
      <Typography.Text type="secondary">
        Изменения настроек DNS sing-box применяются кнопкой «Сохранить» вверху страницы.
        Дополнительные службы управляются отдельно.
      </Typography.Text>
    </Space>
  );
}
