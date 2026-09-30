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
  canEnable: boolean;
  coreType: string;
  xrayRunning: boolean;
};

const HEADER_TARGET = '.settings-page .header-actions';

export default function GatewayModeControl() {
  const [portalTarget, setPortalTarget] = useState<Element | null>(null);
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  // SettingsPage renders its header only after settings are loaded. Observe DOM
  // changes until the header appears instead of polling every animation frame.
  useEffect(() => {
    const findTarget = () => document.querySelector(HEADER_TARGET);
    const initialTarget = findTarget();
    if (initialTarget) {
      setPortalTarget(initialTarget);
      return;
    }

    const observer = new MutationObserver(() => {
      const target = findTarget();
      if (!target) return;
      setPortalTarget(target);
      observer.disconnect();
    });
    observer.observe(document.body, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, []);

  const refresh = useCallback(
    async (quiet = false) => {
      try {
        const response = (await HttpUtil.get('/panel/api/gateway/status', undefined, {
          silent: true,
        })) as ApiMsg<GatewayStatus>;
        if (!response?.success || !response.obj) {
          if (!quiet) {
            messageApi.error(response?.msg || 'Не удалось получить состояние Gateway Mode');
          }
          return;
        }
        setStatus(response.obj);
      } catch (error) {
        if (!quiet) {
          messageApi.error(
            error instanceof Error ? error.message : 'Не удалось получить состояние Gateway Mode',
          );
        }
      } finally {
        setLoading(false);
      }
    },
    [messageApi],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const onSettingsSaved = () => void refresh(true);
    window.addEventListener('xui-core-settings-saved', onSettingsSaved);
    return () => window.removeEventListener('xui-core-settings-saved', onSettingsSaved);
  }, [refresh]);

  useEffect(() => {
    if (open) void refresh(true);
  }, [open, refresh]);

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
  const singBoxSelected = status?.coreType === 'sing-box';
  const canEnable = status?.canEnable === true;

  const trigger = portalTarget
    ? createPortal(
        <span style={{ display: 'inline-flex', marginInlineStart: 8 }}>
          <Tooltip title="Управление прозрачным шлюзом Xray">
            <Button icon={<ApartmentOutlined />} loading={loading} onClick={() => setOpen(true)}>
              Gateway: {enabled ? 'вкл.' : 'выкл.'}
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
            type={enabled ? 'success' : 'info'}
            showIcon
            title={enabled ? 'Gateway Mode включён' : 'Gateway Mode выключен'}
            description={
              enabled
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
          </Space>

          {singBoxSelected && !enabled && (
            <Alert
              type="warning"
              showIcon
              title="Для включения выберите Xray"
              description="Gateway Mode меняет конфигурацию Xray и недоступен для включения, пока выбрано ядро sing-box."
            />
          )}

          {singBoxSelected && enabled && (
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
            Для работы сервера как шлюза также должны быть настроены маршрутизация Linux/TPROXY, а
            клиентские устройства должны отправлять трафик через этот сервер.
          </Typography.Paragraph>

          <Space wrap>
            {enabled ? (
              <Popconfirm
                title="Выключить Gateway Mode?"
                description="Будут удалены только TPROXY-вход, Gateway-outbound и связанное правило маршрутизации. Остальные изменения Xray сохранятся."
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
