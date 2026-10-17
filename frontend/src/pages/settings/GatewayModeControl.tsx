import { useCallback, useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Collapse,
  Col,
  Input,
  InputNumber,
  Popconfirm,
  Row,
  Select,
  Space,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { onNumber } from '@/utils/onNumber';
import './GatewayModeControl.css';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };

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
  nat: boolean;
  persistent: boolean;
  listenerReady: boolean;
  rpFilter: boolean;
  policyRoute: boolean;
  nftables: boolean;
  error?: string;
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

type GatewayInterface = {
  name: string;
  defaultRoute: boolean;
  addresses: { ip: string; prefix: number; private: boolean }[];
};

type GatewayDiscovery = {
  interfaces: GatewayInterface[];
  defaultWAN?: string;
  suggested?: GatewayNetworkConfig;
  note: string;
};

type GatewayPreflight = {
  ready: boolean;
  problems: string[];
  notes: string[];
  verifiedConfig?: string;
};

const emptyNetworkConfig: GatewayNetworkConfig = {
  lanInterface: '',
  lanIP: '',
  lanPrefix: 24,
  wanInterface: '',
};

function coreLabel(core?: string) {
  if (core === 'hiddify-core' || core === 'sing-box') return 'sing-box';
  if (core === 'xray') return 'Xray';
  if (core === 'multiple') return 'Xray + sing-box';
  return 'не определено';
}

export default function GatewayModeControl() {
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [networkConfig, setNetworkConfig] = useState<GatewayNetworkConfig>(emptyNetworkConfig);
  const [discovery, setDiscovery] = useState<GatewayDiscovery | null>(null);
  const [preflight, setPreflight] = useState<GatewayPreflight | null>(null);
  const [loading, setLoading] = useState(true);
  const [discovering, setDiscovering] = useState(false);
  const [checking, setChecking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [allowDefaultInterface, setAllowDefaultInterface] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const applyStatus = useCallback((next: GatewayStatus) => {
    setStatus(next);
    if (next.network?.configured && next.network.config) {
      setNetworkConfig(next.network.config);
    }
  }, []);

  const loadStatus = useCallback(
    async (quiet = false): Promise<GatewayStatus | null> => {
      try {
        const response = (await HttpUtil.get('/panel/api/gateway/status', undefined, {
          silent: true,
        })) as ApiMsg<GatewayStatus>;
        if (!response?.success || !response.obj) {
          if (!quiet) messageApi.error(response?.msg || 'Не удалось получить состояние шлюза');
          return response?.obj ?? null;
        }
        return response.obj;
      } catch (error) {
        if (!quiet) messageApi.error(error instanceof Error ? error.message : 'Ошибка подключения');
        return null;
      }
    },
    [messageApi],
  );

  const refresh = useCallback(
    async (quiet = false) => {
      const next = await loadStatus(quiet);
      if (next) applyStatus(next);
      setLoading(false);
    },
    [loadStatus, applyStatus],
  );

  const discover = useCallback(
    async (quiet = false) => {
      setDiscovering(true);
      try {
        const response = (await HttpUtil.get('/panel/api/gateway/interfaces', undefined, {
          silent: true,
        })) as ApiMsg<GatewayDiscovery>;
        if (!response?.success || !response.obj) {
          if (!quiet) messageApi.error(response?.msg || 'Не удалось обнаружить интерфейсы');
          return;
        }
        setDiscovery(response.obj);
        if (response.obj.suggested) {
          const suggested = response.obj.suggested;
          setNetworkConfig((current) => (current.lanInterface ? current : { ...suggested }));
        }
      } catch (error) {
        if (!quiet)
          messageApi.error(error instanceof Error ? error.message : 'Ошибка обнаружения сети');
      } finally {
        setDiscovering(false);
      }
    },
    [messageApi],
  );

  useEffect(() => {
    let active = true;
    void loadStatus()
      .then((next) => {
        if (active && next) applyStatus(next);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    // Discover asynchronously to avoid synchronous state updates inside an effect.
    void Promise.resolve().then(() => {
      if (active) return discover(true);
    });
    return () => {
      active = false;
    };
  }, [loadStatus, applyStatus, discover]);

  useEffect(() => {
    const onSaved = () => void refresh(true);
    window.addEventListener('xui-core-settings-saved', onSaved);
    return () => window.removeEventListener('xui-core-settings-saved', onSaved);
  }, [refresh]);

  // Gateway may be restored by systemd or affected by a core restart. A quiet
  // status refresh updates diagnostics without changing unsaved LAN choices.
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible' && !busy && !checking) {
        void refresh(true);
      }
    }, 30000);
    return () => window.clearInterval(timer);
  }, [refresh, busy, checking]);

  const updateNetwork = (patch: Partial<GatewayNetworkConfig>) => {
    setNetworkConfig((current) => ({ ...current, ...patch }));
    setPreflight(null);
  };

  const runPreflight = useCallback(
    async (quiet = false): Promise<boolean> => {
      setChecking(true);
      setPreflight(null);
      try {
        const response = (await HttpUtil.post(
          '/panel/api/gateway/preflight',
          { network: networkConfig },
          { silent: true },
        )) as ApiMsg<GatewayPreflight>;
        if (!response?.success || !response.obj) {
          if (!quiet) messageApi.error(response?.msg || 'Не удалось проверить готовность');
          return false;
        }
        setPreflight({ ...response.obj, verifiedConfig: JSON.stringify(networkConfig) });
        return response.obj.ready;
      } catch (error) {
        if (!quiet) {
          messageApi.error(error instanceof Error ? error.message : 'Не удалось проверить шлюз');
        }
        return false;
      } finally {
        setChecking(false);
      }
    },
    [networkConfig, messageApi],
  );

  const runAction = async (action: 'enable' | 'disable') => {
    if (busy) return;
    setBusy(true);
    try {
      if (action === 'enable') {
        const ready = await runPreflight();
        if (!ready) return;
      }
      const response = (await HttpUtil.post(
        '/panel/api/gateway/' + action,
        action === 'enable' ? { network: networkConfig } : undefined,
        { silentSuccess: true },
      )) as ApiMsg<GatewayStatus>;
      if (response?.obj) applyStatus(response.obj);
      if (!response?.success) {
        messageApi.error(response?.msg || 'Не удалось изменить режим шлюза');
        return;
      }
      setPreflight(null);
      messageApi.success(action === 'enable' ? 'Шлюз настроен' : 'Шлюз выключен');
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
  const coreRunning = status?.coreRunning === true;
  const networkHealthy =
    networkConfigured &&
    status?.network?.forwarding === true &&
    status?.network?.rpFilter === true &&
    status?.network?.policyRoute === true &&
    status?.network?.nftables === true &&
    status?.network?.nat === true &&
    status?.network?.persistent === true &&
    status?.network?.listenerReady === true;

  const lanInterfaceIsWAN =
    networkConfig.lanInterface !== '' && networkConfig.lanInterface === discovery?.defaultWAN;
  const networkInputValid =
    networkConfig.lanInterface.trim() !== '' &&
    networkConfig.lanIP.trim() !== '' &&
    networkConfig.lanPrefix >= 1 &&
    networkConfig.lanPrefix <= 32 &&
    networkConfig.wanInterface !== networkConfig.lanInterface &&
    (!lanInterfaceIsWAN || allowDefaultInterface || networkConfigured);

  const availableInterfaces = discovery?.interfaces ?? [];
  const options = availableInterfaces.map((item) => ({
    value: item.name,
    label: item.name + (item.defaultRoute ? ' · основной маршрут' : ''),
  }));
  const selectedLAN = availableInterfaces.find((item) => item.name === networkConfig.lanInterface);
  const issues = [
    { label: 'Ядро запущено', healthy: coreRunning },
    { label: 'TCP/UDP listener', healthy: status?.network?.listenerReady === true },
    { label: 'IPv4 forwarding', healthy: status?.network?.forwarding === true },
    { label: 'rp_filter', healthy: status?.network?.rpFilter === true },
    { label: 'Policy routing', healthy: status?.network?.policyRoute === true },
    { label: 'nftables', healthy: status?.network?.nftables === true },
    { label: 'NAT (если нужен)', healthy: status?.network?.nat === true },
    { label: 'Автовосстановление', healthy: status?.network?.persistent === true },
  ];
  const canSubmit = !loading && !busy && !checking && canEnable && coreRunning && networkInputValid;
  const locked = networkConfigured || busy;
  // Automatically check a stable LAN/WAN choice, but never activate traffic
  // interception without explicit confirmation. Stale check results are ignored.
  useEffect(() => {
    if (loading || !coreRunning || !networkInputValid || networkConfigured || busy) return;
    const timer = window.setTimeout(() => {
      void runPreflight(true);
    }, 700);
    return () => {
      window.clearTimeout(timer);
    };
  }, [loading, coreRunning, networkInputValid, networkConfigured, busy, runPreflight]);

  const gatewayActionLabel =
    coreMismatch && networkHealthy
      ? 'Перенести на ' + selectedCore
      : networkConfigured && !networkHealthy
        ? 'Восстановить шлюз'
        : enabled && !networkConfigured
          ? 'Настроить Linux'
          : 'Включить шлюз';

  return (
    <div className="gateway-control">
      {contextHolder}
      <div className="singbox-section-header gateway-heading">
        <div>
          <div className="singbox-section-title">Режим шлюза</div>
          <div className="singbox-section-description">
            TPROXY для IPv4 TCP/UDP. Панель управляет правилами Linux и восстановлением после
            перезагрузки.
          </div>
        </div>
        <Space size={6} wrap>
          <Tag>{selectedCore}</Tag>
          <Tag color={networkHealthy ? 'success' : enabled ? 'warning' : 'default'}>
            {networkHealthy ? 'Работает' : enabled ? 'Требует внимания' : 'Выключен'}
          </Tag>
          <Tag>TPROXY {status?.port ?? 52345}</Tag>
        </Space>
      </div>

      {conflict || recoveryOnly || coreMismatch || (enabled && !networkHealthy) || !coreRunning ? (
        <Alert
          showIcon
          type={conflict || recoveryOnly ? 'error' : 'warning'}
          title={
            conflict
              ? 'Конфликт между ядрами'
              : recoveryOnly
                ? 'Неполное состояние шлюза'
                : coreMismatch
                  ? 'Шлюз принадлежит другому ядру: ' + ownerCore
                  : !coreRunning
                    ? 'Ядро остановлено'
                    : 'Настройки требуют восстановления'
          }
          description={
            conflict || recoveryOnly
              ? 'Сначала выключите режим шлюза для безопасной очистки его служебных объектов.'
              : coreMismatch
                ? 'После проверки можно перенести шлюз на выбранное ядро.'
                : !coreRunning
                  ? 'Запустите ядро перед настройкой сетевого перехвата.'
                  : 'Проверьте состояние служб и восстановите правила одним действием.'
          }
        />
      ) : null}
      {status?.network?.error && (
        <Alert type="error" showIcon title="Ошибка сети" description={status.network.error} />
      )}

      <Card
        className="singbox-item-card gateway-card"
        title="1. Сетевые параметры"
        extra={
          <Button
            icon={<SearchOutlined />}
            onClick={() => void discover()}
            loading={discovering}
            disabled={locked}
          >
            Найти интерфейсы
          </Button>
        }
      >
        <Typography.Paragraph type="secondary">
          Введите адрес Linux-сервера на стороне локальной сети. Если найден единственный подходящий
          LAN, поля заполняются автоматически. Ничего в сети не меняется до подтверждения включения.
        </Typography.Paragraph>
        {discovery?.note && (
          <Alert
            type={discovery.suggested ? 'info' : 'warning'}
            showIcon
            title={discovery.suggested ? 'Безопасный вариант обнаружен' : 'Нужен ручной выбор'}
            description={discovery.note}
            className="gateway-discovery-note"
          />
        )}
        <Row gutter={[12, 12]}>
          <Col xs={24} sm={12}>
            <label className="singbox-field-label" htmlFor="gateway-lan-interface">
              LAN-интерфейс
            </label>
            {options.length ? (
              <Select
                id="gateway-lan-interface"
                className="gateway-full-width"
                showSearch
                disabled={locked}
                value={networkConfig.lanInterface || undefined}
                placeholder="Выберите интерфейс"
                options={options}
                onChange={(name: string) => {
                  const iface = availableInterfaces.find((entry) => entry.name === name);
                  const privateIPs = iface?.addresses.filter((addr) => addr.private) || [];
                  const address = privateIPs.length === 1 ? privateIPs[0] : null;
                  setAllowDefaultInterface(false);
                  updateNetwork({
                    lanInterface: name,
                    lanIP: address?.ip || '',
                    lanPrefix: address?.prefix || 24,
                    wanInterface:
                      name === networkConfig.wanInterface ? '' : networkConfig.wanInterface,
                  });
                }}
              />
            ) : (
              <Input
                id="gateway-lan-interface"
                disabled={locked}
                value={networkConfig.lanInterface}
                placeholder="br-lan"
                onChange={(event) => updateNetwork({ lanInterface: event.target.value })}
              />
            )}
          </Col>
          <Col xs={24} sm={12}>
            <label className="singbox-field-label" htmlFor="gateway-lan-ip">
              IPv4 адрес LAN
            </label>
            <Input
              id="gateway-lan-ip"
              disabled={locked}
              placeholder="192.168.50.1"
              value={networkConfig.lanIP}
              onChange={(event) => updateNetwork({ lanIP: event.target.value })}
            />
          </Col>
          <Col xs={24} sm={12}>
            <label className="singbox-field-label" htmlFor="gateway-lan-prefix">
              Маска сети (CIDR)
            </label>
            <InputNumber
              id="gateway-lan-prefix"
              min={1}
              max={32}
              disabled={locked}
              className="gateway-full-width"
              value={networkConfig.lanPrefix}
              onChange={onNumber((value) => updateNetwork({ lanPrefix: value }))}
            />
          </Col>
          <Col xs={24} sm={12}>
            <label className="singbox-field-label" htmlFor="gateway-wan-interface">
              WAN / NAT (необязательно)
            </label>
            {options.length ? (
              <Select
                id="gateway-wan-interface"
                className="gateway-full-width"
                allowClear
                showSearch
                disabled={locked}
                placeholder="Без NAT"
                options={options.filter((item) => item.value !== networkConfig.lanInterface)}
                value={networkConfig.wanInterface || undefined}
                onChange={(name?: string) => updateNetwork({ wanInterface: name || '' })}
              />
            ) : (
              <Input
                id="gateway-wan-interface"
                disabled={locked}
                placeholder="eth0"
                value={networkConfig.wanInterface}
                onChange={(event) => updateNetwork({ wanInterface: event.target.value })}
              />
            )}
          </Col>
        </Row>
        {selectedLAN?.addresses.length ? (
          <div className="gateway-hint">
            Адреса интерфейса:{' '}
            {selectedLAN.addresses.map((address) => address.ip + '/' + address.prefix).join(', ')}
          </div>
        ) : null}
        {lanInterfaceIsWAN && !networkConfigured && (
          <Alert
            type="warning"
            showIcon
            title="Основной сетевой интерфейс выбран как LAN"
            description={
              <Space orientation="vertical" size={6}>
                <span>
                  Это нетипичная схема: ошибка в перехвате может нарушить работу сервера.
                  Автоматически такое подключение не предлагается.
                </span>
                <Checkbox
                  checked={allowDefaultInterface}
                  disabled={locked}
                  onChange={(event) => {
                    setAllowDefaultInterface(event.target.checked);
                    setPreflight(null);
                  }}
                >
                  Понимаю риск и хочу использовать этот интерфейс как LAN
                </Checkbox>
              </Space>
            }
          />
        )}
        <div className="gateway-inline-actions">
          {discovery?.suggested &&
            !networkConfigured &&
            JSON.stringify(discovery.suggested) !== JSON.stringify(networkConfig) && (
              <Button
                icon={<SearchOutlined />}
                disabled={locked}
                onClick={() => updateNetwork({ ...discovery.suggested! })}
              >
                Заполнить автоматически
              </Button>
            )}
          <Button
            icon={<CheckCircleOutlined />}
            loading={checking}
            disabled={!networkInputValid || busy}
            onClick={() => void runPreflight()}
          >
            {checking ? 'Проверка…' : 'Проверить ещё раз'}
          </Button>
        </div>
        {preflight && preflight.verifiedConfig === JSON.stringify(networkConfig) && (
          <Alert
            showIcon
            type={preflight.ready ? 'success' : 'error'}
            title={preflight.ready ? 'Проверка пройдена' : 'Перед включением устраните проблемы'}
            description={
              <ul className="gateway-preflight-list">
                {(preflight.ready ? preflight.notes : preflight.problems).map((item, i) => (
                  <li key={i}>{item}</li>
                ))}
              </ul>
            }
          />
        )}
      </Card>

      <Card
        className="singbox-item-card gateway-card"
        title="2. Состояние и восстановление"
        extra={
          <Button icon={<ReloadOutlined />} disabled={busy} onClick={() => void refresh()}>
            Обновить
          </Button>
        }
      >
        <Space wrap className="gateway-status-summary">
          <Tag color={coreRunning ? 'success' : 'error'}>
            Ядро: {coreRunning ? 'работает' : 'остановлено'}
          </Tag>
          <Tag color={networkHealthy ? 'success' : networkConfigured ? 'warning' : 'default'}>
            Linux:{' '}
            {networkHealthy
              ? 'готов'
              : networkConfigured
                ? 'требует восстановления'
                : 'не настроен'}
          </Tag>
          <Tag color={status?.network?.persistent ? 'success' : 'default'}>
            Автовосстановление: {status?.network?.persistent ? 'активно' : 'неактивно'}
          </Tag>
        </Space>
        <Collapse
          className="gateway-details"
          items={[
            {
              key: 'diagnostics',
              label: 'Подробная диагностика',
              children: (
                <div className="gateway-check-grid">
                  {issues.map((item) => (
                    <div className="gateway-check" key={item.label}>
                      <Tag color={item.healthy ? 'success' : 'default'}>
                        {item.healthy ? 'OK' : '—'}
                      </Tag>
                      <span>{item.label}</span>
                    </div>
                  ))}
                </div>
              ),
            },
          ]}
        />
        <Typography.Paragraph type="secondary" className="gateway-hint">
          Перехват ограничен выбранной IPv4 LAN-подсетью. Клиенты должны использовать этот сервер
          как свой шлюз. Исходящий маршрут задаётся обычными правилами выбранного ядра. Изменение
          LAN/WAN при включённом шлюзе требует предварительного выключения.
        </Typography.Paragraph>
        <div className="gateway-inline-actions">
          {enabled ? (
            <>
              {(!networkHealthy || coreMismatch) && !conflict && !recoveryOnly && (
                <Popconfirm
                  title={
                    coreMismatch ? 'Перенести шлюз на ' + selectedCore + '?' : 'Восстановить шлюз?'
                  }
                  description="Сначала будет проверена готовность, затем панель применит изменения с откатом при ошибке."
                  okText="Продолжить"
                  cancelText="Отмена"
                  onConfirm={() => void runAction('enable')}
                >
                  <Button
                    type="primary"
                    loading={busy}
                    disabled={!canSubmit}
                    icon={<ToolOutlined />}
                  >
                    {gatewayActionLabel}
                  </Button>
                </Popconfirm>
              )}
              <Popconfirm
                title="Выключить режим шлюза?"
                description="Будут удалены только управляемые панелью TPROXY-правила, восстановлено исходное forwarding и отключено автоприменение."
                okText="Выключить"
                cancelText="Отмена"
                onConfirm={() => void runAction('disable')}
              >
                <Button danger loading={busy}>
                  Выключить шлюз
                </Button>
              </Popconfirm>
            </>
          ) : (
            <Popconfirm
              title="Включить режим шлюза?"
              description={
                'Будут включены TPROXY и правила Linux для ' +
                selectedCore +
                '. Проверка выполнится перед изменением системы.'
              }
              okText="Включить"
              cancelText="Отмена"
              disabled={!canSubmit}
              onConfirm={() => void runAction('enable')}
            >
              <Button type="primary" loading={busy} disabled={!canSubmit}>
                Включить шлюз
              </Button>
            </Popconfirm>
          )}
        </div>
      </Card>
    </div>
  );
}
