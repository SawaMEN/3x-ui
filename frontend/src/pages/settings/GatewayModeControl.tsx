import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Input, InputNumber, Popconfirm, Space, Tag, Typography, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { onNumber } from '@/utils/onNumber';

type ApiMsg<T = unknown> = {
  success?: boolean;
  msg?: string;
  obj?: T;
};

type GatewayNetworkConfig = {
  lanInterface: string;
  lanIP: string;
  lanPrefix: number;
  wanInterface?: string;
};

type GatewayNetworkStatus = {
  configured: boolean;
  active: boolean;
  persistent: boolean;
  config: GatewayNetworkConfig;
  forwarding: boolean;
  policyRoute: boolean;
  nftables: boolean;
};

type GatewayStatus = {
  enabled: boolean;
  configured: boolean;
  recoveryBackup: boolean;
  canEnable: boolean;
  coreType: string;
  gatewayCoreType?: string;
  coreMismatch?: boolean;
  conflict?: boolean;
  port: number;
  network?: GatewayNetworkStatus;
};

const emptyNetworkConfig: GatewayNetworkConfig = {
  lanInterface: '',
  lanIP: '',
  lanPrefix: 24,
  wanInterface: '',
};

function coreLabel(core?: string) {
  if (core === 'sing-box') return 'sing-box';
  if (core === 'xray') return 'Xray';
  if (core === 'multiple') return 'Xray + sing-box';
  return 'не определено';
}

export default function GatewayModeControl() {
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [networkConfig, setNetworkConfig] = useState<GatewayNetworkConfig>(emptyNetworkConfig);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const loadStatus = useCallback(
    async (quiet = false): Promise<GatewayStatus | null> => {
      try {
        const response = (await HttpUtil.get('/panel/api/gateway/status', undefined, {
          silent: true,
        })) as ApiMsg<GatewayStatus>;
        if (!response?.success || !response.obj) {
          if (!quiet) {
            messageApi.error(response?.msg || 'Не удалось получить состояние режима шлюза');
          }
          return null;
        }
        return response.obj;
      } catch (error) {
        if (!quiet) {
          messageApi.error(
            error instanceof Error ? error.message : 'Не удалось получить состояние режима шлюза',
          );
        }
        return null;
      }
    },
    [messageApi],
  );

  const applyStatus = useCallback((nextStatus: GatewayStatus) => {
    setStatus(nextStatus);
    if (nextStatus.network?.configured && nextStatus.network.config) {
      setNetworkConfig(nextStatus.network.config);
    }
  }, []);

  const refresh = useCallback(
    async (quiet = false) => {
      const nextStatus = await loadStatus(quiet);
      if (nextStatus) applyStatus(nextStatus);
      setLoading(false);
    },
    [applyStatus, loadStatus],
  );

  useEffect(() => {
    let active = true;

    void loadStatus()
      .then((nextStatus) => {
        if (active && nextStatus) applyStatus(nextStatus);
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [applyStatus, loadStatus]);

  useEffect(() => {
    const onSettingsSaved = () => void refresh(true);
    window.addEventListener('xui-core-settings-saved', onSettingsSaved);
    return () => window.removeEventListener('xui-core-settings-saved', onSettingsSaved);
  }, [refresh]);

  const runAction = async (action: 'enable' | 'disable') => {
    setBusy(true);
    try {
      const data = action === 'enable' ? { network: networkConfig } : undefined;
      const response = (await HttpUtil.post(`/panel/api/gateway/${action}`, data, {
        silentSuccess: true,
      })) as ApiMsg<GatewayStatus>;

      if (response?.obj) applyStatus(response.obj);
      if (!response?.success) {
        messageApi.error(response?.msg || 'Не удалось изменить режим шлюза');
        return;
      }

      messageApi.success(action === 'enable' ? 'Режим шлюза включён' : 'Режим шлюза выключен');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : 'Не удалось изменить режим шлюза');
    } finally {
      await refresh(true);
      setBusy(false);
    }
  };

  const enabled = status?.enabled === true;
  const configured = status?.configured === true;
  const coreMismatch = status?.coreMismatch === true;
  const conflict = status?.conflict === true;
  const recoveryOnly = enabled && !configured && !coreMismatch && status?.recoveryBackup === true;
  const incompleteCore = enabled && !configured && !coreMismatch;
  const canEnable = status?.canEnable === true;
  const selectedCore = coreLabel(status?.coreType);
  const ownerCore = coreLabel(status?.gatewayCoreType);
  const networkConfigured = status?.network?.configured === true;
  const networkActive = status?.network?.active === true;
  const networkPersistent = status?.network?.persistent === true;
  const networkHealthy =
    networkConfigured &&
    status?.network?.forwarding === true &&
    status?.network?.policyRoute === true &&
    status?.network?.nftables === true &&
    networkPersistent;
  const staleNetworkOnly = !enabled && (networkConfigured || networkActive);
  const orphanedNetwork = networkActive && !networkConfigured;
  const networkInputValid =
    networkConfig.lanInterface.trim() !== '' &&
    networkConfig.lanIP.trim() !== '' &&
    networkConfig.lanPrefix >= 1 &&
    networkConfig.lanPrefix <= 32;
  const canConfigureNetwork = networkConfigured || networkInputValid;

  return (
    <>
      {contextHolder}
      <Space direction="vertical" size={14} style={{ width: '100%' }}>
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            Режим шлюза
          </Typography.Title>
          <Typography.Text type="secondary">
            Прозрачная маршрутизация TCP/UDP через TPROXY для выбранного ядра.
          </Typography.Text>
        </div>

        <Alert
          type={
            conflict
              ? 'error'
              : incompleteCore || staleNetworkOnly || coreMismatch || (enabled && !networkHealthy)
                ? 'warning'
                : enabled
                  ? 'success'
                  : 'info'
          }
          showIcon
          title={
            conflict
              ? 'Обнаружен конфликт режима шлюза'
              : incompleteCore
                ? 'Неполное состояние Gateway в конфигурации ядра'
                : staleNetworkOnly
                  ? 'Linux-настройки Gateway остались без активного inbound'
                  : coreMismatch
                    ? `Режим шлюза активен на другом ядре: ${ownerCore}`
                    : enabled && !networkConfigured
                      ? 'Inbound создан, Linux-маршрутизация не настроена'
                      : enabled && !networkHealthy
                        ? 'Сетевая часть режима шлюза требует восстановления'
                        : enabled
                          ? 'Режим шлюза включён'
                          : 'Режим шлюза выключен'
          }
          description={
            conflict
              ? 'Служебная конфигурация режима шлюза обнаружена одновременно в Xray и sing-box. Сначала выключите режим шлюза, чтобы безопасно очистить конфликтующие объекты.'
              : incompleteCore
                ? recoveryOnly
                  ? 'Найдена резервная копия Gateway, но полноценный inbound отсутствует. Linux TPROXY включать нельзя: сначала нажмите «Выключить режим шлюза», затем включите его заново.'
                  : 'Найдена частичная конфигурация Gateway. Чтобы не перенаправлять трафик на неполный inbound, сначала очистите состояние кнопкой «Выключить режим шлюза», затем включите его заново.'
                : staleNetworkOnly
                  ? orphanedNetwork
                    ? 'Найдены оставшиеся policy/nftables-объекты без корректного gateway.env. Их нужно очистить перед повторным включением.'
                    : 'Сетевые правила Gateway сохранены, но Gateway inbound в ядре отсутствует. Можно восстановить режим или удалить оставшиеся Linux-настройки.'
                  : coreMismatch
                    ? `Сейчас выбрано ядро ${selectedCore}. Режим шлюза можно перенести на него; Linux-настройки будут проверены и восстановлены при необходимости.`
                    : enabled && !networkConfigured
                      ? 'Укажите LAN-параметры ниже и нажмите «Настроить Linux». Второй inbound создан не будет.'
                      : enabled && !networkHealthy
                        ? 'Сохранённая конфигурация найдена, но отсутствует часть runtime-правил или файлов автозапуска. Нажмите «Восстановить сетевые правила».'
                        : enabled
                          ? `Режим шлюза настроен для ${ownerCore === 'не определено' ? selectedCore : ownerCore}. Linux TPROXY и policy routing управляются панелью.`
                          : `Режим шлюза сейчас не активен. При включении он будет настроен для ${selectedCore} вместе с Linux-маршрутизацией.`
          }
        />

        <Space wrap>
          <Typography.Text strong>Выбранное ядро:</Typography.Text>
          <Tag color={status?.coreType === 'sing-box' ? 'gold' : 'blue'}>{selectedCore}</Tag>
          {enabled && (
            <Tag color={coreMismatch || conflict ? 'warning' : 'success'}>Шлюз: {ownerCore}</Tag>
          )}
          <Tag>TPROXY: {status?.port ?? 52345}</Tag>
          <Tag color={networkHealthy ? 'success' : networkConfigured || networkActive ? 'warning' : 'default'}>
            Linux: {networkHealthy ? 'готов' : networkConfigured || networkActive ? 'частично' : 'не настроен'}
          </Tag>
        </Space>

        <Typography.Text strong>Сеть Gateway</Typography.Text>
        <Space wrap align="start">
          <div>
            <Typography.Text type="secondary">LAN-интерфейс</Typography.Text>
            <Input
              value={networkConfig.lanInterface}
              placeholder="eth1 / br-lan"
              disabled={networkConfigured || orphanedNetwork || busy}
              onChange={(event) =>
                setNetworkConfig((current) => ({ ...current, lanInterface: event.target.value }))
              }
              style={{ width: 150, display: 'block' }}
            />
          </div>
          <div>
            <Typography.Text type="secondary">LAN IP сервера</Typography.Text>
            <Input
              value={networkConfig.lanIP}
              placeholder="192.168.1.1"
              disabled={networkConfigured || orphanedNetwork || busy}
              onChange={(event) =>
                setNetworkConfig((current) => ({ ...current, lanIP: event.target.value }))
              }
              style={{ width: 170, display: 'block' }}
            />
          </div>
          <div>
            <Typography.Text type="secondary">Префикс</Typography.Text>
            <InputNumber
              min={1}
              max={32}
              value={networkConfig.lanPrefix}
              disabled={networkConfigured || orphanedNetwork || busy}
              onChange={onNumber((value) =>
                setNetworkConfig((current) => ({ ...current, lanPrefix: value })),
              )}
              style={{ width: 90, display: 'block' }}
            />
          </div>
          <div>
            <Typography.Text type="secondary">WAN-интерфейс (необязательно)</Typography.Text>
            <Input
              value={networkConfig.wanInterface}
              placeholder="eth0"
              disabled={networkConfigured || orphanedNetwork || busy}
              onChange={(event) =>
                setNetworkConfig((current) => ({ ...current, wanInterface: event.target.value }))
              }
              style={{ width: 190, display: 'block' }}
            />
          </div>
        </Space>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          LAN IP должен быть назначен выбранному LAN-интерфейсу. Перехват ограничивается именно этим
          интерфейсом, поэтому bridge-интерфейсы вроде br-lan поддерживаются. WAN-интерфейс нужен
          только если сервер должен выполнять NAT/masquerade в интернет.
        </Typography.Paragraph>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Сейчас Gateway перехватывает IPv4 TCP/UDP. IPv6 намеренно не отправляется в TPROXY, пока для
          него не настроены отдельные IPv6 policy routing и адреса: это исключает поломку IPv6 из-за
          неполной конфигурации.
        </Typography.Paragraph>

        <Space wrap>
          {enabled && configured && !coreMismatch && !conflict && !networkHealthy && !orphanedNetwork && (
            <Button
              type="primary"
              loading={busy}
              disabled={loading || !canConfigureNetwork}
              onClick={() => void runAction('enable')}
            >
              {networkConfigured ? 'Восстановить сетевые правила' : 'Настроить Linux'}
            </Button>
          )}

          {coreMismatch && !conflict && canEnable && !orphanedNetwork && (
            <Popconfirm
              title={`Перенести режим шлюза на ${selectedCore}?`}
              description="Панель удалит собственную конфигурацию режима шлюза из предыдущего ядра и включит её для выбранного ядра. Сетевые правила будут проверены и восстановлены."
              okText="Перенести"
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
              disabled={!canConfigureNetwork}
            >
              <Button type="primary" loading={busy} disabled={loading || !canConfigureNetwork}>
                Перенести на {selectedCore}
              </Button>
            </Popconfirm>
          )}

          {staleNetworkOnly && !orphanedNetwork && canEnable && (
            <Popconfirm
              title="Восстановить режим шлюза?"
              description={`Gateway inbound будет заново создан для ${selectedCore} с сохранёнными Linux-настройками.`}
              okText="Восстановить"
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
            >
              <Button type="primary" loading={busy} disabled={loading}>
                Восстановить Gateway
              </Button>
            </Popconfirm>
          )}

          {enabled || staleNetworkOnly ? (
            <Popconfirm
              title={staleNetworkOnly && !enabled ? 'Удалить оставшиеся Linux-настройки Gateway?' : 'Выключить режим шлюза?'}
              description={
                staleNetworkOnly && !enabled
                  ? 'Будут удалены nftables, policy routing, systemd restore-файлы и gateway.env.'
                  : recoveryOnly
                    ? 'Будет удалено устаревшее состояние восстановления и Linux-правила Gateway.'
                    : 'Будут удалены TPROXY-объекты, nftables и policy routing Gateway; исходное значение IPv4 forwarding будет восстановлено.'
              }
              okText="Выключить"
              cancelText="Отмена"
              onConfirm={() => void runAction('disable')}
            >
              <Button danger loading={busy}>
                {staleNetworkOnly && !enabled ? 'Очистить Linux-настройки' : 'Выключить режим шлюза'}
              </Button>
            </Popconfirm>
          ) : (
            <Popconfirm
              title="Включить режим шлюза?"
              description={`Панель настроит TPROXY для ${selectedCore}, IPv4 forwarding, policy routing и nftables.`}
              okText="Включить"
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
              disabled={!canEnable || loading || !networkInputValid}
            >
              <Button type="primary" loading={busy} disabled={!canEnable || loading || !networkInputValid}>
                Включить режим шлюза
              </Button>
            </Popconfirm>
          )}
          <Button icon={<ReloadOutlined />} disabled={busy} onClick={() => void refresh()}>
            Обновить
          </Button>
        </Space>
      </Space>
    </>
  );
}
