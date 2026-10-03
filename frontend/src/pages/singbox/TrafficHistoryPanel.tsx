import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Input,
  Select,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };
type TrafficPoint = { t: number; up: number; down: number };
type HistoryPayload = { points: TrafficPoint[] };

function formatBytes(value = 0) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${size >= 10 || unit === 0 ? size.toFixed(0) : size.toFixed(1)} ${units[unit]}`;
}

function TrafficChart({ points }: { points: TrafficPoint[] }) {
  const width = 960;
  const height = 260;
  const padding = 28;
  const maxValue = Math.max(1, ...points.flatMap((point) => [point.up, point.down]));
  const x = (index: number) =>
    padding +
    (points.length <= 1 ? 0 : (index * (width - padding * 2)) / (points.length - 1));
  const y = (value: number) => height - padding - (value / maxValue) * (height - padding * 2);
  const path = (key: 'up' | 'down') =>
    points
      .map((point, index) => `${index === 0 ? 'M' : 'L'} ${x(index)} ${y(point[key])}`)
      .join(' ');

  if (points.length === 0) {
    return (
      <Alert
        type="info"
        showIcon
        message="No history yet. The first five-minute sample establishes the baseline."
      />
    );
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Traffic history chart"
        style={{ width: '100%', minWidth: 620, height: 280 }}
      >
        <path d={path('up')} fill="none" stroke="#1677ff" strokeWidth="3" />
        <path d={path('down')} fill="none" stroke="#52c41a" strokeWidth="3" />
      </svg>
      <Space wrap>
        <Tag color="blue">Upload</Tag>
        <Tag color="green">Download</Tag>
        <Typography.Text type="secondary">
          Peak {formatBytes(maxValue)} / bucket
        </Typography.Text>
      </Space>
    </div>
  );
}

export default function TrafficHistoryPanel() {
  const [messageApi, contextHolder] = message.useMessage();
  const [resource, setResource] = useState<'client' | 'inbound' | 'outbound'>('client');
  const [tag, setTag] = useState('');
  const [bucket, setBucket] = useState('1h');
  const [points, setPoints] = useState<TrafficPoint[]>([]);
  const [loading, setLoading] = useState(false);

  const rows = useMemo(
    () =>
      points
        .slice()
        .reverse()
        .map((point) => ({
          key: point.t,
          time: new Date(point.t * 1000).toLocaleString(),
          up: formatBytes(point.up),
          down: formatBytes(point.down),
        })),
    [points],
  );

  const load = async () => {
    if (!tag.trim()) {
      messageApi.warning('Enter a client email, inbound tag or outbound tag.');
      return;
    }
    setLoading(true);
    try {
      const response = (await HttpUtil.get(
        `/panel/api/server/trafficHistory/${resource}/${bucket}`,
        { tag: tag.trim(), limit: 360 },
        { silent: true },
      )) as ApiMsg<HistoryPayload>;
      if (!response.success || !response.obj) {
        throw new Error(response.msg || 'Failed to load traffic history');
      }
      setPoints(Array.isArray(response.obj.points) ? response.obj.points : []);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : String(error));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      {contextHolder}
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Space wrap>
          <Select
            value={resource}
            style={{ width: 150 }}
            options={['client', 'inbound', 'outbound'].map((value) => ({
              value,
              label: value,
            }))}
            onChange={setResource}
          />
          <Input
            value={tag}
            onChange={(event) => setTag(event.target.value)}
            onPressEnter={() => void load()}
            placeholder="resource tag / client email"
            style={{ width: 300 }}
          />
          <Select
            value={bucket}
            style={{ width: 120 }}
            options={['5m', '15m', '30m', '1h', '6h', '12h', '1d'].map((value) => ({
              value,
              label: value,
            }))}
            onChange={setBucket}
          />
          <Button
            type="primary"
            icon={<ReloadOutlined />}
            loading={loading}
            onClick={() => void load()}
          >
            Load
          </Button>
        </Space>
        <TrafficChart points={points} />
        <Table
          rowKey="key"
          size="small"
          dataSource={rows}
          pagination={{ pageSize: 12, showSizeChanger: false }}
          columns={[
            { title: 'Time', dataIndex: 'time' },
            { title: 'Upload', dataIndex: 'up', width: 140 },
            { title: 'Download', dataIndex: 'down', width: 140 },
          ]}
        />
      </Space>
    </Card>
  );
}
