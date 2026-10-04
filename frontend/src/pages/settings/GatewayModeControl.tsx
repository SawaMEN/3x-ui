import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Input, InputNumber, Popconfirm, Space, Tag, Typography, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';

type ApiMsg<T = unknown> = {
  success?: boolean;
  msg?: string;
  obj?: T;
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
  systemConfigured?: boolean;
  systemActive?: boolean;
  ipForward?: boolean;
  policyRoute?: boolean;
  firewall?: boolean;
  nat?: boolean;
  lanInterface?: string;
  lanIP?: string;
  lanPrefix?: number;
  lanNetwork?: string;
  wanInterface?: string;
  systemError?: string;
};

function coreLabel(core?: string) {
  if (core === 'sing-box') return 'sing-box';
  if (core === 'xray') return 'Xray';
  if (core === 'multiple') return 'Xray + sing-box';
  return 'не определено';
}

export default function GatewayModeControl() {
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [lanInterface, setLanInterface] = useState('');
  const [lanIP, setLanIP] = useState('');
  const [lanPrefix, setLanPrefix] = useState(24);
  const [wanInterface, setWanInterface] = useState('');
  const [messageApi, contextHolder] = message.useMessage();

  const applyStatus = useCallback((nextStatus: GatewayStatus) => {
    setStatus(nextStatus);
    if (nextStatus.systemConfigured) {
      setLanInterface(nextStatus.lanInterface || '');
      setLanIP(nextStatus.lanIP || '');
      setLanPrefix(nextStatus.lanPrefix ?? 24);
      setWanInterface(nextStatus.wanInterface || '');
    }
  }, []);

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
      const data =
        action === 'enable'
          ? {
              lanInterface: lanInterface.trim(),
              lanIP: lanIP.trim(),
              lanPrefix,
              wanInterface: wanInterface.trim(),
            }
          : undefined;
      const response = (await HttpUtil.post(`/panel/api/gateway/${action}`, data, {
        silentSuccess: true,
      })) as ApiMsg<GatewayStatus>;

      if (response?.obj) applyStatus(response.obj);
      if (!response?.success) {
        messageApi.error(response?.msg || 'Не удалось изменить режим шлюза');
        return;
      }

      if (action === 'disable') {
        setLanInterface('');
        setLanIP('');
        setLanPrefix(24);
        setWanInterface('');
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
  const systemConfigured = status?.systemConfigured === true;
  const systemActive = status?.systemActive === true;
  const systemError = status?.systemError || '';
  const recoveryOnly = enabled && !configured && !coreMismatch && status?.recoveryBackup === true;
  const canEnable = status?.canEnable === true;
  const canDisable = enabled || systemConfigured;
  const selectedCore = coreLabel(status?.coreType);
  const ownerCore = coreLabel(status?.gatewayCoreType);
  const needsLinuxRepair = enabled && configured && !systemActive && !coreMismatch && !conflict;
  const healthy = enabled && configured && systemActive && !coreMismatch && !conflict;
  const networkReady = useMemo(
    () => lanInterface.trim().length > 0 && lanIP.trim().length > 0 && lanPrefix >= 0 && lanPrefix <= 32,
    [lanIP, lanInterface, lanPrefix],
  );
  const enableDisabled = loading || busy || !canEnable || !networkReady || recoveryOnly || conflict;

  let alertType: 'success' | 'info' | 'warning' | 'error' = 'info';
  let alertTitle = 'Режим шлюза выключен';
  let alertDescription = `Режим шлюза сейчас не активен. При включении он будет настроен для ${selectedCore}.`;
  if (conflict) {
    alertType = 'error';
    alertTitle = 'Обнаружен конфликт режима шлюза';
    alertDescription =
      'Служебная конфигурация режима шлюза обнаружена одновременно в Xray и sing-box. Выключение безопасно очистит собственные объекты режима шлюза в обоих ядрах.';
  } else if (recoveryOnly) {
    alertType = 'warning';
    alertTitle = 'Требуется очистка состояния режима шлюза';
    alertDescription =
      'Найдена резервная копия режима шлюза, но его служебная конфигурация отсутствует. Нажмите «Выключить», чтобы безопасно удалить устаревшее состояние восстановления.';
  } else if (coreMismatch) {
    alertType = 'warning';
    alertTitle = `Режим шлюза активен на другом ядре: ${ownerCore}`;
    alertDescription = `Сейчас выбрано ядро ${selectedCore}. Режим шлюза можно перенести на него с сохранением Linux TPROXY-настройки.`;
  } else if (systemError) {
    alertType = 'error';
    alertTitle = 'Ошибка проверки Linux Gateway';
    alertDescription = systemError;
  } else if (needsLinuxRepair) {
    alertType = 'warning';
    alertTitle = 'TPROXY-вход создан, но Linux Gateway не активен';
    alertDescription =
      'Не все системные настройки применены: IPv4 forwarding, policy routing или nftables отсутствуют. Нажмите «Применить / восстановить Linux rules».';
  } else if (!enabled && systemConfigured) {
    alertType = 'warning';
    alertTitle = 'Linux Gateway настроен, но вход ядра отсутствует';
    alertDescription =
      'Системные правила Gateway сохранены, но TPROXY-вход ядра не активен. Можно включить режим заново либо полностью выключить и очистить правила.';
  } else if (healthy) {
    alertType = 'success';
    alertTitle = 'Режим шлюза полностью активен';
    alertDescription = `TPROXY-вход ${ownerCore === 'не определено' ? selectedCore : ownerCore} и Linux networking применены.`;
  } else if (enabled) {
    alertType = 'warning';
    alertTitle = 'Режим шлюза включён частично';
    alertDescription = 'Проверьте состояние ядра и Linux networking ниже.';
  }

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

        <Alert type={alertType} showIcon title={alertTitle} description={alertDescription} />

        <Space wrap>
          <Typography.Text strong>Выбранное ядро:</Typography.Text>
          <Tag color={status?.coreType === 'sing-box' ? 'gold' : 'blue'}>{selectedCore}</Tag>
          {enabled && (
            <Tag color={coreMismatch || conflict ? 'warning' : configured ? 'success' : 'warning'}>
              Шлюз: {ownerCore}
            </Tag>
          )}
          <Tag>TPROXY: {status?.port ?? 52345}</Tag>
          <Tag color={status?.ipForward ? 'success' : 'default'}>ip_forward</Tag>
          <Tag color={status?.policyRoute ? 'success' : 'default'}>ip rule</Tag>
          <Tag color={status?.firewall ? 'success' : 'default'}>nft</Tag>
          {status?.wanInterface && <Tag color={status?.nat ? 'success' : 'warning'}>NAT</Tag>}
        </Space>

        <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
          Панель создаёт TPROXY-вход ядра и автоматически настраивает Linux: IPv4 forwarding,
          policy routing, nftables и восстановление правил после перезагрузки. Outbound и routing
          ядра не подменяются — перехваченный трафик проходит через ваши обычные правила.
        </Typography.Paragraph>

        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Typography.Text strong>Linux Gateway</Typography.Text>
          <Space wrap>
            <Input
              style={{ width: 180 }}
              placeholder="LAN interface, например eth0"
              value={lanInterface}
              disabled={busy || systemConfigured}
              onChange={(event) => setLanInterface(event.target.value)}
            />
            <Input
              style={{ width: 180 }}
              placeholder="LAN IPv4, например 192.168.1.1"
              value={lanIP}
              disabled={busy || systemConfigured}
              onChange={(event) => setLanIP(event.target.value)}
            />
            <InputNumber
              min={0}
              max={32}
              precision={0}
              addonBefore="/"
              value={lanPrefix}
              disabled={busy || systemConfigured}
              onChange={(value) => setLanPrefix(value ?? 24)}
            />
            <Input
              style={{ width: 220 }}
              placeholder="WAN interface (опционально)"
              value={wanInterface}
              disabled={busy || systemConfigured}
              onChange={(event) => setWanInterface(event.target.value)}
            />
          </Space>
          <Typography.Text type="secondary">
            LAN IPv4 должен быть назначен указанному интерфейсу. WAN нужен только если панели следует
            добавить masquerade/NAT. Чтобы изменить уже сохранённые интерфейсы, сначала выключите Gateway Mode.
          </Typography.Text>
          {systemConfigured && status?.lanNetwork && (
            <Typography.Text type="secondary">Сохранённая сеть: {status.lanNetwork}</Typography.Text>
          )}
        </Space>

        <Space wrap>
          {canEnable && (
            <Popconfirm
              title={
                coreMismatch
                  ? `Перенести режим шлюза на ${selectedCore}?`
                  : needsLinuxRepair
                    ? 'Восстановить Linux Gateway rules?'
                    : 'Включить режим шлюза?'
              }
              description={
                coreMismatch
                  ? 'Панель перенесёт TPROXY-вход на выбранное ядро и проверит системные Gateway rules.'
                  : needsLinuxRepair
                    ? 'Панель повторно применит IPv4 forwarding, policy routing и nftables, не создавая второй inbound.'
                    : `Панель создаст TPROXY-вход для ${selectedCore}, настроит Linux policy routing и nftables и сохранит их для reboot.`
              }
              okText={coreMismatch ? 'Перенести' : needsLinuxRepair ? 'Восстановить' : 'Включить'}
              cancelText="Отмена"
              onConfirm={() => void runAction('enable')}
              disabled={enableDisabled}
            >
              <Button type="primary" loading={busy} disabled={enableDisabled}>
                {coreMismatch
                  ? `Перенести на ${selectedCore}`
                  : needsLinuxRepair
                    ? 'Применить / восстановить Linux rules'
                    : 'Включить режим шлюза'}
              </Button>
            </Popconfirm>
          )}

          {canDisable && (
            <Popconfirm
              title="Выключить режим шлюза?"
              description={
                recoveryOnly
                  ? 'Будет удалено устаревшее состояние восстановления и собственные системные Gateway rules.'
                  : 'Будут удалены TPROXY-объекты ядра, nftables, policy routing и сохранённые systemd Gateway units. Предыдущее значение IPv4 forwarding будет восстановлено.'
              }
              okText="Выключить"
              cancelText="Отмена"
              onConfirm={() => void runAction('disable')}
            >
              <Button danger loading={busy} disabled={loading}>
                Выключить режим шлюза
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
