import { useState } from 'react';
import { Collapse, Form, Input, message } from 'antd';
import { useTranslation } from 'react-i18next';
import { HttpUtil } from '@/utils';
import type { OpenFluxTransport } from '@/lib/openflux';
import OpenFluxChannels from '@/components/openflux/OpenFluxChannels';

export default function OpenFluxOutboundEditor({
  settings,
  update,
}: {
  settings: Record<string, unknown>;
  update: (values: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const [link, setLink] = useState('');
  const [busy, setBusy] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();
  const transports = (
    Array.isArray(settings.transports) ? settings.transports : []
  ) as OpenFluxTransport[];
  const importLink = async () => {
    setBusy(true);
    try {
      const result = await HttpUtil.post<{ protocol?: string; settings?: Record<string, unknown> }>(
        '/panel/api/server/externalvpn/parse-link',
        { link: link.trim() },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result.success || result.obj?.protocol !== 'openflux' || !result.obj.settings) {
        messageApi.error(result.msg || t('pages.inbounds.form.openfluxImportFailed'));
        return;
      }
      const imported = result.obj.settings;
      update({
        secret: imported.secret,
        context: imported.context ?? '',
        transports: imported.transports,
      });
      setLink('');
      messageApi.success(t('pages.inbounds.form.openfluxImported'));
    } catch (error) {
      messageApi.error(
        error instanceof Error ? error.message : t('pages.inbounds.form.openfluxImportFailed'),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      {contextHolder}
      <Form.Item
        label={t('pages.inbounds.form.openfluxImport')}
        tooltip={t('pages.inbounds.form.openfluxImportHint')}
      >
        <Input.Search
          aria-label={t('pages.inbounds.form.openfluxImport')}
          value={link}
          onChange={(event) => setLink(event.target.value)}
          placeholder="openflux://v1/…"
          autoComplete="off"
          loading={busy}
          enterButton={t('pages.inbounds.form.openfluxImportAction')}
          onSearch={() => {
            if (link.trim()) void importLink();
          }}
        />
      </Form.Item>
      <Form.Item
        label={t('pages.inbounds.form.openfluxSecret')}
        tooltip={t('pages.inbounds.form.openfluxSecretHint')}
      >
        <Input.Password
          value={String(settings.secret ?? '')}
          onChange={(event) => update({ secret: event.target.value })}
          autoComplete="new-password"
        />
      </Form.Item>
      <Collapse
        ghost
        items={[
          {
            key: 'context',
            label: t('pages.inbounds.advancedTitle'),
            children: (
              <Form.Item
                label={t('pages.inbounds.form.openfluxContext')}
                tooltip={t('pages.inbounds.form.openfluxContextHint')}
              >
                <Input
                  value={String(settings.context ?? '')}
                  maxLength={4096}
                  onChange={(event) => update({ context: event.target.value })}
                />
              </Form.Item>
            ),
          },
        ]}
      />
      <OpenFluxChannels
        value={transports}
        client
        onChange={(next) => update({ transports: next })}
      />
    </>
  );
}
