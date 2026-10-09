import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Input,
  InputNumber,
  Popconfirm,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
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
  config: GatewayNetworkConfig;
  forwarding: boolean;
  nat?: boolean;
  persistent?: boolean;
  listenerReady?: boolean;
  error?: string;
  rpFilter?: boolean;
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
  coreRunning?: boolean;
  network?: GatewayNetworkStatus;
};

const emptyNetworkConfig: GatewayNetworkConfig = {
  lanInterface: '',
  lanIP: '',
  lanPrefix: 24,
  wanInterface: '',
};

function coreLabel(core?: string) {
  if (core === 'hiddify-core') return 'hiddify-core';
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
          return response?.obj ?? null;
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
  const recoveryOnly = enabled && !configured && !coreMismatch;
  const canEnable = status?.canEnable === true;
  const selectedCore = coreLabel(status?.coreType);
  const ownerCore = coreLabel(status?.gatewayCoreType);
  const networkConfigured = status?.network?.configured === true;
  const networkHealthy =
    networkConfigured &&
    status?.network?.forwarding === true &&
    status?.network?.rpFilter !== false &&
    status?.network?.policyRoute === true &&
    status?.network?.nftables === true &&
    status?.network?.nat !== false &&
    status?.network?.persistent !== false &&
    status?.network?.listenerReady !== false;
  const coreRunning = status?.coreRunning !== false;
  const networkInputValid =
    networkConfig.lanInterface.trim() !== '' &&
    networkConfig.lanIP.trim() !== '' &&
    networkConfig.lanPrefix >= 1 &&
    networkConfig.lanPrefix <= 32;

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
              : enabled && !networkHealthy
                ? 'warning'
                : recoveryOnly || coreMismatch
                  ? 'warning'
                  : enabled
                    ? 'success'
                    : 'info'
          }
          showIcon
          title={
            conflict
              ? 'Обнаружен конфликт режима шлюза'
              : recoveryOnly
                ? 'Требуется очистка состояния режима шлюза'
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
              ? 'Служебная конфигурация найдена в обоих ядрах. Выключите Gateway для очистки его объектов.'
              : recoveryOnly
                ? 'Найдены сетевые настройки или резервная копия без полного inbound. Выключите Gateway перед повторным включением.'
                : coreMismatch
                  ? `Сейчас выбрано ядро ${selectedCore}. Gateway можно перенести на него кнопкой включения.`
                  : enabled && !networkConfigured
                    ? 'Укажите LAN-параметры и нажмите «Настроить Linux»: панель добавит forwarding, policy routing и nftables.'
                    : enabled && !networkHealthy
                      ? 'Сохранённая конфигурация найдена, но часть правил, служб или listener отсутствует. Восстановите сетевые правила после запуска ядра.'
                      : enabled
                        ? `Gateway настроен для ${ownerCore === 'не определено' ? selectedCore : ownerCore}. Linux-маршрутизацией управляет панель.`
                        : `При включении Gateway будет настроен для ${selectedCore} вместе с Linux-маршрутизацией.`
          }
        />

        {status?.network?.error && (
          <Alert
            type="error"
            showIcon
            title="Ошибка состояния Gateway"
            description={status.network.error}
          />
        )}
        {!coreRunning && (
          <Alert
            type="warning"
            showIcon
            title="Ядро остановлено"
            description="Запустите выбранное ядро, затем включите или восстановите Gateway."
          />
        )}
        {recoveryOnly && (
          <Alert
            type="warning"
            showIcon
            title="Требуется очистка Gateway"
            description="Сетевые настройки или резервная копия найдены без полного инбайнда. Выключите Gateway перед повторным включением."
          />
        )}
        <Space wrap>
          <Typography.Text strong>Выбранное ядро:</Typography.Text>
          <Tag color={status?.coreType === 'sing-box' ? 'gold' : 'blue'}>{selectedCore}</Tag>
          {enabled && (
            <Tag color={coreMismatch || conflict ? 'warning' : 'success'}>Шлюз: {ownerCore}</Tag>
          )}
          <Tag>TPROXY: {status?.port ?? 52345}</Tag>
          <Tag color={networkHealthy ? 'success' : networkConfigured ? 'warning' : 'default'}>
            Linux: {networkHealthy ? 'готов' : networkConfigured ? 'частично' : 'не настроен'}
          </Tag>
        </Space>

        <Typography.Text strong>Сеть Gateway</Typography.Text>
        <Space wrap align="start">
          <div>
            <Typography.Text type="secondary">LAN-интерфейс</Typography.Text>
            <Input
              value={networkConfig.lanInterface}
              placeholder="eth1"
              disabled={networkConfigured || busy}
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
              disabled={networkConfigured || busy}
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
              disabled={networkConfigured || busy}
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
              disabled={networkConfigured || busy}
              onChange={(event) =>
                setNetworkConfig((current) => ({ ...current, wanInterface: event.target.value }))
              }
              style={{ width: 190, display: 'block' }}
            />
          </div>
        </Space>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          LAN IP должен быть назначен выбранному интерфейсу. Перехватывается IPv4 TCP/UDP трафик
          клиентов этой подсети на LAN-интерфейсе; обращения к локальным и частным адресам обходят
          прокси. WAN-интерфейс нужен только если этот сервер должен выполнять NAT/masquerade в
          интернет. Для изменения уже сохранённых сетевых параметров сначала выключите режим шлюза.
        </Typography.Paragraph>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          При включении панель добавляет TPROXY-вход выбранному ядру, включает IPv4 forwarding,
          создаёт policy rule/table 100 и nftables-таблицы. Состояние и правила сохраняются для
          автоматического восстановления после перезагрузки.
        </Typography.Paragraph>

        <Space wrap>
          {enabled && !networkHealthy && !recoveryOnly && !conflict && (
            <Button
              type="primary"
              loading={busy}
              disabled={
                busy || loading || !coreRunning || (!networkConfigured && !networkInputValid)
              }
              onClick={() => void runAction('enable')}
            >
              {networkConfigured ? 'Восстановить сетевые правила' : 'Настроить Linux'}
            </Button>
          )}

          {coreMismatch && !conflict && canEnable && networkHealthy && (
            <Popconfirm
              title={`Перенести режим шлюза на ${selectedCore}?`}
              description="Панель удалит собственную конфигурацию режима шлюза из предыдущего ядра и включит её для выбранного ядра. Сетевые правила сохранятся. При ошибке будет выполнен откат."
              okText="Перенести"
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
            >
              <Button type="primary" loading={busy} disabled={loading}>
                Перенести на {selectedCore}
              </Button>
            </Popconfirm>
          )}

          {enabled ? (
            <Popconfirm
              title="Выключить режим шлюза?"
              description={
                recoveryOnly
                  ? 'Будет удалено устаревшее состояние восстановления и Linux-правила Gateway.'
                  : 'Будут удалены TPROXY-объекты, nftables и policy routing Gateway; исходное значение IPv4 forwarding будет восстановлено.'
              }
              okText="Выключить"
              cancelText="Отмена"
              onConfirm={() => void runAction('disable')}
            >
              <Button danger loading={busy}>
                Выключить режим шлюза
              </Button>
            </Popconfirm>
          ) : (
            <Popconfirm
              title="Включить режим шлюза?"
              description={`Панель настроит TPROXY для ${selectedCore}, IPv4 forwarding, policy routing и nftables.`}
              okText="Включить"
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
              disabled={!canEnable || loading || busy || !coreRunning || !networkInputValid}
            >
              <Button
                type="primary"
                loading={busy}
                disabled={!canEnable || loading || busy || !coreRunning || !networkInputValid}
              >
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
