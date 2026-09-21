import { useEffect, useMemo, useState } from 'react';
import { Card, Empty, Space, Table, Tag, Typography } from 'antd';
import { GlobalOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';

interface TelemtIPLocation {
  ip: string;
  city: string;
  region: string;
  country: string;
  countryCode: string;
  latitude: number;
  longitude: number;
}

interface TelemtConnection {
  username: string;
  currentConnections: number;
  activeIPs: TelemtIPLocation[];
  recentIPCount: number;
  firstSeenAt: number;
  lastSeenAt: number;
  totalBytes: number;
  active: boolean;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 Б';
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ', 'ПБ'];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** exponent;
  return value >= 10 || exponent === 0
    ? `${value.toFixed(0)} ${units[exponent]}`
    : `${value.toFixed(1)} ${units[exponent]}`;
}

function formatRelativeTime(timestamp: number): string {
  if (!timestamp) return '—';

  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return 'меньше минуты назад';

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} мин назад`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} ч назад`;

  const days = Math.floor(hours / 24);
  return `${days} дн назад`;
}

function formatLocation(location: TelemtIPLocation): string {
  return [location.city, location.region, location.country].filter(Boolean).join(' · ');
}

export default function TelemtConnectionsCard() {
  const [rows, setRows] = useState<TelemtConnection[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const load = async () => {
      setLoading(true);
      try {
        const msg = await HttpUtil.get<TelemtConnection[]>(
          '/panel/api/telemt/connections',
          undefined,
          { silent: true },
        );

        if (!disposed && msg?.success && Array.isArray(msg.obj)) {
          setRows(
            msg.obj.map((row) => ({
              ...row,
              activeIPs: Array.isArray(row.activeIPs) ? row.activeIPs : [],
            })),
          );
        }
      } finally {
        if (!disposed) {
          setLoading(false);
          timer = setTimeout(load, 10000);
        }
      }
    };

    void load();

    return () => {
      disposed = true;
      if (timer) clearTimeout(timer);
    };
  }, []);

  const activeCount = useMemo(() => rows.filter((row) => row.active).length, [rows]);

  const columns = useMemo(
    () => [
      {
        title: 'Пользователь',
        dataIndex: 'username',
        key: 'username',
        render: (username: string, row: TelemtConnection) => (
          <Space direction="vertical" size={0}>
            <Typography.Text strong>{username}</Typography.Text>
            <Typography.Text type="secondary">
              {row.recentIPCount > 0
                ? `${row.recentIPCount} IP за недавнее окно`
                : 'IP в недавнем окне нет'}
            </Typography.Text>
          </Space>
        ),
      },
      {
        title: 'Состояние',
        key: 'connections',
        render: (_: unknown, row: TelemtConnection) => (
          <Space direction="vertical" size={0}>
            {row.active ? (
              <Tag color="green">
                {row.currentConnections > 0 ? `${row.currentConnections} соединений` : 'Сейчас'}
              </Tag>
            ) : (
              <Tag>Неактивен</Tag>
            )}
            <Typography.Text type="secondary">{formatRelativeTime(row.lastSeenAt)}</Typography.Text>
          </Space>
        ),
      },
      {
        title: 'Активные IP',
        key: 'activeIPs',
        render: (_: unknown, row: TelemtConnection) => (
          <Space direction="vertical" size={2}>
            {row.activeIPs.length > 0 ? (
              row.activeIPs.map((location) => (
                <Typography.Text key={location.ip} code copyable={{ text: location.ip }}>
                  {location.ip}
                </Typography.Text>
              ))
            ) : (
              <Typography.Text type="secondary">Нет активных IP</Typography.Text>
            )}
          </Space>
        ),
      },
      {
        title: 'Геолокация',
        key: 'geo',
        render: (_: unknown, row: TelemtConnection) => (
          <Space direction="vertical" size={0}>
            {row.activeIPs.length > 0 ? (
              row.activeIPs.map((location) => (
                <span key={location.ip}>
                  {location.countryCode ? <Tag bordered={false}>{location.countryCode}</Tag> : null}
                  {formatLocation(location) || 'Неизвестно'}
                </span>
              ))
            ) : (
              <Typography.Text type="secondary">—</Typography.Text>
            )}
          </Space>
        ),
      },
      {
        title: 'Трафик',
        key: 'traffic',
        render: (_: unknown, row: TelemtConnection) => (
          <Typography.Text strong>{formatBytes(row.totalBytes)}</Typography.Text>
        ),
      },
    ],
    [],
  );

  return (
    <Card
      hoverable
      styles={{ body: { padding: 0 } }}
      title={
        <div className="ov-telemt-head">
          <span className="ov-kicker ov-kicker-icon">
            <GlobalOutlined />
            Telemt · пользователи
          </span>
          <span className="ov-telemt-count">
            {activeCount > 0 ? `${activeCount} активных / ${rows.length}` : rows.length}
          </span>
        </div>
      }
      loading={loading && rows.length === 0}
    >
      {rows.length === 0 && !loading ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="Пользователей ещё не зафиксировано"
        />
      ) : (
        <Table<TelemtConnection>
          rowKey="username"
          size="small"
          pagination={false}
          virtual={rows.length > 30}
          scroll={{ y: 420, x: 1080 }}
          dataSource={rows}
          columns={columns}
        />
      )}
    </Card>
  );
}
