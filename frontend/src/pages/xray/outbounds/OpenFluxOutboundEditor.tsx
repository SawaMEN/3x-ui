import { useState } from 'react';
import { Alert, Button, Form, Input, InputNumber, Select, Space, message } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import { OPENFLUX_TRANSPORTS, validOpenFluxChannelURL, type OpenFluxTransport } from '@/lib/openflux';

export default function OpenFluxOutboundEditor({
  settings, update,
}: { settings: Record<string, unknown>; update: (values: Record<string, unknown>) => void }) {
  const { t } = useTranslation();
  const [link, setLink] = useState('');
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();
  const transports = (Array.isArray(settings.transports) ? settings.transports : []) as OpenFluxTransport[];
  const used = new Set(transports.map((transport) => transport.type));
  const changeTransport = (index: number, patch: Partial<OpenFluxTransport>) =>
    update({ transports: transports.map((transport, i) => i === index ? { ...transport, ...patch } : transport) });
  const importLink = async () => {
    setBusy(true);
    try {
      const result = await HttpUtil.post<{ protocol?: string; settings?: Record<string, unknown> }>(
        '/panel/api/server/externalvpn/parse-link', { link: link.trim() },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result.success || result.obj?.protocol !== 'openflux' || !result.obj.settings) {
        messageApi.error(result.msg || t('pages.inbounds.form.openfluxImportFailed'));
        return;
      }
      const imported = result.obj.settings;
      update({ secret: imported.secret, context: imported.context ?? '', transports: imported.transports });
      setLink('');
      messageApi.success(t('pages.inbounds.form.openfluxImported'));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      {contextHolder}
      <Form.Item label={t('pages.inbounds.form.openfluxImport')}>
        <Input.Search
          aria-label={t('pages.inbounds.form.openfluxImport')}
          value={link}
          onChange={(event) => setLink(event.target.value)}
          placeholder="openflux://v1/…"
          autoComplete="off"
          loading={busy}
          enterButton={t('pages.inbounds.form.openfluxImportAction')}
          onSearch={() => { if (link.trim()) void importLink(); }}
        />
      </Form.Item>
      <Form.Item label={t('pages.inbounds.form.openfluxSecret')}>
        <Input.Password
          value={String(settings.secret ?? '')}
          onChange={(event) => update({ secret: event.target.value })}
          autoComplete="new-password"
        />
      </Form.Item>
      <Form.Item label={t('pages.inbounds.form.openfluxContext')} extra={t('pages.inbounds.form.openfluxContextHint')}>
        <Input value={String(settings.context ?? '')} maxLength={4096} onChange={(event) => update({ context: event.target.value })} />
      </Form.Item>
      <Alert type="info" showIcon description={t('pages.inbounds.form.openfluxOutboundHint')} />
      {transports.map((transport, index) => (
        <Space key={index} wrap align="start">
          <Form.Item label={t('pages.inbounds.form.openfluxTransport')}>
            <Select
              style={{ width: 150 }}
              value={transport.type}
              options={OPENFLUX_TRANSPORTS.map((type) => ({ value: type, label: type, disabled: used.has(type) && transport.type !== type }))}
              onChange={(type) => changeTransport(index, { type })}
            />
          </Form.Item>
          <Form.Item label={t('pages.inbounds.form.openfluxPriority')}>
            <InputNumber value={transport.priority ?? 50} min={0} max={1000} onChange={(priority) => changeTransport(index, { priority: priority ?? 50 })} />
          </Form.Item>
          {transport.type === 'direct' ? (
            <Form.Item label={t('pages.inbounds.form.openfluxEndpoint')}>
              <Input value={transport.dial ?? ''} placeholder="vpn.example.org:443" onChange={(event) => changeTransport(index, { dial: event.target.value.trim() })} />
            </Form.Item>
          ) : (
            <Form.Item
              label="URL"
              validateStatus={validOpenFluxChannelURL(transport.url ?? '') ? undefined : 'error'}
              help={validOpenFluxChannelURL(transport.url ?? '') ? undefined : t('pages.inbounds.form.openfluxURLRequired')}
            >
              <Input value={transport.url ?? ''} placeholder="https://…" onChange={(event) => changeTransport(index, { url: event.target.value.trim() })} />
            </Form.Item>
          )}
          <Button disabled={transports.length === 1} onClick={() => update({ transports: transports.filter((_, i) => i !== index) })}>{t('remove')}</Button>
        </Space>
      ))}
      <Button
        disabled={used.size >= OPENFLUX_TRANSPORTS.length}
        onClick={() => {
          const type = OPENFLUX_TRANSPORTS.find((candidate) => !used.has(candidate));
          if (type) update({ transports: [...transports, { type, url: '', priority: type === 'direct' ? 100 : 50 }] });
        }}
      >{t('pages.inbounds.form.openfluxAdd')}</Button>
    </>
  );
}
