import { Button, Checkbox, Drawer, Form, Input, Select, Switch, Tabs, Typography } from 'antd';
import type { FormInstance } from 'antd';
import { useTranslation } from 'react-i18next';
import type { DomainConfigForm, DomainForm } from '@/schemas/domain-front';
import { DomainFormSchema } from '@/schemas/domain-front';
import { antdRule } from '@/utils/antdRule';
import { TelemtWebCatalogs } from '@/schemas/telemt-web-catalog';
import CoverSiteHTMLEditor, {
  coverSiteHTMLValidation,
  starterCoverSiteHTML,
} from '@/components/CoverSiteHTMLEditor';
export default function DomainEditor({
  open,
  editing,
  form,
  draft,
  compatible,
  primaryDomain,
  setPrimaryDomain,
  tab,
  onTab,
  onSave,
  onClose,
}: {
  open: boolean;
  editing?: number;
  form: FormInstance<DomainForm>;
  draft: DomainConfigForm;
  compatible: { id: number; remark?: string; network?: string; port?: number }[];
  primaryDomain: boolean;
  setPrimaryDomain: (value: boolean) => void;
  tab: string;
  onTab: (value: string) => void;
  onSave: () => void;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const label = (key: string) => t('pages.settings.domains.' + key);
  const mode = Form.useWatch('mode', form);
  const siteCatalog = Form.useWatch('siteCatalog', form);
  const tls = Form.useWatch('tls', form);
  return (
    <Drawer
      open={open}
      title={label('editor')}
      onClose={onClose}
      size={680}
      forceRender
      footer={
        <div className="domain-drawer-footer">
          <Button onClick={onClose}>{t('cancel')}</Button>
          <Button type="primary" onClick={onSave}>
            {label('workspace.saveDomain')}
          </Button>
        </div>
      }
    >
      <Form form={form} layout="vertical" preserve={false} className="domain-editor-form">
        <Tabs
          activeKey={tab}
          onChange={onTab}
          items={[
            {
              key: 'details',
              label: label('redesign.domainDetails'),
              forceRender: true,
              children: (
                <>
                  <Form.Item
                    name="name"
                    label={label('name')}
                    tooltip={label('nameHelp')}
                    rules={[
                      antdRule(DomainFormSchema.shape.name, t),
                      {
                        validator: async (_rule, value: string) => {
                          if (
                            draft.domains.some(
                              (d, i) => i !== editing && d.name === value?.trim().toLowerCase(),
                            )
                          )
                            throw new Error(label('duplicate'));
                        },
                      },
                    ]}
                  >
                    <Input placeholder="sub.example.com" />
                  </Form.Item>
                  <Form.Item
                    name="mode"
                    label={label('modeLabel')}
                    tooltip={label('modeHelp')}
                    rules={[antdRule(DomainFormSchema.shape.mode, t)]}
                  >
                    <Select
                      onChange={(value) => {
                        // TLS fields are unregistered for remote and certificate-only modes.
                        // Restore a valid default before those controls mount again.
                        form.setFieldValue(
                          'tls',
                          value === 'cert_only'
                            ? 'certmagic'
                            : value === 'shared_sni'
                              ? 'auto'
                              : form.getFieldValue('tls') ||
                                (value === 'sni_only' ? 'certmagic' : 'auto'),
                        );
                        if (
                          value === 'sub_link_only' ||
                          value === 'sni_only' ||
                          value === 'shared_sni' ||
                          value === 'cert_only'
                        ) {
                          form.setFieldsValue({
                            inboundIds: [],
                            origin: '',
                            tls:
                              value === 'cert_only'
                                ? 'certmagic'
                                : value === 'shared_sni'
                                  ? 'auto'
                                  : form.getFieldValue('tls'),
                          });
                          if (value === 'sni_only') {
                            form.setFieldsValue({
                              siteCatalog: 'builtin',
                              siteTemplate: 'notes',
                              siteHtml: '',
                            });
                          } else {
                            form.setFieldsValue({
                              siteCatalog: '',
                              siteTemplate: '',
                              siteHtml: '',
                            });
                          }
                          if (
                            value === 'sni_only' ||
                            value === 'shared_sni' ||
                            value === 'cert_only'
                          )
                            setPrimaryDomain(false);
                        } else if (value !== 'relay') form.setFieldsValue({ origin: '' });
                      }}
                      options={[
                        'direct',
                        'cdn',
                        'relay',
                        'sub_link_only',
                        'sni_only',
                        'shared_sni',
                        'cert_only',
                      ].map((value) => ({
                        value,
                        label:
                          value === 'cert_only'
                            ? i18n.language.startsWith('ru')
                              ? 'Только сертификат (DNS-01)'
                              : 'Certificate only (DNS-01)'
                            : label(`mode.${value}`),
                      }))}
                    />
                  </Form.Item>
                  <Typography.Paragraph type="secondary">
                    {mode === 'cert_only'
                      ? i18n.language.startsWith('ru')
                        ? 'Сертификат выпускается и продлевается через DNS-01 на основной ноде, без сайта и портов 80/443.'
                        : 'Issues and renews a certificate via DNS-01 on this node without a website or public listeners.'
                      : mode && label(`modeDesc.${mode}`)}
                  </Typography.Paragraph>
                  {mode === 'relay' && (
                    <Form.Item
                      name="origin"
                      label={label('origin')}
                      tooltip={label('originDesc')}
                      rules={[{ required: true, message: label('originRequired') }]}
                    >
                      <Input placeholder="https://origin.example.com" />
                    </Form.Item>
                  )}
                  {mode !== 'sub_link_only' &&
                    mode !== 'sni_only' &&
                    mode !== 'shared_sni' &&
                    mode !== 'cert_only' && (
                      <>
                        <Form.Item
                          name="inboundIds"
                          label={label('inbounds')}
                          tooltip={label('inboundsDesc')}
                        >
                          <Select
                            mode="multiple"
                            options={compatible.map((o) => ({
                              value: o.id,
                              label: `${o.remark} · ${o.network} · ${o.port}`,
                            }))}
                          />
                        </Form.Item>
                      </>
                    )}
                  {mode !== 'sni_only' && mode !== 'shared_sni' && mode !== 'cert_only' && (
                    <Form.Item tooltip={label('useForSubscriptionHelp')}>
                      <Checkbox
                        checked={primaryDomain}
                        onChange={(event) => setPrimaryDomain(event.target.checked)}
                      >
                        {label('useForSubscription')}
                      </Checkbox>
                    </Form.Item>
                  )}
                </>
              ),
            },
            ...(mode === 'sni_only'
              ? [
                  {
                    key: 'site',
                    label: label('redesign.coverSite'),
                    forceRender: true,
                    children: (
                      <>
                        {mode === 'sni_only' && (
                          <>
                            <Form.Item
                              name="siteCatalog"
                              label={
                                i18n.language.startsWith('ru')
                                  ? 'Каталог сайта-заглушки'
                                  : 'Cover site catalog'
                              }
                            >
                              <Select
                                options={[
                                  ...Object.entries(TelemtWebCatalogs).map(([value, entry]) => ({
                                    value,
                                    label: i18n.language.startsWith('ru') ? entry.ru : entry.en,
                                  })),
                                  {
                                    value: 'custom',
                                    label: i18n.language.startsWith('ru')
                                      ? 'Свой HTML'
                                      : 'Custom HTML',
                                  },
                                ]}
                                onChange={(value: 'builtin' | 'mtproxyl' | 'custom') => {
                                  form.setFieldValue(
                                    'siteTemplate',
                                    value === 'custom'
                                      ? 'html'
                                      : value === 'builtin'
                                        ? 'notes'
                                        : 'stub',
                                  );
                                  if (value === 'custom' && !form.getFieldValue('siteHtml')) {
                                    form.setFieldValue('siteHtml', starterCoverSiteHTML);
                                  }
                                }}
                              />
                            </Form.Item>
                            <Form.Item
                              name="siteTemplate"
                              label={
                                i18n.language.startsWith('ru')
                                  ? 'Какой сайт создать'
                                  : 'Website template'
                              }
                              rules={[{ required: true }]}
                            >
                              {siteCatalog === 'custom' ? (
                                <Select
                                  disabled
                                  options={[
                                    {
                                      value: 'html',
                                      label: i18n.language.startsWith('ru')
                                        ? 'Моя HTML-страница'
                                        : 'My HTML page',
                                    },
                                  ]}
                                />
                              ) : (
                                <Select
                                  options={TelemtWebCatalogs[
                                    siteCatalog === 'mtproxyl' ? 'mtproxyl' : 'builtin'
                                  ].templates.map((entry) => ({
                                    value: entry.id,
                                    label: i18n.language.startsWith('ru') ? entry.ru : entry.en,
                                    title: i18n.language.startsWith('ru')
                                      ? entry.descriptionRu
                                      : entry.descriptionEn,
                                  }))}
                                />
                              )}
                            </Form.Item>
                            {siteCatalog === 'custom' && (
                              <Form.Item
                                name="siteHtml"
                                rules={[
                                  {
                                    validator: (_, value: string) =>
                                      coverSiteHTMLValidation(value || '')
                                        ? Promise.reject(
                                            new Error(
                                              i18n.language.startsWith('ru')
                                                ? 'Укажите полный HTML до 256 КБ'
                                                : 'Enter a full HTML document up to 256 KiB',
                                            ),
                                          )
                                        : Promise.resolve(),
                                  },
                                ]}
                              >
                                <CoverSiteHTMLEditor />
                              </Form.Item>
                            )}
                          </>
                        )}
                      </>
                    ),
                  },
                ]
              : []),
            {
              key: 'tls',
              label: label('redesign.tlsAndState'),
              forceRender: true,
              children: (
                <>
                  <>
                    <Form.Item
                      name="enabled"
                      label={label('domainEnabled')}
                      tooltip={label('domainEnabledHelp')}
                      valuePropName="checked"
                    >
                      <Switch />
                    </Form.Item>
                    {mode !== 'shared_sni' && mode !== 'cert_only' && (
                      <>
                        <Form.Item
                          name="tls"
                          label={label('tlsLabel')}
                          tooltip={label('tlsHelp')}
                          rules={[antdRule(DomainFormSchema.shape.tls, t)]}
                        >
                          <Select
                            options={['auto', 'certmagic', 'existing', 'manual'].map((value) => ({
                              value,
                              label: label(`tls.${value}`),
                            }))}
                          />
                        </Form.Item>
                        <Typography.Paragraph type="secondary">
                          {label(
                            tls === 'manual'
                              ? 'manualDesc'
                              : tls === 'certmagic'
                                ? 'certmagicDesc'
                                : tls === 'existing'
                                  ? 'existingTLSDesc'
                                  : 'autoDesc',
                          )}
                        </Typography.Paragraph>
                      </>
                    )}
                    {mode !== 'shared_sni' && mode !== 'cert_only' && tls === 'manual' && (
                      <>
                        <Form.Item
                          name="certFile"
                          label={label('certFile')}
                          tooltip={label('certFileHelp')}
                          rules={[{ required: true }]}
                        >
                          <Input placeholder="/etc/ssl/fullchain.pem" />
                        </Form.Item>
                        <Form.Item
                          name="keyFile"
                          label={label('keyFile')}
                          tooltip={label('keyFileHelp')}
                          rules={[{ required: true }]}
                        >
                          <Input placeholder="/etc/ssl/privkey.pem" />
                        </Form.Item>
                      </>
                    )}
                  </>
                  {(mode === 'cert_only' || mode === 'shared_sni') && (
                    <Typography.Paragraph type="secondary">
                      {mode === 'cert_only' ? label('dns01.hint') : label('remoteSNIInfo')}
                    </Typography.Paragraph>
                  )}
                </>
              ),
            },
          ]}
        />
        <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
          {label('workspace.editorSaveHint')}
        </Typography.Paragraph>
      </Form>
    </Drawer>
  );
}
