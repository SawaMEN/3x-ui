import * as React from 'react';
import { createRoot } from 'react-dom/client';
import * as antd from 'antd';
import { MemoryRouter } from 'react-router';
import { QueryProvider } from '@/api/QueryProvider';
import { ThemeProvider, useTheme } from '@/hooks/useTheme';
import { AntdRootConfig } from '@/i18n/AntdRootConfig';
import { readyI18n } from '@/i18n/react';
import { setupHttp } from '@/api/http-init';

export async function mountThemeAudit() {
  window.X_UI_BASE_PATH = '/';
  window.__SUB_PAGE_DATA__ = { enabled: true, subTitle: 'Theme audit', links: [], emails: [] };
  await readyI18n();
  setupHttp();
  for (const css of ['utils', 'page-shell', 'page-cards', 'material-ui']) {
    await import(`/src/styles/${css}.css`);
  }
  await import('/src/pages/xray/routing/RoutingTab.css');
  const h = React.createElement;
  const root = createRoot(document.getElementById('app'));
  let setView;
  function Harness() {
    const { antdThemeConfig, setThemeMode, toggleLowPower } = useTheme();
    const [view, updateView] = React.useState(null);
    React.useLayoutEffect(() => {
      setView = updateView;
      window.audit.setMode = setThemeMode;
      window.audit.toggleLowPower = toggleLowPower;
    }, [setThemeMode, toggleLowPower]);
    return h(
      AntdRootConfig,
      null,
      h(
        antd.ConfigProvider,
        { theme: antdThemeConfig },
        h(QueryProvider, null, h(MemoryRouter, null, view)),
      ),
    );
  }
  window.audit = {
    async showPage(path) {
      const mod = await import(`/src/pages/${path}.tsx`);
      setView(h(mod.default));
    },
    async showModal(kind) {
      antd.Modal.destroyAll();
      if (kind === 'static') {
        setView(null);
        antd.Modal.confirm({
          title: 'Theme confirmation',
          content: h('div', { className: 'rule-card' }, 'Portal routing card'),
        });
        antd.message.info({ content: 'Theme message', duration: 0 });
        antd.notification.info({ title: 'Theme notification', duration: 0 });
        return;
      }
      const noop = () => {};
      if (kind === 'controls') {
        const { default: DateTimePicker } = await import('/src/components/form/DateTimePicker.tsx');
        const { default: JsonEditor } = await import('/src/components/form/JsonEditor.tsx');
        const { setDatepicker } = await import('/src/hooks/useDatepicker.ts');
        setDatepicker('jalalian');
        setView(
          h(
            antd.Modal,
            { title: 'Shared controls', open: true, footer: null },
            h(antd.Input, { placeholder: 'Input' }),
            h(antd.Select, { open: true, options: [{ value: 'one', label: 'One' }] }),
            h(antd.Popover, { open: true, content: 'Popover' }, h('span', null, 'Anchor')),
            h(antd.DatePicker, { open: true }),
            h(DateTimePicker, { value: null, onChange: noop }),
            h(JsonEditor, { value: '{"theme":true}', minHeight: '100px' }),
            h('div', { className: 'rule-card' }, 'Routing card'),
          ),
        );
        return;
      }
      const cases = {
        inbound: [
          'inbounds/form/InboundFormModal',
          { mode: 'add', dbInbound: null, dbInbounds: [], onClose: noop, onSaved: noop },
        ],
        client: [
          'clients/ClientFormModal',
          {
            mode: 'add',
            client: null,
            inbounds: [],
            save: async () => null,
            onOpenChange: noop,
          },
        ],
        node: [
          'nodes/NodeFormModal',
          {
            mode: 'add',
            node: null,
            testConnection: async () => ({}),
            fetchFingerprint: async () => ({}),
            fetchInbounds: async () => ({}),
            save: async () => ({}),
            onOpenChange: noop,
          },
        ],
        outbound: [
          'singbox/SingBoxOutboundModal',
          { value: null, existingTags: [], onCancel: noop, onSave: noop },
        ],
        rule: [
          'xray/routing/RuleFormModal',
          {
            rule: null,
            inboundTags: [],
            outboundTags: [],
            balancerTags: [],
            onClose: noop,
            onConfirm: noop,
          },
        ],
        backup: ['index/BackupModal', { basePath: '/', onClose: noop, onBusy: noop }],
        security: [
          'settings/TwoFactorModal',
          { title: 'Two-factor authentication', onConfirm: noop, onOpenChange: noop },
        ],
      };
      const [path, props] = cases[kind];
      const { default: Component } = await import(`/src/pages/${path}.tsx`);
      setView(h(Component, { ...props, open: true }));
    },
    clearFeedback() {
      antd.Modal.destroyAll();
      antd.message.destroy();
      antd.notification.destroy();
    },
  };
  root.render(h(ThemeProvider, null, h(Harness)));
}
