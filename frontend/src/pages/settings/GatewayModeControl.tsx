import { useCallback, useEffect, useLayoutEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import {
  Alert,
  Button,
  Modal,
  Popconfirm,
  Space,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
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

export default function GatewayModeControl() {
  const [portalTarget, setPortalTarget] = useState<Element | null>(null);
  const [open, setOpen] = useState(false);
  const [status, setStatus] = useState<GatewayStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  useLayoutEffect(() => {
    let frame = 0;
    const attach = () => {
      const target = document.querySelector('.settings-page .header-actions');
      setPortalTarget(target);
      if (!target) frame = window.requestAnimationFrame(attach);
    };
    frame = window.requestAnimationFrame(attach);
    return () => {
      if (frame) window.cancelAnimationFrame(frame);
    };
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
    const frame = window.requestAnimationFrame(() => void refresh());
    return () => window.cancelAnimationFrame(frame);
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
  const singBoxSelected = status?.coreType === 'sing-box';
  const canEnable = status?.canEnable === true;

  const trigger = portalTarget
    ? createPortal(
        <span style={{ display: 'inline-flex', marginInlineStart: 8 }}>
          <Tooltip title="Управление прозрачным шлюзом Xray">
            <Button
              icon={<ApartmentOutlined />}
              loading={loading}
              onClick={() => setOpen(true)}
            >
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
                ? 'Xray использует отдельный TPROXY-вход и системный Gateway-outbound.'
                : 'Режим прозрачного шлюза сейчас не добавлен в конфигурацию Xray.'
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
              description="Gateway Mode в текущей реализации работает с конфигурацией Xray и недоступен для sing-box."
            />
          )}

          {singBoxSelected && enabled && (
            <Alert
              type="warning"
              showIcon
              title="Gateway Mode остался включён в Xray"
              description="Сейчас выбрано ядро sing-box. Gateway можно выключить, чтобы восстановить сохранённую конфигурацию Xray."
            />
          )}

          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            При включении исходный шаблон Xray сохраняется в резервную копию. При выключении он
            восстанавливается. Если Xray уже запущен, панель перезапустит его для применения
            изменения.
          </Typography.Paragraph>

          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            Для работы сервера как шлюза также должны быть настроены маршрутизация Linux/TPROXY,
            а клиентские устройства должны отправлять трафик через этот сервер.
          </Typography.Paragraph>

          <Space wrap>
            {enabled ? (
              <Popconfirm
                title="Выключить Gateway Mode?"
                description="Будет восстановлен Xray-конфиг, сохранённый перед включением Gateway Mode."
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
                description="Панель изменит шаблон Xray и сохранит его текущую версию для последующего восстановления."
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
