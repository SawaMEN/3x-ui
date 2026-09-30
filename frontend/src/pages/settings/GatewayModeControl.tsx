import { useCallback, useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import { Alert, Button, Modal, Popconfirm, Space, Tag, Tooltip, Typography, message } from 'antd';
import { ApartmentOutlined, ReloadOutlined } from '@ant-design/icons';

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
  xrayRunning: boolean;
  port: number;
};

const HEADER_TARGET = '.settings-page .header-actions';

export default function GatewayModeControl() {
  const [portalTarget, setPortalTarget] = useState<Element | null>(() =>
    document.querySelector(HEADER_TARGET),
  );
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  // SettingsPage renders its header only after settings are loaded. If it did
  // not exist during our first render, wait for the DOM insertion once.
  useEffect(() => {
    if (portalTarget) return;

    const observer = new MutationObserver(() => {
      const target = document.querySelector(HEADER_TARGET);
      if (!target) return;
      setPortalTarget(target);
      observer.disconnect();
    });
    observer.observe(document.body, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [portalTarget]);

  const loadStatus = useCallback(
    async (quiet = false): Promise<GatewayStatus | null> => {
      try {
        const response = (await HttpUtil.get('/panel/api/gateway/status', undefined, {
          silent: true,
        })) as ApiMsg<GatewayStatus>;
        if (!response?.success || !response.obj) {
          if (!quiet) {
            messageApi.error(response?.msg || 'Не удалось получить состояние Gateway Mode');
          }
          return null;
        }
        return response.obj;
      } catch (error) {
        if (!quiet) {
          messageApi.error(
            error instanceof Error ? error.message : 'Не удалось получить состояние Gateway Mode',
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
      if (nextStatus) setStatus(nextStatus);
      setLoading(false);
    },
    [loadStatus],
  );

  useEffect(() => {
    let active = true;

    void loadStatus()
      .then((nextStatus) => {
        if (active && nextStatus) setStatus(nextStatus);
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [loadStatus]);

  useEffect(() => {
    const onSettingsSaved = () => void refresh(true);
    window.addEventListener('xui-core-settings-saved', onSettingsSaved);
    return () => window.removeEventListener('xui-core-settings-saved', onSettingsSaved);
  }, [refresh]);

  const runAction = async (action: 'enable' | 'disable') => {
    setBusy(true);
    try {
      const response = (await HttpUtil.post(`/panel/api/gateway/${action}`, undefined, {
        silentSuccess: true,
      })) as ApiMsg<GatewayStatus>;

      if (response?.obj) setStatus(response.obj);
      if (!response?.success) {
        messageApi.error(response?.msg || 'Не удалось изменить Gateway Mode');
        return;
      }

      messageApi.success(action === 'enable' ? 'Gateway Mode включён' : 'Gateway Mode выключен');
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : 'Не удалось изменить Gateway Mode');
    } finally {
      await refresh(true);
      setBusy(false);
    }
  };

  const enabled = status?.enabled === true;
  const configured = status?.configured === true;
  const recoveryOnly = enabled && !configured && status?.recoveryBackup === true;
  const singBoxSelected = status?.coreType === 'sing-box';
  const canEnable = status?.canEnable === true;

  const trigger = portalTarget
    ? createPortal(
        <span style={{ display: 'inline-flex', marginInlineStart: 8 }}>
          <Tooltip title="Управление прозрачным шлюзом Xray">
            <Button
              icon={<ApartmentOutlined />}
              loading={loading}
              onClick={() => {
                setOpen(true);
                void refresh(true);
              }}
            >
              Gateway: {recoveryOnly ? 'восстановление' : enabled ? 'вкл.' : 'выкл.'}
            </Button>
          </Tooltip>
        </span>,
        portalTarget,
      )
    : null;

  return (
    <>
      {contextHolder}
      {trigger}
      <Modal
        open={open}
        title="Gateway Mode"
        footer={null}
        destroyOnClose={false}
        onCancel={() => setOpen(false)}
      >
        <Space direction="vertical" size={14} style={{ width: '100%' }}>
          <Alert
            type={recoveryOnly ? 'warning' : enabled ? 'success' : 'info'}
            showIcon
            title={
              recoveryOnly
                ? 'Требуется очистка состояния Gateway Mode'
                : enabled
                  ? 'Gateway Mode включён'
                  : 'Gateway Mode выключен'
            }
            description={
              recoveryOnly
                ? 'Найдена резервная копия Gateway Mode, но его объекты отсутствуют в текущем Xray-конфиге. Нажмите «Выключить», чтобы безопасно удалить устаревший recovery-marker.'
                : enabled
                  ? 'В шаблоне Xray активны отдельный TPROXY-вход и Gateway-outbound.'
                  : 'Gateway-объекты сейчас не активны в конфигурации Xray.'
            }
          />

          <Space wrap>
            <Typography.Text strong>Ядро:</Typography.Text>
            <Tag color={singBoxSelected ? 'gold' : 'blue'}>
              {singBoxSelected ? 'sing-box' : 'Xray'}
            </Tag>
            <Tag color={status?.xrayRunning ? 'success' : 'default'}>
              Xray {status?.xrayRunning ? 'работает' : 'остановлен'}
            </Tag>
            <Tag>TPROXY: {status?.port ?? 52345}</Tag>
          </Space>

          {singBoxSelected && !enabled && (
            <Alert
              type="warning"
              showIcon
              title="Для включения выберите Xray"
              description="Gateway Mode меняет конфигурацию Xray и недоступен для включения, пока выбрано ядро sing-box."
            />
          )}

          {singBoxSelected && enabled && !recoveryOnly && (
            <Alert
              type="warning"
              showIcon
              title="Gateway Mode активен в Xray"
              description="Сейчас выбрано ядро sing-box. Gateway Mode можно безопасно выключить: будут удалены только его собственные элементы конфигурации."
            />
          )}

          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            При включении панель сохраняет резервную копию текущего шаблона Xray и добавляет только
            Gateway-объекты. При обычном выключении удаляются только эти объекты, поэтому изменения
            Xray, сделанные после включения Gateway Mode, не откатываются.
          </Typography.Paragraph>

          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Linux должен перенаправлять TCP/UDP через TPROXY на порт {status?.port ?? 52345} и иметь
            включённый IPv4 forwarding. Клиентские устройства должны использовать этот сервер как
            шлюз.
          </Typography.Paragraph>

          <Space wrap>
            {enabled ? (
              <Popconfirm
                title="Выключить Gateway Mode?"
                description={
                  recoveryOnly
                    ? 'Будет удалён только устаревший recovery-marker. Текущий Xray-конфиг не будет откатан.'
                    : 'Будут удалены только TPROXY-вход, Gateway-outbound и связанное правило маршрутизации. Остальные изменения Xray сохранятся.'
                }
                okText="Выключить"
                cancelText="Отмена"
                onConfirm={() => void runAction('disable')}
              >
                <Button danger loading={busy}>
                  Выключить Gateway Mode
                </Button>
              </Popconfirm>
            ) : (
              <Popconfirm
                title="Включить Gateway Mode?"
                description="Панель сохранит резервную копию и добавит Gateway-объекты в текущий шаблон Xray."
                okText="Включить"
                cancelText="Отмена"
                onConfirm={() => void runAction('enable')}
                disabled={!canEnable || loading}
              >
                <Button type="primary" loading={busy} disabled={!canEnable || loading}>
                  Включить Gateway Mode
                </Button>
              </Popconfirm>
            )}
            <Button icon={<ReloadOutlined />} disabled={busy} onClick={() => void refresh()}>
              Обновить
            </Button>
          </Space>
        </Space>
      </Modal>
    </>
  );
}
