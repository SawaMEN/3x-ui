import { Alert, Collapse, Descriptions, Modal, Space, Tag, Typography } from 'antd';

import type { ProxyPresetPreviewItem } from '@/schemas/api/proxyPreset';

interface Props {
  open: boolean;
  presetName: string;
  items: ProxyPresetPreviewItem[];
  confirming?: boolean;
  onCancel: () => void;
  onApply: () => void | Promise<void>;
}

function text(value: unknown): string {
  if (value == null || value === '') return '—';
  if (Array.isArray(value)) return value.length > 0 ? value.join(', ') : '—';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  return String(value);
}

export default function ProxyPresetPreviewModal({
  open,
  presetName,
  items,
  confirming,
  onCancel,
  onApply,
}: Props) {
  const warningCount = items.reduce((total, item) => total + item.warnings.length, 0);
  const changedCount = items.reduce((total, item) => total + item.changedFields.length, 0);

  return (
    <Modal
      open={open}
      width={860}
      title={`Preview: ${presetName}`}
      okText="Apply preset"
      cancelText="Cancel"
      confirmLoading={confirming}
      onCancel={onCancel}
      onOk={onApply}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Typography.Text type="secondary">
          {items.length} Host group(s), {changedCount} changed field(s), {warningCount} warning(s). No database changes have been made yet.
        </Typography.Text>
        <Collapse
          items={items.map((item) => ({
            key: item.groupId,
            label: (
              <Space wrap>
                <Typography.Text strong>{item.remark || item.groupId}</Typography.Text>
                <Tag>{item.changedFields.length} changes</Tag>
                {item.warnings.length > 0 ? <Tag color="warning">{item.warnings.length} warnings</Tag> : null}
              </Space>
            ),
            children: (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                {item.changedFields.length > 0 ? (
                  <Space wrap>
                    {item.changedFields.map((field) => (
                      <Tag key={field} color="blue">
                        {field}
                      </Tag>
                    ))}
                  </Space>
                ) : (
                  <Alert type="info" showIcon message="This preset does not change the stored Host values." />
                )}
                {item.warnings.map((warning) => (
                  <Alert key={warning} type="warning" showIcon message={warning} />
                ))}
                <Descriptions size="small" bordered column={2}>
                  <Descriptions.Item label="Addresses">
                    {text(item.effective.hosts)}
                  </Descriptions.Item>
                  <Descriptions.Item label="Port">{text(item.effective.port)}</Descriptions.Item>
                  <Descriptions.Item label="Security">
                    {text(item.effective.security)}
                  </Descriptions.Item>
                  <Descriptions.Item label="SNI">{text(item.effective.sni)}</Descriptions.Item>
                  <Descriptions.Item label="Host header">
                    {text(item.effective.hostHeader)}
                  </Descriptions.Item>
                  <Descriptions.Item label="Path">{text(item.effective.path)}</Descriptions.Item>
                  <Descriptions.Item label="ALPN">{text(item.effective.alpn)}</Descriptions.Item>
                  <Descriptions.Item label="Fingerprint">
                    {text(item.effective.fingerprint)}
                  </Descriptions.Item>
                  <Descriptions.Item label="ECH">
                    {text(item.effective.echConfigList)}
                  </Descriptions.Item>
                  <Descriptions.Item label="Excluded formats">
                    {text(item.effective.excludeFromSubTypes)}
                  </Descriptions.Item>
                </Descriptions>
              </Space>
            ),
          }))}
        />
      </Space>
    </Modal>
  );
}
