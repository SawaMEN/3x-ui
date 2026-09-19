import { useEffect, useMemo, useState } from 'react';
import { Card, Empty, Table, Tag, Typography } from 'antd';
import { GlobalOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';

interface TelemtConnection {
  ip: string;
  users: string[];
  city: string;
  region: string;
  country: string;
  countryCode: string;
  latitude: number;
  longitude: number;
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
        const msg = await HttpUtil.get<TelemtConnection[]>('/panel/api/telemt/connections', undefined, {
          silent: true,
        });
        if (!disposed && msg?.success && Array.isArray(msg.obj)) setRows(msg.obj);
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

  const columns = useMemo(
    () => [
      {
        title: 'IP',
        dataIndex: 'ip',
        key: 'ip',
        render: (ip: string) => (
          <Typography.Text code copyable={{ text: ip }}>
            {ip}
          </Typography.Text>
        ),
      },
      {
        title: 'Пользователь',
        key: 'users',
        render: (_: unknown, row: TelemtConnection) =>
          row.users?.length ? row.users.join(', ') : '—',
      },
      {
        title: 'Город',
        key: 'city',
        render: (_: unknown, row: TelemtConnection) => (
          <span>
            {row.city}
            {row.region ? <Typography.Text type="secondary"> · {row.region}</Typography.Text> : null}
          </span>
        ),
      },
      {
        title: 'Страна',
        key: 'country',
        render: (_: unknown, row: TelemtConnection) => (
          <span>
            {row.countryCode ? <Tag bordered={false}>{row.countryCode}</Tag> : null}
            {row.country}
          </span>
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
            Telemt · подключённые IP
          </span>
          <span className="ov-telemt-count">{rows.length}</span>
        </div>
      }
      loading={loading && rows.length === 0}
    >
      {rows.length === 0 && !loading ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Нет активных подключений" />
      ) : (
        <Table<TelemtConnection>
          rowKey="ip"
          size="small"
          pagination={false}
          virtual={rows.length > 30}
          scroll={{ y: 360, x: 720 }}
          dataSource={rows}
          columns={columns}
        />
      )}
    </Card>
  );
}
