import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Checkbox, Descriptions, Modal, Space, Typography, message } from 'antd';
import { DownloadOutlined, UploadOutlined } from '@ant-design/icons';

import { useProxyPresetMutations } from '@/api/queries/useProxyPresets';
import {
  ProxyBundleSchema,
  type ProxyBundle,
  type ProxyBundleImportResult,
} from '@/schemas/api/proxyPreset';

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export default function ProxyBundleControls() {
  const { t } = useTranslation();
  const [messageApi, contextHolder] = message.useMessage();
  const inputRef = useRef<HTMLInputElement>(null);
  const [bundle, setBundle] = useState<ProxyBundle | null>(null);
  const [preview, setPreview] = useState<ProxyBundleImportResult | null>(null);
  const [allowMissing, setAllowMissing] = useState(false);
  const { exportBundle, importBundle, importing } = useProxyPresetMutations();

  const onExport = async () => {
    try {
      const data = await exportBundle();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `3x-ui-proxy-bundle-${new Date().toISOString().slice(0, 10)}.json`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };

  const onFile = async (file: File) => {
    let parsed: unknown;
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      messageApi.error('Invalid JSON');
      return;
    }
    const validated = ProxyBundleSchema.safeParse(parsed);
    if (!validated.success) {
      messageApi.error(validated.error.issues.map((issue) => issue.message).join('; '));
      return;
    }
    try {
      const nextBundle = validated.data;
      const nextPreview = await importBundle(nextBundle, true, false);
      setBundle(nextBundle);
      setPreview(nextPreview);
      setAllowMissing(false);
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };

  const applyImport = async () => {
    if (!bundle) return;
    try {
      const result = await importBundle(bundle, false, allowMissing);
      setPreview(null);
      setBundle(null);
      messageApi.success(
        `${t('success')}: ${result.createdHosts + result.updatedHosts} Hosts, ${result.createdPresets + result.updatedPresets} presets`,
      );
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };

  const hasMissing = Boolean(preview?.missingInboundTags.length || preview?.missingPresetNames.length);
  const blockedByPreset = Boolean(preview?.missingPresetNames.length);

  return (
    <>
      {contextHolder}
      <Space size="small">
        <Button icon={<DownloadOutlined />} onClick={onExport}>
          Bundle
        </Button>
        <Button icon={<UploadOutlined />} onClick={() => inputRef.current?.click()}>
          Bundle
        </Button>
      </Space>
      <input
        ref={inputRef}
        type="file"
        accept="application/json,.json"
        hidden
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = '';
          if (file) void onFile(file);
        }}
      />
      <Modal
        open={preview !== null}
        title="Proxy bundle preview"
        okText={t('confirm')}
        cancelText={t('cancel')}
        confirmLoading={importing}
        okButtonProps={{
          disabled: blockedByPreset || (Boolean(preview?.missingInboundTags.length) && !allowMissing),
        }}
        onOk={applyImport}
        onCancel={() => {
          setPreview(null);
          setBundle(null);
        }}
      >
        {preview ? (
          <Space direction="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions size="small" column={2} bordered>
              <Descriptions.Item label="Presets +">{preview.createdPresets}</Descriptions.Item>
              <Descriptions.Item label="Presets ~">{preview.updatedPresets}</Descriptions.Item>
              <Descriptions.Item label="Hosts +">{preview.createdHosts}</Descriptions.Item>
              <Descriptions.Item label="Hosts ~">{preview.updatedHosts}</Descriptions.Item>
              <Descriptions.Item label="Assignments">{preview.assignedPresets}</Descriptions.Item>
              <Descriptions.Item label="Skipped">{preview.skippedHosts.length}</Descriptions.Item>
            </Descriptions>
            {hasMissing ? (
              <Alert
                type={blockedByPreset ? 'error' : 'warning'}
                showIcon
                message="Bundle dependencies are missing"
                description={
                  <Space direction="vertical" size={4}>
                    {preview.missingInboundTags.length > 0 ? (
                      <Typography.Text>
                        Inbound tags: {preview.missingInboundTags.join(', ')}
                      </Typography.Text>
                    ) : null}
                    {preview.missingPresetNames.length > 0 ? (
                      <Typography.Text>
                        Presets: {preview.missingPresetNames.join(', ')}
                      </Typography.Text>
                    ) : null}
                  </Space>
                }
              />
            ) : null}
            {preview.missingInboundTags.length > 0 && !blockedByPreset ? (
              <Checkbox checked={allowMissing} onChange={(event) => setAllowMissing(event.target.checked)}>
                Skip Host groups whose inbound tags do not exist on this panel
              </Checkbox>
            ) : null}
          </Space>
        ) : null}
      </Modal>
    </>
  );
}
