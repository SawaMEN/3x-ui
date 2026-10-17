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
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
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
  const [manualOpen, setManualOpen] = useState(false);
  const [diagnosticsOpen, setDiagnosticsOpen] = useState(false);
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
      // A fresh automatic preflight is advisory; the enable endpoint always
      // revalidates the host under its operation lock before changing the system.
      if (action === 'enable' && !(
        preflight?.ready && preflight.verifiedConfig === JSON.stringify(networkConfig)
      )) {
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
  const verified = preflight?.verifiedConfig === JSON.stringify(networkConfig);
  const ready = verified && preflight?.ready === true;
  const canSubmit =
    !loading && !busy && !checking && canEnable && coreRunning && networkInputValid &&
    (networkConfigured || ready);
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

  const needsRepair = enabled && (!networkHealthy || coreMismatch);
  const gatewayActionLabel = coreMismatch && networkHealthy
    ? 'Перенести шлюз'
    : networkConfigured && !networkHealthy
      ? 'Восстановить шлюз'
      : enabled && !networkConfigured
        ? 'Настроить шлюз'
        : 'Включить шлюз';
  const showManual = manualOpen || (discovery !== null && !discovery.suggested && !networkConfigured);
  const displayLAN = networkConfig.lanInterface
    ? `${networkConfig.lanInterface} · ${networkConfig.lanIP || 'адрес не выбран'}/${networkConfig.lanPrefix}`
    : 'Не выбран';
  const displayWAN = networkConfig.wanInterface || 'Без отдельного WAN / NAT';
  const missingNetwork = !networkConfig.lanInterface || !networkConfig.lanIP;
  const actionBlocked = conflict || recoveryOnly;

  return (
    <div className="gateway-control">
      {contextHolder}
      <Card className="singbox-item-card gateway-card gateway-main-card" loading={loading}>
        <div className="singbox-section-header gateway-heading">
          <div>
            <div className="singbox-section-title">Режим шлюза</div>
            <div className="singbox-section-description">
              Пропускает трафик устройств вашей локальной сети через sing-box.
              Сетевые правила и восстановление после перезагрузки настраиваются автоматически.
            </div>
          </div>
          <Tag color={networkHealthy && enabled && !coreMismatch ? 'success' : enabled ? 'warning' : 'default'}>
            {networkHealthy && enabled && !coreMismatch ? 'Работает' : enabled ? 'Требует внимания' : 'Выключен'}
          </Tag>
        </div>

        {status === null && !loading ? (
          <Alert showIcon type="error" title="Состояние шлюза недоступно"
            description="Проверьте подключение к панели и повторите запрос." />
        ) : (
          <>
            {conflict || recoveryOnly || coreMismatch || !coreRunning || (enabled && !networkHealthy) ? (
              <Alert className="gateway-important-alert" showIcon
                type={conflict || recoveryOnly ? 'error' : 'warning'}
                title={
                  conflict ? 'Конфликт между ядрами' :
                  recoveryOnly ? 'Шлюз настроен не полностью' :
                  coreMismatch ? 'Шлюз работает через ' + ownerCore :
                  !coreRunning ? 'Ядро остановлено' : 'Необходимо восстановить шлюз'
                }
                description={
                  conflict || recoveryOnly
                    ? 'Для исправления сначала выключите шлюз. Существующие правила будут удалены безопасно.'
                    : coreMismatch
                      ? 'Можно перенести настройки на текущее ядро после проверки.'
                      : !coreRunning
                        ? 'Сначала запустите sing-box.'
                        : 'Попробуйте восстановить настройки одним действием.'
                }
              />
            ) : null}
            {status?.network?.error && (
              <Alert className="gateway-important-alert" type="error" showIcon
                title="Ошибка сети" description={status.network.error} />
            )}

            <div className="gateway-network-overview">
              <div className="gateway-overview-line">
                <span className="gateway-overview-label">Локальная сеть</span>
                <strong>{displayLAN}</strong>
              </div>
              <div className="gateway-overview-line">
                <span className="gateway-overview-label">Выход в интернет</span>
                <strong>{displayWAN}</strong>
              </div>
              <div className="gateway-overview-line">
                <span className="gateway-overview-label">Ядро</span>
                <strong>{selectedCore}</strong>
              </div>
            </div>

            {!enabled && discovery?.note && (
              <div className="gateway-auto-summary">
                {discovery.suggested && !missingNetwork
                  ? 'Сеть определена автоматически. Проверьте адрес и включите шлюз.'
                  : discovery.note}
              </div>
            )}
            {!enabled && !discovery && discovering && (
              <div className="gateway-auto-summary">Определяем локальную сеть…</div>
            )}
            {!enabled && networkInputValid && verified && !checking && (
              <div className="gateway-readiness">
                {ready
                  ? <><CheckCircleOutlined /> Готов к включению</>
                  : <span>Проверка обнаружила ограничения. Подробности — ниже.</span>}
              </div>
            )}
            {!enabled && networkInputValid && checking && (
              <div className="gateway-auto-summary">Проверяем готовность сети…</div>
            )}

            <div className="gateway-primary-actions">
              {enabled ? (
                <>
                  {needsRepair && !actionBlocked && (
                    <Popconfirm
                      title={coreMismatch ? 'Перенести шлюз на ' + selectedCore + '?' : 'Восстановить шлюз?'}
                      description="Панель повторно проверит сеть и откатит настройки при ошибке."
                      okText="Продолжить" cancelText="Отмена"
                      onConfirm={() => void runAction('enable')}
                    >
                      <Button type="primary" size="large" loading={busy} disabled={!canSubmit}>
                        {gatewayActionLabel}
                      </Button>
                    </Popconfirm>
                  )}
                  <Popconfirm title="Выключить режим шлюза?"
                    description="Будут удалены управляемые панелью TPROXY-правила. Настройки sing-box и пользователи сохранятся."
                    okText="Выключить" cancelText="Отмена"
                    onConfirm={() => void runAction('disable')}
                  >
                    <Button danger size="large" loading={busy} disabled={busy}>
                      Выключить шлюз
                    </Button>
                  </Popconfirm>
                </>
              ) : (
                <Popconfirm title="Включить режим шлюза?"
                  description="Панель настроит перехват трафика выбранной LAN-сети на этом сервере. Если параметры неверны, доступ по сети может быть нарушен."
                  okText="Включить" cancelText="Отмена" disabled={!canSubmit}
                  onConfirm={() => void runAction('enable')}
                >
                  <Button type="primary" size="large" loading={busy} disabled={!canSubmit}>
                    Включить шлюз
                  </Button>
                </Popconfirm>
              )}
              <Button type="text" icon={<ReloadOutlined />} loading={loading}
                disabled={busy} onClick={() => void refresh()}>
                Обновить
              </Button>
            </div>
          </>
        )}
      </Card>

      <Collapse
        className="singbox-advanced-groups gateway-settings-groups"
        activeKey={[...(showManual ? ['network'] : []), ...(diagnosticsOpen ? ['diagnostics'] : [])]}
        onChange={(keys) => {
          const opened = Array.isArray(keys) ? keys : [keys];
          setManualOpen(opened.includes('network'));
          setDiagnosticsOpen(opened.includes('diagnostics'));
        }}
        items={[
          {
            key: 'network',
            label: 'Параметры сети',
            extra: <Tag color={discovery?.suggested ? 'success' : 'default'}>
              {networkConfigured ? 'Применены' : discovery?.suggested ? 'Авто' : 'Ручной выбор'}
            </Tag>,
            children: (
              <Space orientation="vertical" size={14} className="gateway-fields">
                <div className="singbox-section-description">
                  Настройки определяются автоматически, когда найден единственный безопасный LAN-интерфейс.
                  Изменить их при работающем шлюзе можно после выключения.
                </div>
                <Row gutter={[12, 12]}>
                  <Col xs={24} sm={12}>
                    <label className="singbox-field-label" htmlFor="gateway-lan-interface">LAN-интерфейс</label>
                    {options.length ? (
                      <Select id="gateway-lan-interface" aria-label="LAN-интерфейс"
                        className="gateway-full-width" showSearch disabled={locked}
                        value={networkConfig.lanInterface || undefined} placeholder="Выберите интерфейс"
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
                            wanInterface: name === networkConfig.wanInterface ? '' : networkConfig.wanInterface,
                          });
                        }}
                      />
                    ) : (
                      <Input id="gateway-lan-interface" disabled={locked}
                        value={networkConfig.lanInterface} placeholder="br-lan"
                        onChange={(event) => updateNetwork({ lanInterface: event.target.value })} />
                    )}
                  </Col>
                  <Col xs={24} sm={12}>
                    <label className="singbox-field-label" htmlFor="gateway-lan-ip">IPv4-адрес LAN</label>
                    <Input id="gateway-lan-ip" disabled={locked} placeholder="192.168.50.1"
                      value={networkConfig.lanIP}
                      onChange={(event) => updateNetwork({ lanIP: event.target.value })} />
                  </Col>
                  <Col xs={24} sm={12}>
                    <label className="singbox-field-label" htmlFor="gateway-lan-prefix">Префикс сети</label>
                    <InputNumber id="gateway-lan-prefix" min={1} max={32} disabled={locked}
                      className="gateway-full-width" value={networkConfig.lanPrefix}
                      onChange={onNumber((value) => updateNetwork({ lanPrefix: value }))} />
                  </Col>
                  <Col xs={24} sm={12}>
                    <label className="singbox-field-label" htmlFor="gateway-wan-interface">WAN / NAT (при необходимости)</label>
                    {options.length ? (
                      <Select id="gateway-wan-interface" aria-label="WAN / NAT"
                        className="gateway-full-width" allowClear showSearch disabled={locked}
                        placeholder="Без NAT" options={options.filter((o) => o.value !== networkConfig.lanInterface)}
                        value={networkConfig.wanInterface || undefined}
                        onChange={(name?: string) => updateNetwork({ wanInterface: name || '' })} />
                    ) : (
                      <Input id="gateway-wan-interface" disabled={locked} placeholder="eth0"
                        value={networkConfig.wanInterface}
                        onChange={(event) => updateNetwork({ wanInterface: event.target.value })} />
                    )}
                  </Col>
                </Row>
                {selectedLAN?.addresses.length ? (
                  <div className="gateway-hint">
                    Адреса интерфейса: {selectedLAN.addresses.map((a) => a.ip + '/' + a.prefix).join(', ')}
                  </div>
                ) : null}
                {lanInterfaceIsWAN && !networkConfigured && (
                  <Alert type="warning" showIcon title="LAN совпадает с основным WAN"
                    description={<Space orientation="vertical" size={8}>
                      <span>Перехват на основном интерфейсе может нарушить доступ к серверу. Этот вариант не выбирается автоматически.</span>
                      <Checkbox checked={allowDefaultInterface} disabled={locked}
                        onChange={(event) => {
                          setAllowDefaultInterface(event.target.checked);
                          setPreflight(null);
                        }}>
                        Я понимаю риск и хочу использовать WAN как LAN
                      </Checkbox>
                    </Space>} />
                )}
                <div className="gateway-secondary-actions">
                  <Button icon={<SearchOutlined />} loading={discovering} disabled={locked}
                    onClick={() => void discover()}>Найти интерфейсы</Button>
                  {discovery?.suggested && !networkConfigured && (
                    <Button disabled={locked} onClick={() => {
                      setAllowDefaultInterface(false);
                      updateNetwork({ ...discovery.suggested! });
                    }}>Использовать найденную сеть</Button>
                  )}
                  <Button icon={<CheckCircleOutlined />} loading={checking}
                    disabled={!networkInputValid || busy}
                    onClick={() => void runPreflight()}>Проверить</Button>
                </div>
                {preflight && verified && (!preflight.ready || diagnosticsOpen) && (
                  <Alert showIcon type={preflight.ready ? 'success' : 'warning'}
                    title={preflight.ready ? 'Готово к включению' : 'Проверьте параметры'}
                    description={<ul className="gateway-preflight-list">
                      {(preflight.ready ? preflight.notes : preflight.problems).map((item, i) =>
                        <li key={i}>{item}</li>)}
                    </ul>} />
                )}
              </Space>
            ),
          },
          {
            key: 'diagnostics',
            label: 'Диагностика',
            children: (
              <Space orientation="vertical" size={12} className="gateway-fields">
                <div className="gateway-check-grid">
                  {issues.map((item) => (
                    <div className="gateway-check" key={item.label}>
                      <Tag color={item.healthy ? 'success' : 'default'}>{item.healthy ? 'OK' : '—'}</Tag>
                      <span>{item.label}</span>
                    </div>
                  ))}
                </div>
                <div className="gateway-hint">TPROXY: {status?.port ?? 52345} · Трафик перехватывается
                  только из выбранной IPv4-подсети. Клиенты должны использовать этот сервер как шлюз.
                  Маршрутизация настраивается в разделе «Маршрутизация» sing-box.</div>
                {preflight && verified && (
                  <Alert showIcon type={preflight.ready ? 'success' : 'warning'}
                    title={preflight.ready ? 'Последняя проверка пройдена' : 'Проблемы при проверке'}
                    description={<ul className="gateway-preflight-list">
                      {(preflight.ready ? preflight.notes : preflight.problems).map((item, i) =>
                        <li key={i}>{item}</li>)}
                    </ul>} />
                )}
              </Space>
            ),
          },
        ]}
      />
    </div>
  );
}
